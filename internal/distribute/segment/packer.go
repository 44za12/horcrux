package segment

import (
	"fmt"
	"os"

	"horcrux/internal/config"
	"horcrux/internal/crypto"
	"horcrux/internal/vault/filestore"
)

// Packer converts vault content to/from segments.
type Packer struct {
	fileStore *filestore.Store
}

// NewPacker creates a Packer for the vault's chunked file store.
func NewPacker(fileStore *filestore.Store) *Packer {
	return &Packer{fileStore: fileStore}
}

// VaultToSegments serializes all vault data into a list of segments.
// Each segment is a self-contained block of up to TargetSize bytes.
//
// Metadata files (passes, totp, apikeys) are packed first, each as a single
// entry. File chunks from the chunked store follow, grouped into segments
// of approximately TargetSize.
//
// Returns the ordered list of segment references for the manifest.
func (p *Packer) VaultToSegments(passphrase string) ([]*Segment, error) {
	var allEntries []SegmentEntry

	// --- Metadata files ---
	metadataPaths := []struct {
		path string
		name string
	}{
		{config.PassesPath(), "passes.hrcrx"},
		{config.TotpPassPath(), "totp.hrcrx"},
		{config.ApiKeysPath(), "apikeys.hrcrx"},
	}

	for _, mf := range metadataPaths {
		data, err := crypto.DecryptFile(mf.path, passphrase)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("decrypting %s: %w", mf.name, err)
		}
		allEntries = append(allEntries, SegmentEntry{
			Type: 0, // metadata file (plaintext)
			Name: mf.name,
			Data: data,
		})
	}

	// --- File chunks from chunked store ---
	if p.fileStore != nil {
		err := p.fileStore.StreamChunks(passphrase, func(ss filestore.SegmentStream) error {
			name := fmt.Sprintf("%s:%d", ss.FileName, ss.ChunkIndex)
			allEntries = append(allEntries, SegmentEntry{
				Type:      1, // file chunk
				Name:      name,
				Data:      ss.Data,
				MimeType:  ss.MimeType,
				TotalSize: ss.TotalSize,
				ChunkHash: ss.ChunkHash,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("streaming file chunks: %w", err)
		}
	}

	if len(allEntries) == 0 {
		return nil, fmt.Errorf("no vault data found to distribute")
	}

	// --- Pack entries into segments ---
	return packEntriesIntoSegments(allEntries), nil
}

// SegmentsToVault reconstructs vault files from segments.
// Metadata files are written directly; file chunks are imported into the store.
func (p *Packer) SegmentsToVault(segments []*Segment, passphrase string) error {
	for _, seg := range segments {
		if err := seg.Unpack(); err != nil {
			return fmt.Errorf("unpacking segment %s: %w", seg.Hash[:16], err)
		}

		for _, entry := range seg.Entries {
			switch entry.Type {
			case 0: // Metadata file
				var filePath string
				switch entry.Name {
				case "passes.hrcrx":
					filePath = config.PassesPath()
				case "totp.hrcrx":
					filePath = config.TotpPassPath()
				case "apikeys.hrcrx":
					filePath = config.ApiKeysPath()
				default:
					return fmt.Errorf("unknown metadata file in segment: %s", entry.Name)
				}
				// Re-encrypt with current passphrase
				if err := crypto.EncryptFile(filePath, entry.Data, passphrase); err != nil {
					return fmt.Errorf("re-encrypting %s: %w", entry.Name, err)
				}

			case 1: // File chunk
				if p.fileStore == nil {
					return fmt.Errorf("file chunk in segment but no file store configured")
				}
				if err := p.fileStore.ImportChunk(entry.ChunkHash, entry.Data, passphrase); err != nil {
					return fmt.Errorf("importing chunk %s: %w", entry.ChunkHash[:16], err)
				}

			default:
				return fmt.Errorf("unknown entry type %d in segment", entry.Type)
			}
		}
	}

	return nil
}

// RebuildFileIndex reconstructs the file store index from segment entries.
// Must be called after SegmentsToVault to link chunks back to their files.
func (p *Packer) RebuildFileIndex(segments []*Segment, passphrase string) error {
	if p.fileStore == nil {
		return nil
	}

	// Collect file metadata from chunk entries
	type fileInfo struct {
		mimeType    string
		totalSize   int64
		chunkHashes []string // indexed by chunk index
	}
	files := make(map[string]*fileInfo)

	for _, seg := range segments {
		for _, entry := range seg.Entries {
			if entry.Type != 1 {
				continue
			}
			// Entry name format: "filename:chunkIdx"
			// Parse it to group chunks by filename
			lastColon := -1
			for i := len(entry.Name) - 1; i >= 0; i-- {
				if entry.Name[i] == ':' {
					lastColon = i
					break
				}
			}
			if lastColon < 0 {
				continue
			}
			filename := entry.Name[:lastColon]

			fi, ok := files[filename]
			if !ok {
				fi = &fileInfo{
					mimeType:    entry.MimeType,
					totalSize:   entry.TotalSize,
					chunkHashes: make([]string, 0),
				}
				files[filename] = fi
			}
			// Parse chunk index from the same lastColon we already found
			chunkIdx := 0
			fmt.Sscanf(entry.Name[lastColon+1:], "%d", &chunkIdx)
			// Ensure slice is large enough
			for len(fi.chunkHashes) <= chunkIdx {
				fi.chunkHashes = append(fi.chunkHashes, "")
			}
			fi.chunkHashes[chunkIdx] = entry.ChunkHash
		}
	}

	for filename, fi := range files {
		// Compact: remove empty slots
		hashes := make([]string, 0, len(fi.chunkHashes))
		for _, h := range fi.chunkHashes {
			if h != "" {
				hashes = append(hashes, h)
			}
		}
		if err := p.fileStore.RebuildIndex(filename, filestore.FileMeta{
			MimeType:    fi.mimeType,
			TotalSize:   fi.totalSize,
			ChunkHashes: hashes,
		}, passphrase); err != nil {
			return fmt.Errorf("rebuilding index for %s: %w", filename, err)
		}
	}

	return nil
}

// packEntriesIntoSegments groups entries into segments of approximately TargetSize.
func packEntriesIntoSegments(entries []SegmentEntry) []*Segment {
	var segments []*Segment
	var current []SegmentEntry
	var currentSize int

	flush := func() {
		if len(current) > 0 {
			segments = append(segments, Pack(current))
			current = nil
			currentSize = 0
		}
	}

	for _, e := range entries {
		entrySize := len(e.Data) + len(e.Name) + 100 // ~overhead
		if currentSize+entrySize > TargetSize && len(current) > 0 {
			flush()
		}
		current = append(current, e)
		currentSize += entrySize
	}
	flush()

	return segments
}

// CollectEntryHashes returns the set of chunk hashes referenced in segments.
// Used for GC to know which chunks are in the current distribution.
func CollectEntryHashes(segments []*Segment) map[string]bool {
	hashes := make(map[string]bool)
	for _, seg := range segments {
		for _, entry := range seg.Entries {
			if entry.Type == 1 && entry.ChunkHash != "" {
				hashes[entry.ChunkHash] = true
			}
		}
	}
	return hashes
}
