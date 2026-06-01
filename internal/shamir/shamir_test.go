package shamir_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"horcrux/internal/shamir"
)

func TestGF256MulDiv(t *testing.T) {
	for a := 0; a < 256; a++ {
		for b := 0; b < 256; b++ {
			result := shamir.GFMul(byte(a), byte(b))
			if a == 0 || b == 0 {
				if result != 0 {
					t.Errorf("GFMul(%d,%d) = %d, expected 0", a, b, result)
				}
			} else {
				if result == 0 {
					t.Errorf("GFMul(%d,%d) = 0, expected non-zero", a, b)
				}
			}
		}
	}
}

func TestGF256Inverse(t *testing.T) {
	for a := 1; a < 256; a++ {
		inv := shamir.GFInv(byte(a))
		if inv == 0 {
			t.Errorf("GFInv(%d) = 0", a)
		}
		product := shamir.GFMul(byte(a), inv)
		if product != 1 {
			t.Errorf("GFMul(%d, GFInv(%d)) = %d, expected 1", a, a, product)
		}
	}
}

func TestGF256DivIsMulInv(t *testing.T) {
	for a := 1; a < 256; a++ {
		for b := 1; b < 256; b++ {
			ab := shamir.GFMul(byte(a), byte(b))
			result := shamir.GFDiv(ab, byte(b))
			if result != byte(a) {
				t.Errorf("GFDiv(GFMul(%d,%d), %d) = %d, expected %d", a, b, b, result, a)
			}
		}
	}
}

func TestShamirSplitCombine3of2(t *testing.T) {
	secret := make([]byte, 32)
	rand.Read(secret)

	shares, err := shamir.Split(secret, 3, 2)
	if err != nil {
		t.Fatal(err)
	}

	if len(shares) != 3 {
		t.Fatalf("expected 3 shares, got %d", len(shares))
	}

	for _, share := range shares {
		if len(share) != 33 {
			t.Fatalf("expected share length 33, got %d", len(share))
		}
	}

	combinations := [][][]byte{
		{shares[0], shares[1]},
		{shares[0], shares[2]},
		{shares[1], shares[2]},
		{shares[0], shares[1], shares[2]},
	}

	for i, combo := range combinations {
		recovered, err := shamir.Combine(combo)
		if err != nil {
			t.Fatalf("combine combo %d: %v", i, err)
		}
		if !bytes.Equal(recovered, secret) {
			t.Errorf("combo %d: recovered %x != secret %x", i, recovered, secret)
		}
	}
}

func TestShamirSplitCombine5of3(t *testing.T) {
	secret := make([]byte, 64)
	rand.Read(secret)

	shares, err := shamir.Split(secret, 5, 3)
	if err != nil {
		t.Fatal(err)
	}

	combinations := [][][]byte{
		{shares[0], shares[1], shares[2]},
		{shares[0], shares[3], shares[4]},
		{shares[1], shares[2], shares[4]},
		{shares[0], shares[1], shares[2], shares[3]},
	}

	for i, combo := range combinations {
		recovered, err := shamir.Combine(combo)
		if err != nil {
			t.Fatalf("combine combo %d: %v", i, err)
		}
		if !bytes.Equal(recovered, secret) {
			t.Errorf("combo %d: mismatch", i)
		}
	}
}

func TestShamirWrongSharesDontMatch(t *testing.T) {
	secret := []byte{0x42, 0x37, 0xFF}
	shares, _ := shamir.Split(secret, 3, 2)

	shares2, _ := shamir.Split(secret, 3, 2)

	recovered, _ := shamir.Combine([][]byte{shares[0], shares2[1]})
	if bytes.Equal(recovered, secret) {
		t.Error("mixing shares from different splits should not recover secret (extremely unlikely)")
	}
}

func TestEncryptDecryptShare(t *testing.T) {
	share := make([]byte, 33)
	rand.Read(share)

	passphrase := "test-passphrase-12345"

	encrypted, err := shamir.EncryptShare(share, passphrase)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(encrypted, share) {
		t.Error("encrypted share should differ from original")
	}

	decrypted, err := shamir.DecryptShare(encrypted, passphrase)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decrypted, share) {
		t.Error("decrypted share doesn't match original")
	}
}

func TestDecryptShareWrongPassphrase(t *testing.T) {
	share := make([]byte, 33)
	rand.Read(share)

	encrypted, _ := shamir.EncryptShare(share, "correct-passphrase")
	_, err := shamir.DecryptShare(encrypted, "wrong-passphrase")
	if err == nil {
		t.Error("expected error with wrong passphrase")
	}
}
