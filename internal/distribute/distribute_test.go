package distribute_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"testing"

	"horcrux/internal/config"
	"horcrux/internal/crypto"
	"horcrux/internal/distribute/segment"
	"horcrux/internal/shamir"
)

func TestSegmentEncryptDecryptRoundTrip(t *testing.T) {
	entries := []segment.SegmentEntry{
		{Type: 0, Name: "passes.hrcrx", Data: make([]byte, 100)},
		{Type: 0, Name: "totp.hrcrx", Data: make([]byte, 50)},
	}
	for i := range entries {
		rand.Read(entries[i].Data)
	}

	seg := segment.Pack(entries)
	if seg.Hash == "" {
		t.Fatal("segment hash is empty")
	}

	dek, _ := segment.GenerateDEK()

	ciphertext, err := seg.Encrypt(dek)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if bytes.Equal(ciphertext, seg.Plaintext) {
		t.Error("encrypted data should differ from plaintext")
	}

	restored, err := segment.DecryptAndUnpack(ciphertext, dek)
	if err != nil {
		t.Fatalf("DecryptAndUnpack: %v", err)
	}

	if len(restored.Entries) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(restored.Entries))
	}

	for i, entry := range restored.Entries {
		if !bytes.Equal(entry.Data, entries[i].Data) {
			t.Errorf("data mismatch for entry %d", i)
		}
	}
}

func TestSegmentWrongDEKFails(t *testing.T) {
	seg := segment.Pack([]segment.SegmentEntry{
		{Type: 0, Name: "test", Data: []byte("sensitive data")},
	})

	dek, _ := segment.GenerateDEK()
	ciphertext, _ := seg.Encrypt(dek)

	wrongDEK, _ := segment.GenerateDEK()
	_, err := segment.DecryptAndUnpack(ciphertext, wrongDEK)
	if err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}

func TestErasureEncodeDecodeRoundTrip(t *testing.T) {
	data := make([]byte, 1024)
	rand.Read(data)

	dataShards := 3
	parityShards := 2

	shards, err := segment.ErasureEncodeSegment(data, dataShards, parityShards)
	if err != nil {
		t.Fatalf("ErasureEncodeSegment: %v", err)
	}

	if len(shards) != dataShards+parityShards {
		t.Fatalf("expected %d shards, got %d", dataShards+parityShards, len(shards))
	}

	// Test recovery with exactly M shards
	recovered, err := segment.ErasureDecodeSegment(shards[:dataShards], dataShards, dataShards+parityShards)
	if err != nil {
		t.Fatalf("ErasureDecodeSegment: %v", err)
	}

	if !bytes.Equal(recovered, data) {
		t.Error("recovered data doesn't match original")
	}

	// Test recovery with some parity shards (simulating lost data shard)
	shardsWithLoss := make([][]byte, len(shards))
	copy(shardsWithLoss, shards)
	shardsWithLoss[1] = nil // simulate lost shard

	recovered2, err := segment.ErasureDecodeSegment(shardsWithLoss, dataShards, dataShards+parityShards)
	if err != nil {
		t.Fatalf("ErasureDecodeSegment with loss: %v", err)
	}

	if !bytes.Equal(recovered2, data) {
		t.Error("recovered data with lost shard doesn't match original")
	}
}

func TestFullPipeline(t *testing.T) {
	fmt.Println("\n=== Full Pipeline Test ===")

	// Setup test files
	originalData := map[string][]byte{
		"passes.hrcrx": make([]byte, 100),
		"totp.hrcrx":   make([]byte, 50),
	}
	for _, data := range originalData {
		rand.Read(data)
	}

	passphrase := "test-pipeline-passphrase"

	tmpPasses, _ := os.CreateTemp("", "passes-*")
	tmpTotp, _ := os.CreateTemp("", "totp-*")
	defer os.Remove(tmpPasses.Name())
	defer os.Remove(tmpTotp.Name())
	tmpPasses.Close()
	tmpTotp.Close()

	crypto.EncryptFile(tmpPasses.Name(), originalData["passes.hrcrx"], passphrase)
	crypto.EncryptFile(tmpTotp.Name(), originalData["totp.hrcrx"], passphrase)

	// Override config paths
	oldGlobal := config.Default()
	tmpCfg := config.New("")
	tmpCfg.PassesPath = tmpPasses.Name()
	tmpCfg.TotpPassPath = tmpTotp.Name()
	tmpCfg.ApiKeysPath = tmpPasses.Name() // reuse for missing file
	config.ResetForTest(tmpCfg)
	defer config.ResetForTest(oldGlobal)

	// --- Step 1: Pack vault into segments ---
	// Use a nil file store since we don't have file chunks
	packer := segment.NewPacker(nil)
	segments, err := packer.VaultToSegments(passphrase)
	if err != nil {
		t.Fatalf("VaultToSegments: %v", err)
	}
	fmt.Printf("  ✓ Packed vault into %d segments\n", len(segments))

	// --- Step 2: Encrypt and erasure-code each segment ---
	dek, _ := segment.GenerateDEK()
	n := 5
	m := 3

	type shardData struct {
		hash   string
		shards [][]byte
	}
	var segmentShards []shardData

	for _, seg := range segments {
		ciphertext, err := seg.Encrypt(dek)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}

		shards, err := segment.ErasureEncodeSegment(ciphertext, m, n-m)
		if err != nil {
			t.Fatalf("ErasureEncodeSegment: %v", err)
		}
		segmentShards = append(segmentShards, shardData{hash: seg.Hash, shards: shards})
	}
	fmt.Println("  ✓ Encrypted and erasure-coded all segments")

	// --- Step 3: Shamir-split the DEK ---
	dekShares, err := shamir.Split(dek, n, m)
	if err != nil {
		t.Fatalf("shamir.Split: %v", err)
	}
	fmt.Printf("  ✓ Split DEK into %d shares\n", n)

	// --- Step 4: Reconstruct DEK ---
	recoveredDEK, err := shamir.Combine(dekShares[:m])
	if err != nil {
		t.Fatalf("shamir.Combine: %v", err)
	}
	if !bytes.Equal(recoveredDEK, dek) {
		t.Fatal("recovered DEK doesn't match original")
	}
	fmt.Printf("  ✓ Reconstructed DEK from %d shares\n", m)

	// --- Step 5: Erasure-decode and decrypt each segment ---
	var restoredSegments []*segment.Segment
	for _, sd := range segmentShards {
		ciphertext, err := segment.ErasureDecodeSegment(sd.shards, m, n)
		if err != nil {
			t.Fatalf("ErasureDecodeSegment: %v", err)
		}

		seg, err := segment.DecryptAndUnpack(ciphertext, recoveredDEK)
		if err != nil {
			t.Fatalf("DecryptAndUnpack: %v", err)
		}
		restoredSegments = append(restoredSegments, seg)
	}
	fmt.Println("  ✓ Decoded and decrypted all segments")

	// --- Step 6: Verify ---
	if len(restoredSegments) != len(segments) {
		t.Fatalf("segment count mismatch: %d vs %d", len(restoredSegments), len(segments))
	}

	foundFiles := make(map[string]bool)
	for _, seg := range restoredSegments {
		for _, entry := range seg.Entries {
			foundFiles[entry.Name] = true
			expected, ok := originalData[entry.Name]
			if !ok {
				t.Errorf("unexpected file: %s", entry.Name)
				continue
			}
			if !bytes.Equal(entry.Data, expected) {
				t.Errorf("data mismatch for %s", entry.Name)
			}
		}
	}
	fmt.Println("  ✓ Verified all restored vault files")
	fmt.Println("  ✓ Pipeline test PASSED")
}
