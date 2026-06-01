package crypto_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"horcrux/internal/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	data := make([]byte, 200)
	rand.Read(data)

	passphrase := "my-secret-passphrase"

	encrypted, err := crypto.EncryptData(data, passphrase)
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := crypto.DecryptData(encrypted, passphrase)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decrypted, data) {
		t.Error("decrypted data doesn't match original")
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	data := []byte("sensitive data")
	encrypted, _ := crypto.EncryptData(data, "correct")
	_, err := crypto.DecryptData(encrypted, "wrong")
	if err == nil {
		t.Error("expected error with wrong passphrase")
	}
}


