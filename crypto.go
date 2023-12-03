package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/crypto/pbkdf2"
)

func EncryptBSONFile(filePath string, data map[string]map[string]string, passphrase string) error {
    bsonData, err := bson.Marshal(data)
    if err != nil {
        return err
    }
    salt := make([]byte, 8)
    _, err = io.ReadFull(rand.Reader, salt)
    if err != nil {
        return err
    }
    key := pbkdf2.Key([]byte(passphrase), salt, 4096, 32, sha256.New)
    block, err := aes.NewCipher(key)
    if err != nil {
        return err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return err
    }
    nonce := make([]byte, gcm.NonceSize())
    _, err = io.ReadFull(rand.Reader, nonce)
    if err != nil {
        return err
    }
    encrypted := gcm.Seal(nil, nonce, bsonData, nil)
    fullData := append(salt, nonce...)
    fullData = append(fullData, encrypted...)
    return os.WriteFile(filePath, fullData, 0644)
}

func DecryptBSONFile(filePath string, passphrase string) (map[string]map[string]string, error) {
    encrypted, err := os.ReadFile(filePath)
    if err != nil {
        return nil, err
    }
    salt := encrypted[:8]
    key := pbkdf2.Key([]byte(passphrase), salt, 4096, 32, sha256.New)
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    nonceSize := gcm.NonceSize()
    if len(encrypted) < nonceSize+8 {
        return nil, fmt.Errorf("ciphertext too short")
    }
    nonce := encrypted[8 : 8+nonceSize]
    ciphertext := encrypted[8+nonceSize:]
    decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, err
    }
    var result map[string]map[string]string
    if err := bson.Unmarshal(decrypted, &result); err != nil {
        return nil, err
    }
    return result, nil
}