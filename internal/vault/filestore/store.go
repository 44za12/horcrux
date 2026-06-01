package filestore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"horcrux/internal/crypto"
)

// ChunkSize is the target size for each content-addressed chunk.
// Larger chunks = fewer index entries but more data re-uploaded on small changes.
// 4MB is a good tradeoff for most use cases.
const ChunkSize = 4 * 1024 * 1024 // 4 MB

// Store provides content-addressed, chunked file storage.
// Files are split into fixed-size chunks, each encrypted individually
// and stored under its SHA-256 hash. An encrypted index maps filenames
// to their chunk lists.
type Store struct {
	baseDir string
	index   *indexManager
}

// NewStore creates or opens a chunked file store at the given directory.
// The directory must exist (caller ensures this).
func NewStore(baseDir string) *Store {
	return &Store{
		baseDir: baseDir,
		index:   newIndexManager(baseDir),
	}
}

// BaseDir returns the store's root directory.
func (s *Store) BaseDir() string { return s.baseDir }

// ensureChunksDir creates the chunks subdirectory if it doesn't exist.
func (s *Store) ensureChunksDir() error {
	return os.MkdirAll(filepath.Join(s.baseDir, "chunks"), 0700)
}

// AddFile splits data into chunks, encrypts each, stores them, and
// updates the index. If a chunk already exists (same plaintext hash),
// it is not re-written.
func (s *Store) AddFile(filename, mimeType string, data []byte, passphrase string) error {
	if err := s.ensureChunksDir(); err != nil {
		return err
	}

	var chunkHashes []string
	offset := 0
	for offset < len(data) {
		end := offset + ChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]
		hash := hashPlaintext(chunk)

		// Only write chunk if it doesn't already exist (content-addressed dedup)
		cPath := chunkPath(s.baseDir, hash)
		if _, err := os.Stat(cPath); os.IsNotExist(err) {
			encrypted, err := crypto.EncryptData(chunk, passphrase)
			if err != nil {
				return fmt.Errorf("encrypting chunk %s: %w", hash[:16], err)
			}
			if err := os.WriteFile(cPath, encrypted, 0600); err != nil {
				return fmt.Errorf("writing chunk %s: %w", hash[:16], err)
			}
		}

		chunkHashes = append(chunkHashes, hash)
		offset = end
	}

	idx, err := s.index.load(passphrase)
	if err != nil {
		return err
	}

	idx.Files[filename] = FileMeta{
		MimeType:    mimeType,
		TotalSize:   int64(len(data)),
		ChunkHashes: chunkHashes,
	}

	return s.index.save(idx, passphrase)
}

// GetFile reads a file from the store by reassembling its chunks.
func (s *Store) GetFile(filename, passphrase string) ([]byte, string, error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return nil, "", err
	}

	meta, ok := idx.Files[filename]
	if !ok {
		return nil, "", fmt.Errorf("file '%s' not found", filename)
	}

	var result []byte
	for _, hash := range meta.ChunkHashes {
		cPath := chunkPath(s.baseDir, hash)
		encrypted, err := os.ReadFile(cPath)
		if err != nil {
			return nil, "", fmt.Errorf("reading chunk %s: %w", hash[:16], err)
		}
		plaintext, err := crypto.DecryptData(encrypted, passphrase)
		if err != nil {
			return nil, "", fmt.Errorf("decrypting chunk %s: %w", hash[:16], err)
		}
		result = append(result, plaintext...)
	}

	return result, meta.MimeType, nil
}

// RemoveFile removes a file entry from the index. Chunks are not deleted
// immediately — call GC() to remove unreferenced chunks.
func (s *Store) RemoveFile(filename, passphrase string) error {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return err
	}

	delete(idx.Files, filename)
	return s.index.save(idx, passphrase)
}

// ListFiles returns all stored file metadata.
func (s *Store) ListFiles(passphrase string) ([]FileMeta, error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return nil, err
	}

	var files []FileMeta
	for name, meta := range idx.Files {
		files = append(files, FileMeta{
			MimeType:    meta.MimeType,
			TotalSize:   meta.TotalSize,
			ChunkHashes: append([]string{}, meta.ChunkHashes...),
		})
		// Store filename in MimeType field... hacky, but we need the name
		_ = name
	}
	return files, nil
}

// FileEntry holds a file listing result with the filename.
type FileEntry struct {
	Name     string
	MimeType string
	Size     int64
}

// ListFileEntries returns all stored files with their names.
func (s *Store) ListFileEntries(passphrase string) ([]FileEntry, error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return nil, err
	}

	var entries []FileEntry
	for name, meta := range idx.Files {
		entries = append(entries, FileEntry{
			Name:     name,
			MimeType: meta.MimeType,
			Size:     meta.TotalSize,
		})
	}
	return entries, nil
}

// GC removes chunks that are no longer referenced by any file in the index.
// Returns the number of chunks removed.
func (s *Store) GC(passphrase string) (int, error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return 0, err
	}

	// Build set of referenced hashes
	referenced := make(map[string]bool)
	for _, meta := range idx.Files {
		for _, hash := range meta.ChunkHashes {
			referenced[hash] = true
		}
	}

	chunksDir := filepath.Join(s.baseDir, "chunks")
	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !referenced[entry.Name()] {
			if err := os.Remove(filepath.Join(chunksDir, entry.Name())); err != nil {
				return removed, err
			}
			removed++
		}
	}

	return removed, nil
}

// SegmentStream holds one segment of a file's data for incremental distribution.
type SegmentStream struct {
	FileName   string
	MimeType   string
	TotalSize  int64
	ChunkHash  string // SHA-256 hex of this chunk's plaintext
	ChunkIndex int    // 0-based index within the file
	Data       []byte // plaintext chunk data
}

// StreamChunks iterates over all chunks in all files, calling fn for each.
// This is used by the distribution system to build segments.
func (s *Store) StreamChunks(passphrase string, fn func(SegmentStream) error) error {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return err
	}

	for name, meta := range idx.Files {
		for i, hash := range meta.ChunkHashes {
			cPath := chunkPath(s.baseDir, hash)
			encrypted, err := os.ReadFile(cPath)
			if err != nil {
				return fmt.Errorf("reading chunk %s: %w", hash[:16], err)
			}
			plaintext, err := crypto.DecryptData(encrypted, passphrase)
			if err != nil {
				return fmt.Errorf("decrypting chunk %s: %w", hash[:16], err)
			}

			if err := fn(SegmentStream{
				FileName:   name,
				MimeType:   meta.MimeType,
				TotalSize:  meta.TotalSize,
				ChunkHash:  hash,
				ChunkIndex: i,
				Data:       plaintext,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// ImportChunk writes a single chunk from a remote source (used during restore).
// If the chunk already exists, it is not overwritten.
func (s *Store) ImportChunk(hash string, plaintext []byte, passphrase string) error {
	if err := s.ensureChunksDir(); err != nil {
		return err
	}

	cPath := chunkPath(s.baseDir, hash)
	if _, err := os.Stat(cPath); err == nil {
		// Already exists — verify hash matches
		if hashPlaintext(plaintext) != hash {
			return fmt.Errorf("chunk hash mismatch: expected %s", hash)
		}
		return nil
	}

	// Verify hash before writing
	if got := hashPlaintext(plaintext); got != hash {
		return fmt.Errorf("chunk hash mismatch: expected %s, got %s", hash, got)
	}

	encrypted, err := crypto.EncryptData(plaintext, passphrase)
	if err != nil {
		return fmt.Errorf("encrypting imported chunk: %w", err)
	}
	return os.WriteFile(cPath, encrypted, 0600)
}

// RebuildIndex adds a file entry to the index from a distribution manifest.
func (s *Store) RebuildIndex(filename string, meta FileMeta, passphrase string) error {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return err
	}
	idx.Files[filename] = meta
	return s.index.save(idx, passphrase)
}

// ChunkHashesForFile returns the ordered chunk hash list for a file.
// Used during distribution to know which chunks belong to which file.
func (s *Store) ChunkHashesForFile(filename, passphrase string) ([]string, FileMeta, error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return nil, FileMeta{}, err
	}
	meta, ok := idx.Files[filename]
	if !ok {
		return nil, FileMeta{}, fmt.Errorf("file '%s' not found", filename)
	}
	return meta.ChunkHashes, meta, nil
}

// AllChunkHashes returns every chunk hash referenced in the index (deduplicated).
func (s *Store) AllChunkHashes(passphrase string) ([]string, error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var hashes []string
	for _, meta := range idx.Files {
		for _, h := range meta.ChunkHashes {
			if !seen[h] {
				seen[h] = true
				hashes = append(hashes, h)
			}
		}
	}
	return hashes, nil
}

// ExportIndex returns the raw index data for inclusion in a distribution manifest.
func (s *Store) ExportIndex(passphrase string) (map[string]FileMeta, error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return nil, err
	}
	// Return a copy
	result := make(map[string]FileMeta)
	for k, v := range idx.Files {
		meta := FileMeta{
			MimeType:    v.MimeType,
			TotalSize:   v.TotalSize,
			ChunkHashes: append([]string{}, v.ChunkHashes...),
		}
		result[k] = meta
	}
	return result, nil
}

// ImportIndex replaces the index with data from a distribution manifest.
func (s *Store) ImportIndex(files map[string]FileMeta, passphrase string) error {
	idx := &fileIndex{Files: make(map[string]FileMeta)}
	for k, v := range files {
		idx.Files[k] = v
	}
	return s.index.save(idx, passphrase)
}

// VerifyChunk checks that a chunk exists and has the correct plaintext hash.
func (s *Store) VerifyChunk(hash, passphrase string) (bool, error) {
	cPath := chunkPath(s.baseDir, hash)
	encrypted, err := os.ReadFile(cPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	plaintext, err := crypto.DecryptData(encrypted, passphrase)
	if err != nil {
		return false, err
	}
	return hashPlaintext(plaintext) == hash, nil
}

// Stats returns the number of files, total chunks, and total plaintext bytes.
func (s *Store) Stats(passphrase string) (numFiles int, numChunks int, totalBytes int64, err error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return 0, 0, 0, err
	}
	numFiles = len(idx.Files)
	seen := make(map[string]bool)
	for _, meta := range idx.Files {
		totalBytes += meta.TotalSize
		for _, h := range meta.ChunkHashes {
			if !seen[h] {
				seen[h] = true
				numChunks++
			}
		}
	}
	return
}

// Migration helpers

// ImportLegacyFile imports a single file from the old BSON-based files.hrcrx
// into the chunked store. Called during migration.
func (s *Store) ImportLegacyFile(filename, mimeType string, data []byte, passphrase string) error {
	return s.AddFile(filename, mimeType, data, passphrase)
}

// HasLegacyData checks if a legacy encrypted file exists and can be migrated.
func HasLegacyData(legacyPath string) bool {
	_, err := os.Stat(legacyPath)
	return err == nil
}

// IsEmpty returns true if the store has no files.
func (s *Store) IsEmpty(passphrase string) (bool, error) {
	idx, err := s.index.load(passphrase)
	if err != nil {
		return false, err
	}
	return len(idx.Files) == 0, nil
}

// Ensure defaults
var _ = strings.TrimSpace
