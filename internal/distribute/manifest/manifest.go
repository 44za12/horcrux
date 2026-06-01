package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"horcrux/internal/crypto"
	"horcrux/internal/distribute/segment"
	"horcrux/internal/vault/filestore"
	"horcrux/storage"
)

// Manifest is the versioned index of a distributed vault.
// It is small (~KB), fully replicated to all providers, and encrypted
// with the user's passphrase. The DEK is NOT in the manifest — it is
// separately Shamir-split and stored per-provider.
type Manifest struct {
	Version   uint64                   `json:"version"`
	Timestamp int64                    `json:"timestamp"`
	Segments  []segment.SegmentRef     `json:"segments"`
	FileIndex map[string]filestore.FileMeta `json:"file_index,omitempty"`
}

// Marshal serializes a Manifest to JSON bytes.
func (m *Manifest) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// Unmarshal deserializes a Manifest from JSON bytes.
func Unmarshal(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// --- Provider key naming ---

const (
	manifestKeyPrefix  = "manifest.v"
	dekshareKeyPrefix  = "dekshare.v"
	segmentKeyPrefix   = "seg."
	manifestKeySuffix  = ".hrcrx"
	dekshareKeySuffix  = ".hrcrx"
	segmentKeySuffix   = ".hrcrx"
)

// ManifestKey returns the provider key for a manifest version.
func ManifestKey(version uint64) string {
	return fmt.Sprintf("%s%d%s", manifestKeyPrefix, version, manifestKeySuffix)
}

// DekshareKey returns the provider key for a DEK share version.
func DekshareKey(version uint64) string {
	return fmt.Sprintf("%s%d%s", dekshareKeyPrefix, version, dekshareKeySuffix)
}

// SegmentShardKey returns the provider key for a specific erasure shard of a segment.
func SegmentShardKey(segHash string, shardIdx int) string {
	return fmt.Sprintf("%s%s.%d%s", segmentKeyPrefix, segHash, shardIdx, segmentKeySuffix)
}

// ParseManifestVersion extracts the version number from a manifest key.
// Returns 0 if the key doesn't match the expected format.
func ParseManifestVersion(key string) uint64 {
	if !strings.HasPrefix(key, manifestKeyPrefix) || !strings.HasSuffix(key, manifestKeySuffix) {
		return 0
	}
	versionStr := key[len(manifestKeyPrefix) : len(key)-len(manifestKeySuffix)]
	v, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// --- Provider operations ---

// FindLatestVersion scans all providers for the highest manifest version.
func FindLatestVersion(ctx context.Context, providers []storage.Provider) (uint64, error) {
	var latest uint64
	for _, p := range providers {
		keys, err := p.List(ctx, manifestKeyPrefix)
		if err != nil {
			continue // skip providers that fail listing
		}
		for _, key := range keys {
			v := ParseManifestVersion(key)
			if v > latest {
				latest = v
			}
		}
	}
	return latest, nil
}

// UploadManifest encrypts and uploads a manifest to a single provider.
func UploadManifest(ctx context.Context, p storage.Provider, m *Manifest, passphrase string) error {
	data, err := m.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	encrypted, err := crypto.EncryptData(data, passphrase)
	if err != nil {
		return fmt.Errorf("encrypting manifest: %w", err)
	}
	key := ManifestKey(m.Version)
	if err := p.Upload(ctx, key, encrypted); err != nil {
		return fmt.Errorf("uploading manifest to %s: %w", p.Name(), err)
	}
	return nil
}

// DownloadManifest downloads and decrypts a manifest from a single provider.
func DownloadManifest(ctx context.Context, p storage.Provider, version uint64, passphrase string) (*Manifest, error) {
	key := ManifestKey(version)
	encrypted, err := p.Download(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("downloading manifest from %s: %w", p.Name(), err)
	}
	plaintext, err := crypto.DecryptData(encrypted, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypting manifest: %w", err)
	}
	return Unmarshal(plaintext)
}

// DownloadManifestFromAny tries to download a specific manifest version from any provider.
func DownloadManifestFromAny(ctx context.Context, providers []storage.Provider, version uint64, passphrase string) (*Manifest, error) {
	for _, p := range providers {
		m, err := DownloadManifest(ctx, p, version, passphrase)
		if err == nil {
			return m, nil
		}
	}
	return nil, fmt.Errorf("manifest v%d not found on any provider", version)
}

// UploadDekshare encrypts and uploads a single DEK share to a provider.
func UploadDekshare(ctx context.Context, p storage.Provider, version uint64, share []byte, passphrase string) error {
	encrypted, err := crypto.EncryptData(share, passphrase)
	if err != nil {
		return fmt.Errorf("encrypting DEK share: %w", err)
	}
	key := DekshareKey(version)
	return p.Upload(ctx, key, encrypted)
}

// DownloadDekshare downloads and decrypts a DEK share from a provider.
func DownloadDekshare(ctx context.Context, p storage.Provider, version uint64, passphrase string) ([]byte, error) {
	key := DekshareKey(version)
	encrypted, err := p.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	return crypto.DecryptData(encrypted, passphrase)
}

// CollectDekshares downloads DEK shares from providers until M are collected.
func CollectDekshares(ctx context.Context, providers []storage.Provider, version uint64, neededCount int, passphrase string) ([][]byte, error) {
	var shares [][]byte
	for _, p := range providers {
		share, err := DownloadDekshare(ctx, p, version, passphrase)
		if err != nil {
			continue
		}
		shares = append(shares, share)
		if len(shares) >= neededCount {
			break
		}
	}
	if len(shares) < neededCount {
		return nil, fmt.Errorf("only collected %d DEK shares, need %d", len(shares), neededCount)
	}
	return shares, nil
}

// NewManifest creates a new Manifest with the next version number.
func NewManifest(version uint64, segments []*segment.Segment, fileIndex map[string]filestore.FileMeta) *Manifest {
	refs := make([]segment.SegmentRef, len(segments))
	for i, seg := range segments {
		refs[i] = segment.SegmentRef{
			Hash:    seg.Hash,
			Size:    int64(len(seg.Plaintext)),
			ShardsN: 0, // filled in by caller after erasure coding
		}
	}

	// Sort file index by name for deterministic output
	if fileIndex != nil {
		// Make a copy
		fileIndex = copyFileIndex(fileIndex)
	}

	return &Manifest{
		Version:   version,
		Timestamp: time.Now().Unix(),
		Segments:  refs,
		FileIndex: fileIndex,
	}
}

func copyFileIndex(src map[string]filestore.FileMeta) map[string]filestore.FileMeta {
	dst := make(map[string]filestore.FileMeta, len(src))
	// Sort keys
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := src[k]
		dst[k] = filestore.FileMeta{
			MimeType:    v.MimeType,
			TotalSize:   v.TotalSize,
			ChunkHashes: append([]string{}, v.ChunkHashes...),
		}
	}
	return dst
}

// CollectSegmentHashes returns the set of segment hashes referenced by a manifest.
func (m *Manifest) CollectSegmentHashes() map[string]bool {
	hashes := make(map[string]bool)
	for _, ref := range m.Segments {
		hashes[ref.Hash] = true
	}
	return hashes
}

// --- Local state persistence ---

// LocalState stores the last distributed manifest and DEK info locally.
// This enables incremental distribution by knowing what was already uploaded.
type LocalState struct {
	LastVersion   uint64                   `json:"last_version"`
	LastManifest  *Manifest                `json:"last_manifest"`
	LastDEK       []byte                   `json:"last_dek"` // encrypted with vault passphrase
	SegmentHashes map[string]bool          `json:"segment_hashes"`
	FileIndex     map[string]filestore.FileMeta `json:"file_index,omitempty"`
}

// Marshal serializes LocalState to JSON.
func (ls *LocalState) Marshal() ([]byte, error) {
	return json.Marshal(ls)
}

// UnmarshalLocalState deserializes LocalState from JSON.
func UnmarshalLocalState(data []byte) (*LocalState, error) {
	var ls LocalState
	if err := json.Unmarshal(data, &ls); err != nil {
		return nil, err
	}
	if ls.SegmentHashes == nil {
		ls.SegmentHashes = make(map[string]bool)
	}
	return &ls, nil
}

// --- Key parsing helpers (used by GC) ---

// ManifestKeyPrefix returns the prefix used for manifest object keys.
func ManifestKeyPrefix() string { return manifestKeyPrefix }

// DekshareKeyPrefix returns the prefix used for DEK share object keys.
func DekshareKeyPrefix() string { return dekshareKeyPrefix }

// SegmentKeyPrefix returns the prefix used for segment shard object keys.
func SegmentKeyPrefix() string { return segmentKeyPrefix }

// ParseDekshareVersion extracts the version number from a DEK share key.
// Returns 0 if the key doesn't match the expected format.
func ParseDekshareVersion(key string) uint64 {
	if !strings.HasPrefix(key, dekshareKeyPrefix) || !strings.HasSuffix(key, dekshareKeySuffix) {
		return 0
	}
	versionStr := key[len(dekshareKeyPrefix) : len(key)-len(dekshareKeySuffix)]
	v, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// ParseSegmentHash extracts the segment hash from a segment shard key.
// Key format: "seg.<hash>.<shardIdx>.hrcrx"
// Returns empty string if the key doesn't match.
func ParseSegmentHash(key string) string {
	if !strings.HasPrefix(key, segmentKeyPrefix) || !strings.HasSuffix(key, segmentKeySuffix) {
		return ""
	}
	// Strip prefix and suffix
	inner := key[len(segmentKeyPrefix) : len(key)-len(segmentKeySuffix)]
	// Find last dot to split hash from shard index
	lastDot := strings.LastIndex(inner, ".")
	if lastDot < 0 {
		return ""
	}
	return inner[:lastDot]
}
