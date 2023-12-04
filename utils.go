package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var (
	datadir = getDefaultDataPath()
	passespath = fmt.Sprintf("%s/passes.hrcrx", datadir)
	mainpasspath = fmt.Sprintf("%s/mainpass.hrcrx", datadir)
	totppasspath = fmt.Sprintf("%s/totp.hrcrx", datadir)
)

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

func getDefaultDataPath() string {
    var dataDir string
    if runtime.GOOS == "windows" {
        appDataDir, err := os.UserHomeDir()
        if err != nil {
            panic(err)
        }
        dataDir = filepath.Join(appDataDir, "AppData", "Local", "Horcrux")
    } else {
        homeDir, err := os.UserHomeDir()
        if err != nil {
            panic(err)
        }
        dataDir = filepath.Join(homeDir, ".horcrux")
    }
    err := os.MkdirAll(dataDir, 0700)
    if err != nil {
        panic(err)
    }
    return dataDir
}