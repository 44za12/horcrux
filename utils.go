package main

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generateRandomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return strings.TrimSpace(string(b))
}

func getPassphraseInput(prompt string) string {
    fmt.Print(prompt)
    passphraseBytes, err := terminal.ReadPassword(0)
    if err != nil {
        log.Fatal("Failed to read passphrase")
    }
    fmt.Println()
    return string(passphraseBytes)
}

