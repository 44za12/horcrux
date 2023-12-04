package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

type TOTPService struct {
    UpdatedAt int64  `json:"updatedAt"`
    Name      string `json:"name"`
    Secret    string `json:"secret"`
    OTP       struct {
        Source   string `json:"source"`
        Period   int    `json:"period"`
        Algorithm string `json:"algorithm"`
        Digits   int    `json:"digits"`
        Counter  int    `json:"counter"`
        TokenType string `json:"tokenType"`
        Account  string `json:"account"`
    } `json:"otp"`
}

type TOTPImport struct {
    SchemaVersion  int           `json:"schemaVersion"`
    AppVersionName string        `json:"appVersionName"`
    Services       []TOTPService `json:"services"`
}


func generateTOTP(secretKey string) int {
	timeStep := int64(30)
	position := time.Now()
    key, err := base32.StdEncoding.DecodeString(strings.ToUpper(secretKey))
    if err != nil {
        fmt.Printf("Error generating totp: %s", err)
    }
    counter := position.Unix() / timeStep
    counterBytes := make([]byte, 8)
    binary.BigEndian.PutUint64(counterBytes, uint64(counter))
    hmacSha1 := hmac.New(sha1.New, key)
    hmacSha1.Write(counterBytes)
    hash := hmacSha1.Sum(nil)
    offset := hash[len(hash)-1] & 0x0f
    truncatedHash := int(binary.BigEndian.Uint32(hash[offset:offset+4])) & 0x7fffffff
    totp := truncatedHash % 1000000
    return totp
}

func addUpdateTOTP(service string, secretKey string, passphrase string) {
	decryptedData, err := DecryptBSONFile(totppasspath, passphrase)
    if err != nil {
        panic(err)
    }
	_, ok := decryptedData["totp"]
	if ok {
		decryptedData["totp"][service] = secretKey
	} else {
		decryptedData["totp"] = map[string]string{}
		decryptedData["totp"][service] = secretKey
	}
	err = EncryptBSONFile(totppasspath, decryptedData, passphrase)
	if err != nil {
        panic(err)
    }
}

func removeTOTP(service string, passphrase string) {
	decryptedData, err := DecryptBSONFile(totppasspath, passphrase)
    if err != nil {
        panic(err)
    }
	_, ok := decryptedData["totp"][service]
	if ok {
		delete(decryptedData["totp"], service)
	}
	err = EncryptBSONFile(totppasspath, decryptedData, passphrase)
	if err != nil {
        panic(err)
    }
}

func getTOTP(service string, passphrase string) int {
	decryptedData, err := DecryptBSONFile(totppasspath, passphrase)
    if err != nil {
        panic(err)
    }
	secret, ok := decryptedData["totp"][service]
	if !ok {
		panic("There are no TOTP configured for this service.")
	}
	return generateTOTP(secret)
}