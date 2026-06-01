package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/crypto/argon2"
)

var envelopeMagic = []byte{'H', 'C', 'R', 'X', 1}

const (
	argonTime    = uint32(3)
	argonMemory  = uint32(64 * 1024)
	argonThreads = uint8(4)
	argonKeyLen  = uint32(32)
	saltLen      = 16
)

func EncryptData(plaintext []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	encrypted := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(envelopeMagic)+4+4+1+4+1+len(salt)+1+len(nonce)+len(encrypted))
	out = append(out, envelopeMagic...)
	out = binary.BigEndian.AppendUint32(out, argonTime)
	out = binary.BigEndian.AppendUint32(out, argonMemory)
	out = append(out, argonThreads)
	out = binary.BigEndian.AppendUint32(out, argonKeyLen)
	out = append(out, byte(len(salt)))
	out = append(out, salt...)
	out = append(out, byte(len(nonce)))
	out = append(out, nonce...)
	out = append(out, encrypted...)
	return out, nil
}

func DecryptData(ciphertext []byte, passphrase string) ([]byte, error) {
	if !hasEnvelopeMagic(ciphertext) {
		return nil, fmt.Errorf("not a Horcrux vault file (missing magic header)")
	}
	return decryptArgonEnvelope(ciphertext, passphrase)
}

func hasEnvelopeMagic(ciphertext []byte) bool {
	return len(ciphertext) >= len(envelopeMagic) && string(ciphertext[:len(envelopeMagic)]) == string(envelopeMagic)
}

func decryptArgonEnvelope(ciphertext []byte, passphrase string) ([]byte, error) {
	pos := len(envelopeMagic)
	if len(ciphertext) < pos+4+4+1+4+1 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	timeCost := binary.BigEndian.Uint32(ciphertext[pos : pos+4])
	pos += 4
	memory := binary.BigEndian.Uint32(ciphertext[pos : pos+4])
	pos += 4
	threads := ciphertext[pos]
	pos++
	keyLen := binary.BigEndian.Uint32(ciphertext[pos : pos+4])
	pos += 4
	sLen := int(ciphertext[pos])
	pos++
	if sLen < 16 || len(ciphertext) < pos+sLen+1 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	salt := ciphertext[pos : pos+sLen]
	pos += sLen
	nonceLen := int(ciphertext[pos])
	pos++
	if timeCost == 0 || timeCost > 10 || memory < 19*1024 || memory > 1024*1024 || threads == 0 || threads > 16 || keyLen < 16 || keyLen > 64 {
		return nil, fmt.Errorf("invalid encryption parameters")
	}
	if nonceLen == 0 || keyLen == 0 || len(ciphertext) < pos+nonceLen {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := ciphertext[pos : pos+nonceLen]
	ct := ciphertext[pos+nonceLen:]
	key := argon2.IDKey([]byte(passphrase), salt, timeCost, memory, threads, keyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if nonceLen != gcm.NonceSize() || len(ct) < gcm.Overhead() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, nonce, ct, nil)
}

func EncryptBSONFile(filePath string, data map[string]map[string]string, passphrase string) error {
	bsonData, err := bson.Marshal(data)
	if err != nil {
		return err
	}
	encrypted, err := EncryptData(bsonData, passphrase)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, encrypted, 0600)
}

func DecryptBSONFile(filePath string, passphrase string) (map[string]map[string]string, error) {
	encrypted, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	decrypted, err := DecryptData(encrypted, passphrase)
	if err != nil {
		return nil, err
	}
	var result map[string]map[string]string
	if err := bson.Unmarshal(decrypted, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}

func EncryptFile(filePath string, data []byte, passphrase string) error {
	encrypted, err := EncryptData(data, passphrase)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, encrypted, 0600)
}

func DecryptFile(filePath string, passphrase string) ([]byte, error) {
	encrypted, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return DecryptData(encrypted, passphrase)
}
