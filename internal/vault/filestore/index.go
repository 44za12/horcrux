package filestore

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"horcrux/internal/crypto"
)

// FileMeta describes a stored file's metadata and chunk references.
type FileMeta struct {
	MimeType    string   `json:"mime_type" bson:"mime_type"`
	TotalSize   int64    `json:"total_size" bson:"total_size"`
	ChunkHashes []string `json:"chunk_hashes" bson:"chunk_hashes"`
}

// fileIndex is the on-disk representation of the file index.
type fileIndex struct {
	Files map[string]FileMeta `json:"files" bson:"files"`
}

// indexManager handles reading/writing the encrypted index file.
type indexManager struct {
	indexPath string
}

func newIndexManager(baseDir string) *indexManager {
	return &indexManager{
		indexPath: filepath.Join(baseDir, "index.hrcrx"),
	}
}

func (m *indexManager) load(passphrase string) (*fileIndex, error) {
	data, err := crypto.DecryptBSONFile(m.indexPath, passphrase)
	if err != nil {
		if os.IsNotExist(err) {
			return &fileIndex{Files: make(map[string]FileMeta)}, nil
		}
		return nil, fmt.Errorf("decrypting file index: %w", err)
	}
	idx := &fileIndex{Files: make(map[string]FileMeta)}
	// DecryptBSONFile returns map[string]map[string]string; we need to convert
	for name, fields := range data {
		if fields == nil {
			continue
		}
		meta := FileMeta{
			MimeType:  fields["mime_type"],
			TotalSize: 0,
		}
		if sizeStr := fields["total_size"]; sizeStr != "" {
			fmt.Sscanf(sizeStr, "%d", &meta.TotalSize)
		}
		// Chunk hashes are stored as comma-separated in a single field
		if hashesStr := fields["chunk_hashes"]; hashesStr != "" {
			// Simple split by comma
			start := 0
			for i := 0; i <= len(hashesStr); i++ {
				if i == len(hashesStr) || hashesStr[i] == ',' {
					if i > start {
						meta.ChunkHashes = append(meta.ChunkHashes, hashesStr[start:i])
					}
					start = i + 1
				}
			}
		}
		idx.Files[name] = meta
	}
	return idx, nil
}

func (m *indexManager) save(idx *fileIndex, passphrase string) error {
	// Convert to map[string]map[string]string for BSON encryption
	data := make(map[string]map[string]string)
	for name, meta := range idx.Files {
		hashesStr := ""
		for i, h := range meta.ChunkHashes {
			if i > 0 {
				hashesStr += ","
			}
			hashesStr += h
		}
		data[name] = map[string]string{
			"mime_type":    meta.MimeType,
			"total_size":   fmt.Sprintf("%d", meta.TotalSize),
			"chunk_hashes": hashesStr,
		}
	}
	return crypto.EncryptBSONFile(m.indexPath, data, passphrase)
}

// chunkPath returns the on-disk path for a chunk with the given SHA-256 hex hash.
func chunkPath(baseDir, hash string) string {
	return filepath.Join(baseDir, "chunks", hash)
}

// hashPlaintext returns the hex-encoded SHA-256 of data.
func hashPlaintext(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
