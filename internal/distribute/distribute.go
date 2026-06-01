package distribute

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"horcrux/internal/config"
	"horcrux/internal/crypto"
	"horcrux/internal/distribute/manifest"
	"horcrux/internal/distribute/segment"
	"horcrux/internal/providers"
	"horcrux/internal/shamir"
	"horcrux/internal/vault/filestore"
)

// --- Public API ---

// Distribute uploads the vault to all configured providers incrementally.
// Only segments that have changed since the last distribution are uploaded.
// The manifest and DEK shares are always uploaded (they're tiny).
func Distribute(vaultPassphrase string) error {
	pf, err := providers.LoadConfig(vaultPassphrase)
	if err != nil {
		return fmt.Errorf("loading providers config: %w", err)
	}

	if len(pf.Providers) == 0 {
		return fmt.Errorf("no providers configured. Run 'horcrux providers auth <provider>' first")
	}

	provList, err := providers.BuildProviders(pf)
	if err != nil {
		return err
	}

	if err := providers.AuthenticateAll(provList); err != nil {
		return err
	}

	n := len(provList)
	m := providers.CalculateThreshold(n)
	ctx := context.Background()

	// --- 1. Load previous distribution state ---
	prevState := loadLocalState(vaultPassphrase)
	prevSegmentHashes := make(map[string]bool)
	if prevState != nil {
		for h := range prevState.SegmentHashes {
			prevSegmentHashes[h] = true
		}
	}

	// --- 2. Serialize vault into segments ---
	store := filestore.NewStore(config.FilesChunksDir())
	packer := segment.NewPacker(store)
	segments, err := packer.VaultToSegments(vaultPassphrase)
	if err != nil {
		return fmt.Errorf("packing vault into segments: %w", err)
	}
	fmt.Printf("  Packed vault into %d segments\n", len(segments))

	// --- 3. Identify new/changed segments ---
	var newSegments []*segment.Segment
	var unchangedCount int
	for _, seg := range segments {
		if prevSegmentHashes[seg.Hash] {
			unchangedCount++
		} else {
			newSegments = append(newSegments, seg)
		}
	}
	fmt.Printf("  %d segments unchanged, %d new/changed\n", unchangedCount, len(newSegments))

	// --- 4. Generate DEK, Shamir-split ---
	dek, err := segment.GenerateDEK()
	if err != nil {
		return err
	}
	dekShares, err := shamir.Split(dek, n, m)
	if err != nil {
		return fmt.Errorf("splitting encryption key: %w", err)
	}

	// --- 5. Encrypt, erasure-code, and upload NEW segments ---
	// Each provider gets ALL erasure shards so that any M providers can
	// reconstruct regardless of provider ordering or availability.
	for _, seg := range newSegments {
		ciphertext, err := seg.Encrypt(dek)
		if err != nil {
			return fmt.Errorf("encrypting segment %s: %w", seg.Hash[:16], err)
		}

		shards, err := segment.ErasureEncodeSegment(ciphertext, m, n-m)
		if err != nil {
			return fmt.Errorf("erasure coding segment %s: %w", seg.Hash[:16], err)
		}

		for shardIdx := 0; shardIdx < len(shards); shardIdx++ {
			key := manifest.SegmentShardKey(seg.Hash, shardIdx)
			encryptedShard, err := crypto.EncryptData(shards[shardIdx], vaultPassphrase)
			if err != nil {
				return fmt.Errorf("encrypting segment shard: %w", err)
			}
			for _, p := range provList {
				if err := p.Upload(ctx, key, encryptedShard); err != nil {
					return fmt.Errorf("uploading segment shard %d to %s: %w", shardIdx, p.Name(), err)
				}
			}
		}
		fmt.Printf("  ✓ Uploaded segment %s (%d bytes, %d shards) to %d providers\n",
			seg.Hash[:16], len(seg.Plaintext), len(shards), n)
	}

	// --- 6. Build and upload manifest ---
	nextVersion := uint64(1)
	if prevState != nil {
		nextVersion = prevState.LastVersion + 1
	}

	fileIndex, err := store.ExportIndex(vaultPassphrase)
	if err != nil {
		return fmt.Errorf("exporting file index: %w", err)
	}

	mf := manifest.NewManifest(nextVersion, segments, fileIndex)
	// Fill in shard counts
	for i := range mf.Segments {
		mf.Segments[i].ShardsN = n
	}

	for _, p := range provList {
		if err := manifest.UploadManifest(ctx, p, mf, vaultPassphrase); err != nil {
			return fmt.Errorf("uploading manifest to %s: %w", p.Name(), err)
		}
	}
	fmt.Printf("  ✓ Uploaded manifest v%d to %d providers\n", nextVersion, n)

	// --- 7. Upload DEK shares ---
	for i, p := range provList {
		shareIdx := i
		if shareIdx >= len(dekShares) {
			shareIdx = len(dekShares) - 1
		}
		if err := manifest.UploadDekshare(ctx, p, nextVersion, dekShares[shareIdx], vaultPassphrase); err != nil {
			return fmt.Errorf("uploading DEK share to %s: %w", p.Name(), err)
		}
	}
	fmt.Printf("  ✓ Uploaded %d DEK shares\n", n)

	// --- 8. Save local state ---
	segmentHashes := make(map[string]bool)
	for _, seg := range segments {
		segmentHashes[seg.Hash] = true
	}

	newState := &manifest.LocalState{
		LastVersion:   nextVersion,
		LastManifest:  mf,
		LastDEK:       dek,
		SegmentHashes: segmentHashes,
		FileIndex:     fileIndex,
	}
	if err := saveLocalState(newState, vaultPassphrase); err != nil {
		fmt.Printf("  ⚠ Warning: could not save local distribution state: %v\n", err)
	}

	fmt.Printf("  ✓ Distribution complete (version %d)\n", nextVersion)
	return nil
}

// Restore downloads the latest vault distribution from providers and
// reconstructs all vault files locally.
func Restore(vaultPassphrase string) error {
	pf, err := providers.LoadConfig(vaultPassphrase)
	if err != nil {
		return fmt.Errorf("loading providers config: %w", err)
	}

	if len(pf.Providers) == 0 {
		return fmt.Errorf("no providers configured")
	}

	provList, err := providers.BuildProviders(pf)
	if err != nil {
		return err
	}

	if err := providers.AuthenticateAll(provList); err != nil {
		return err
	}

	n := len(provList)
	m := providers.CalculateThreshold(n)
	ctx := context.Background()

	// --- 1. Find latest manifest version ---
	version, err := manifest.FindLatestVersion(ctx, provList)
	if err != nil {
		return fmt.Errorf("finding latest manifest: %w", err)
	}
	if version == 0 {
		return fmt.Errorf("no distributed vault found on any provider")
	}
	fmt.Printf("  Found distribution version %d\n", version)

	// --- 2. Download manifest ---
	mf, err := manifest.DownloadManifestFromAny(ctx, provList, version, vaultPassphrase)
	if err != nil {
		return fmt.Errorf("downloading manifest: %w", err)
	}
	fmt.Printf("  Manifest: %d segments, %d files\n", len(mf.Segments), len(mf.FileIndex))

	// --- 3. Collect DEK shares, reconstruct DEK ---
	dekShares, err := manifest.CollectDekshares(ctx, provList, version, m, vaultPassphrase)
	if err != nil {
		return fmt.Errorf("collecting DEK shares: %w", err)
	}
	dek, err := shamir.Combine(dekShares)
	if err != nil {
		return fmt.Errorf("reconstructing encryption key: %w", err)
	}
	fmt.Printf("  Reconstructed DEK from %d shares\n", len(dekShares))

	// --- 4. Download and decrypt each segment ---
	// All providers have all shards, so we just need M shards from any provider(s).
	var restoredSegments []*segment.Segment
	for _, ref := range mf.Segments {
		shards := make([][]byte, n)
		collected := 0
		// For each shard index, search all providers until found
		for shardIdx := 0; shardIdx < n; shardIdx++ {
			if collected >= m {
				break // have enough to reconstruct
			}
			key := manifest.SegmentShardKey(ref.Hash, shardIdx)
			for _, p := range provList {
				encrypted, err := p.Download(ctx, key)
				if err != nil {
					continue
				}
				shard, err := crypto.DecryptData(encrypted, vaultPassphrase)
				if err != nil {
					continue
				}
				shards[shardIdx] = shard
				collected++
				break // got this shard, move to next index
			}
		}
		if collected < m {
			return fmt.Errorf("could not collect enough shards for segment %s (got %d, need %d)",
				ref.Hash[:16], collected, m)
		}

		ciphertext, err := segment.ErasureDecodeSegment(shards, m, n)
		if err != nil {
			return fmt.Errorf("erasure decoding segment %s: %w", ref.Hash[:16], err)
		}

		seg, err := segment.DecryptAndUnpack(ciphertext, dek)
		if err != nil {
			return fmt.Errorf("decrypting segment %s: %w", ref.Hash[:16], err)
		}
		restoredSegments = append(restoredSegments, seg)
		fmt.Printf("  ✓ Restored segment %s (%d entries)\n", ref.Hash[:16], len(seg.Entries))
	}

	// --- 5. Reconstruct vault ---
	// Ensure the chunked store directory exists
	os.MkdirAll(config.FilesChunksDir(), 0700)
	store := filestore.NewStore(config.FilesChunksDir())
	packer := segment.NewPacker(store)

	if err := packer.SegmentsToVault(restoredSegments, vaultPassphrase); err != nil {
		return fmt.Errorf("reconstructing vault from segments: %w", err)
	}
	if err := packer.RebuildFileIndex(restoredSegments, vaultPassphrase); err != nil {
		return fmt.Errorf("rebuilding file index: %w", err)
	}

	fmt.Println("  ✓ Vault restored successfully")
	return nil
}

// GC removes unreferenced segments and old manifests from all providers.
// Keeps the latest keepVersions manifests and all their referenced segments.
func GC(vaultPassphrase string, keepVersions int) error {
	pf, err := providers.LoadConfig(vaultPassphrase)
	if err != nil {
		return fmt.Errorf("loading providers config: %w", err)
	}

	if len(pf.Providers) == 0 {
		return fmt.Errorf("no providers configured")
	}

	provList, err := providers.BuildProviders(pf)
	if err != nil {
		return err
	}

	if err := providers.AuthenticateAll(provList); err != nil {
		return err
	}

	ctx := context.Background()

	// --- 1. Find all manifest versions ---
	allVersions := make(map[uint64]bool)
	for _, p := range provList {
		keys, err := p.List(ctx, manifest.ManifestKeyPrefix())
		if err != nil {
			continue
		}
		for _, key := range keys {
			v := manifest.ParseManifestVersion(key)
			if v > 0 {
				allVersions[v] = true
			}
		}
	}

	// --- 2. Sort versions, keep the latest keepVersions ---
	sorted := make([]uint64, 0, len(allVersions))
	for v := range allVersions {
		sorted = append(sorted, v)
	}
	// Simple descending sort
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] > sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	keep := make(map[uint64]bool)
	for i := 0; i < len(sorted) && i < keepVersions; i++ {
		keep[sorted[i]] = true
	}

	// --- 3. Collect all segment hashes referenced by kept manifests ---
	referencedSegments := make(map[string]bool)
	for _, p := range provList {
		for v := range keep {
			mf, err := manifest.DownloadManifest(ctx, p, v, vaultPassphrase)
			if err != nil {
				continue
			}
			for h := range mf.CollectSegmentHashes() {
				referencedSegments[h] = true
			}
			break // one provider is enough for the manifest
		}
	}

	// --- 4. Delete unreferenced objects ---
	totalDeleted := 0
	for _, p := range provList {
		keys, err := p.List(ctx, "")
		if err != nil {
			fmt.Printf("  ⚠ Could not list objects on %s: %v\n", p.Name(), err)
			continue
		}

		for _, key := range keys {
			shouldDelete := false

			// Old manifest versions
			if v := manifest.ParseManifestVersion(key); v > 0 && !keep[v] {
				shouldDelete = true
			}

			// Old DEK shares
			if v := manifest.ParseDekshareVersion(key); v > 0 && !keep[v] {
				shouldDelete = true
			}

			// Unreferenced segments
			if hash := manifest.ParseSegmentHash(key); hash != "" && !referencedSegments[hash] {
				shouldDelete = true
			}

			if shouldDelete {
				if err := p.Delete(ctx, key); err != nil {
					fmt.Printf("  ⚠ Failed to delete %s from %s: %v\n", key, p.Name(), err)
				} else {
					totalDeleted++
				}
			}
		}
	}

	fmt.Printf("  ✓ GC complete: deleted %d unreferenced objects\n", totalDeleted)
	return nil
}

// --- Local state persistence ---

func localStatePath() string {
	return config.Default().DataDir + "/distribution-state.json"
}

func loadLocalState(vaultPassphrase string) *manifest.LocalState {
	data, err := os.ReadFile(localStatePath())
	if err != nil {
		return nil
	}
	// Try plain JSON first
	var ls manifest.LocalState
	if json.Unmarshal(data, &ls) == nil {
		return &ls
	}
	// Try encrypted
	plaintext, err := crypto.DecryptData(data, vaultPassphrase)
	if err != nil {
		return nil
	}
	ls2, err := manifest.UnmarshalLocalState(plaintext)
	if err != nil {
		return nil
	}
	return ls2
}

func saveLocalState(ls *manifest.LocalState, vaultPassphrase string) error {
	plaintext, err := ls.Marshal()
	if err != nil {
		return err
	}
	encrypted, err := crypto.EncryptData(plaintext, vaultPassphrase)
	if err != nil {
		return err
	}
	return os.WriteFile(localStatePath(), encrypted, 0600)
}
