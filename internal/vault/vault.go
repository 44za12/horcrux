package vault

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"horcrux/internal/config"
	"horcrux/internal/crypto"
	"horcrux/internal/vault/filestore"

	"golang.org/x/crypto/argon2"
)

type passwordValue struct {
	P string `json:"p"`
	N string `json:"n"`
}

func packValue(password, notes string) string {
	if notes == "" {
		return password
	}
	b, _ := json.Marshal(passwordValue{P: password, N: notes})
	return string(b)
}

func unpackValue(raw string) (string, string) {
	if len(raw) > 0 && raw[0] == '{' {
		var pv passwordValue
		if json.Unmarshal([]byte(raw), &pv) == nil && pv.P != "" {
			return pv.P, pv.N
		}
	}
	return raw, ""
}

type PasswordEntry struct {
	Site     string `json:"Site"`
	Username string `json:"Username"`
	Password string `json:"Password"`
	Notes    string `json:"Notes"`
}

type TotpService struct {
	Name   string `json:"Name"`
	Secret string `json:"Secret"`
}

// VerificationData stores the passphrase verification hash using Argon2id.
type VerificationData struct {
	Version int    `json:"version"`
	Salt    []byte `json:"salt"`
	Digest  []byte `json:"digest"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"key_len"`
}

// Argon2id verification parameters — fast enough for UI unlock, still memory-hard.
const (
	verifyTime    = uint32(1)
	verifyMemory  = uint32(32 * 1024) // 32 MB
	verifyThreads = uint8(4)
	verifyKeyLen  = uint32(32)
)

func AddUpdatePassword(site, username, password, notes, passphrase string) error {
	decryptedData, err := decryptOrCreate(config.PassesPath(), passphrase)
	if err != nil {
		return fmt.Errorf("decrypting passwords: %w", err)
	}
	if decryptedData == nil {
		decryptedData = make(map[string]map[string]string)
	}
	if _, ok := decryptedData[site]; !ok {
		decryptedData[site] = map[string]string{}
	}
	decryptedData[site][username] = packValue(password, notes)
	return crypto.EncryptBSONFile(config.PassesPath(), decryptedData, passphrase)
}

func AddUpdatePasswordOnly(site, username, password, passphrase string) error {
	return AddUpdatePassword(site, username, password, "", passphrase)
}

func RemovePassword(site string, username string, passphrase string) error {
	decryptedData, err := decryptOrCreate(config.PassesPath(), passphrase)
	if err != nil {
		return fmt.Errorf("decrypting passwords: %w", err)
	}
	if _, ok := decryptedData[site]; ok {
		delete(decryptedData[site], username)
	}
	return crypto.EncryptBSONFile(config.PassesPath(), decryptedData, passphrase)
}

func GetPassword(site string, username string, passphrase string) (string, string, error) {
	decryptedData, err := decryptOrCreate(config.PassesPath(), passphrase)
	if err != nil {
		return "", "", fmt.Errorf("decrypting passwords: %w", err)
	}
	if _, ok := decryptedData[site]; !ok {
		return "", "", fmt.Errorf("site '%s' not found", site)
	}
	raw, ok := decryptedData[site][username]
	if !ok {
		return "", "", fmt.Errorf("no password for username '%s' at site '%s'", username, site)
	}
	pass, notes := unpackValue(raw)
	return pass, notes, nil
}

func ListPasswords(passphrase string) ([]PasswordEntry, error) {
	decryptedData, err := decryptOrCreate(config.PassesPath(), passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypting passwords: %w", err)
	}
	var entries []PasswordEntry
	for site, users := range decryptedData {
		for username, raw := range users {
			_, notes := unpackValue(raw)
			entries = append(entries, PasswordEntry{
				Site:     site,
				Username: username,
				Password: "",
				Notes:    notes,
			})
		}
	}
	return entries, nil
}

func SearchPasswords(passphrase string, query string) ([]PasswordEntry, error) {
	entries, err := ListPasswords(passphrase)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	var results []PasswordEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Site), query) ||
			strings.Contains(strings.ToLower(e.Username), query) ||
			strings.Contains(strings.ToLower(e.Notes), query) {
			results = append(results, e)
		}
	}
	return results, nil
}

func GenerateTOTP(secretKey string) (int, error) {
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secretKey)))
	if err != nil {
		return 0, fmt.Errorf("decoding TOTP secret: %w", err)
	}
	key := decoded
	timeStep := int64(30)
	counter := time.Now().Unix() / timeStep
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, uint64(counter))
	hmacSha1 := hmac.New(sha1.New, key)
	hmacSha1.Write(counterBytes)
	hash := hmacSha1.Sum(nil)
	offset := hash[len(hash)-1] & 0x0f
	truncatedHash := int(binary.BigEndian.Uint32(hash[offset:offset+4])) & 0x7fffffff
	totp := truncatedHash % 1000000
	return totp, nil
}

func GetTotpSecondsRemaining() int {
	return 30 - int(time.Now().Unix()%30)
}

func AddUpdateTOTP(service string, secretKey string, passphrase string) error {
	decryptedData, err := crypto.DecryptBSONFile(config.TotpPassPath(), passphrase)
	if err != nil {
		return fmt.Errorf("decrypting TOTP: %w", err)
	}
	if _, ok := decryptedData["totp"]; ok {
		decryptedData["totp"][service] = secretKey
	} else {
		decryptedData["totp"] = map[string]string{}
		decryptedData["totp"][service] = secretKey
	}
	return crypto.EncryptBSONFile(config.TotpPassPath(), decryptedData, passphrase)
}

func RemoveTOTP(service string, passphrase string) error {
	decryptedData, err := crypto.DecryptBSONFile(config.TotpPassPath(), passphrase)
	if err != nil {
		return fmt.Errorf("decrypting TOTP: %w", err)
	}
	if _, ok := decryptedData["totp"][service]; ok {
		delete(decryptedData["totp"], service)
	}
	return crypto.EncryptBSONFile(config.TotpPassPath(), decryptedData, passphrase)
}

func GetTOTP(service string, passphrase string) (int, error) {
	decryptedData, err := crypto.DecryptBSONFile(config.TotpPassPath(), passphrase)
	if err != nil {
		return 0, fmt.Errorf("decrypting TOTP: %w", err)
	}
	secret, ok := decryptedData["totp"][service]
	if !ok {
		return 0, fmt.Errorf("no TOTP configured for service '%s'", service)
	}
	return GenerateTOTP(secret)
}

func ListTOTPServices(passphrase string) ([]TotpService, error) {
	decryptedData, err := crypto.DecryptBSONFile(config.TotpPassPath(), passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypting TOTP: %w", err)
	}
	var services []TotpService
	totpMap, ok := decryptedData["totp"]
	if !ok {
		return services, nil
	}
	for name, secret := range totpMap {
		services = append(services, TotpService{Name: name, Secret: ""})
		_ = secret
	}
	return services, nil
}

func InitBSONFiles(passphrase string) (string, error) {
	data := make(map[string]map[string]string)
	if err := storeVerification(passphrase); err != nil {
		return "", err
	}
	if err := crypto.EncryptBSONFile(config.PassesPath(), data, passphrase); err != nil {
		return "", err
	}
	recoveryString, err := generateRandomString(20)
	if err != nil {
		return "", err
	}
	if err := crypto.EncryptBSONFile(config.TotpPassPath(), data, passphrase); err != nil {
		return "", err
	}
	if err := crypto.EncryptBSONFile(config.ApiKeysPath(), data, passphrase); err != nil {
		return "", err
	}
	return recoveryString, crypto.EncryptBSONFile(config.FilesPath(), data, passphrase)
}

// ChangePassphrase re-encrypts all vault files with a new passphrase.
// The old passphrase must be valid (caller should verify first).
// Returns an error if any vault file cannot be re-encrypted.
func ChangePassphrase(oldPassphrase, newPassphrase string) error {
	// Re-encrypt BSON vault files (passes, totp, apikeys, files)
	bsonFiles := []string{config.PassesPath(), config.TotpPassPath(), config.ApiKeysPath(), config.FilesPath()}
	for _, path := range bsonFiles {
		data, err := crypto.DecryptBSONFile(path, oldPassphrase)
		if err != nil {
			return fmt.Errorf("decrypting %s: %w", path, err)
		}
		if err := crypto.EncryptBSONFile(path, data, newPassphrase); err != nil {
			return fmt.Errorf("re-encrypting %s: %w", path, err)
		}
	}

	// Re-encrypt providers file (JSON, not BSON)
	provData, err := os.ReadFile(config.ProvidersPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading providers: %w", err)
	}
	if err == nil {
		plaintext, err := crypto.DecryptData(provData, oldPassphrase)
		if err != nil {
			return fmt.Errorf("decrypting providers: %w", err)
		}
		encrypted, err := crypto.EncryptData(plaintext, newPassphrase)
		if err != nil {
			return fmt.Errorf("re-encrypting providers: %w", err)
		}
		if err := os.WriteFile(config.ProvidersPath(), encrypted, 0600); err != nil {
			return fmt.Errorf("writing providers: %w", err)
		}
	}

	// Update verification hash
	return storeVerification(newPassphrase)
}

func storeVerification(passphrase string) error {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	// Argon2id — memory-hard, harder to brute-force than PBKDF2
	digest := argon2.IDKey([]byte(passphrase), salt, verifyTime, verifyMemory, verifyThreads, verifyKeyLen)
	vd := VerificationData{
		Version: 2,
		Salt:    salt,
		Digest:  digest,
		Time:    verifyTime,
		Memory:  verifyMemory,
		Threads: verifyThreads,
		KeyLen:  verifyKeyLen,
	}
	data, err := json.Marshal(vd)
	if err != nil {
		return err
	}
	return os.WriteFile(config.MainPassPath(), data, 0600)
}

func VerifyPassphrase(passphrase string) bool {
	data, err := os.ReadFile(config.MainPassPath())
	if err != nil {
		return false
	}
	var vd VerificationData
	if err := json.Unmarshal(data, &vd); err != nil {
		return false
	}
	if len(vd.Salt) == 0 || len(vd.Digest) == 0 || vd.Version < 2 {
		return false
	}
	digest := argon2.IDKey([]byte(passphrase), vd.Salt, vd.Time, vd.Memory, vd.Threads, vd.KeyLen)
	return hmac.Equal(digest, vd.Digest)
}

func MigrateVerification(passphrase string) error {
	data, err := os.ReadFile(config.MainPassPath())
	if err != nil {
		return err
	}
	var vd VerificationData
	if json.Unmarshal(data, &vd) != nil || vd.Version < 2 {
		// Unreadable, corrupt, or pre-v2 format — rewrite with current format
		return storeVerification(passphrase)
	}
	return nil
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generateRandomString(length int) (string, error) {
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

type ApiKeyEntry struct {
	Service string `json:"Service"`
	Name    string `json:"Name"`
	Key     string `json:"Key"`
	Notes   string `json:"Notes"`
}

func decryptOrCreate(path, passphrase string) (map[string]map[string]string, error) {
	data, err := crypto.DecryptBSONFile(path, passphrase)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]map[string]string), nil
		}
		return nil, err
	}
	return data, nil
}

func AddUpdateApiKey(service, name, key, notes, passphrase string) error {
	decryptedData, err := decryptOrCreate(config.ApiKeysPath(), passphrase)
	if err != nil {
		return fmt.Errorf("decrypting API keys: %w", err)
	}
	if decryptedData == nil {
		decryptedData = make(map[string]map[string]string)
	}
	if _, ok := decryptedData[service]; !ok {
		decryptedData[service] = map[string]string{}
	}
	decryptedData[service][name] = packValue(key, notes)
	return crypto.EncryptBSONFile(config.ApiKeysPath(), decryptedData, passphrase)
}

func RemoveApiKey(service, name, passphrase string) error {
	decryptedData, err := decryptOrCreate(config.ApiKeysPath(), passphrase)
	if err != nil {
		return fmt.Errorf("decrypting API keys: %w", err)
	}
	if users, ok := decryptedData[service]; ok {
		delete(users, name)
		if len(users) == 0 {
			delete(decryptedData, service)
		}
	}
	return crypto.EncryptBSONFile(config.ApiKeysPath(), decryptedData, passphrase)
}

func GetApiKey(service, name, passphrase string) (string, string, error) {
	decryptedData, err := decryptOrCreate(config.ApiKeysPath(), passphrase)
	if err != nil {
		return "", "", fmt.Errorf("decrypting API keys: %w", err)
	}
	if _, ok := decryptedData[service]; !ok {
		return "", "", fmt.Errorf("service '%s' not found", service)
	}
	raw, ok := decryptedData[service][name]
	if !ok {
		return "", "", fmt.Errorf("key '%s' not found for service '%s'", name, service)
	}
	key, notes := unpackValue(raw)
	return key, notes, nil
}

func ListApiKeys(passphrase string) ([]ApiKeyEntry, error) {
	decryptedData, err := decryptOrCreate(config.ApiKeysPath(), passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypting API keys: %w", err)
	}
	var entries []ApiKeyEntry
	for service, keys := range decryptedData {
		for name, raw := range keys {
			_, notes := unpackValue(raw)
			entries = append(entries, ApiKeyEntry{
				Service: service,
				Name:    name,
				Key:     "",
				Notes:   notes,
			})
		}
	}
	return entries, nil
}

type FileEntry struct {
	Name     string `json:"Name"`
	MimeType string `json:"MimeType"`
	Size     int64  `json:"Size"`
}

// getFileStore returns a filestore.Store for the vault's chunked file store.
func getFileStore() *filestore.Store {
	return filestore.NewStore(config.FilesChunksDir())
}

func AddFile(filename, mimeType string, content []byte, passphrase string) error {
	return getFileStore().AddFile(filename, mimeType, content, passphrase)
}

func RemoveFile(filename, passphrase string) error {
	return getFileStore().RemoveFile(filename, passphrase)
}

func GetFile(filename, passphrase string) ([]byte, string, error) {
	return getFileStore().GetFile(filename, passphrase)
}

func ListFiles(passphrase string) ([]FileEntry, error) {
	entries, err := getFileStore().ListFileEntries(passphrase)
	if err != nil {
		return nil, err
	}
	var result []FileEntry
	for _, e := range entries {
		result = append(result, FileEntry{
			Name:     e.Name,
			MimeType: e.MimeType,
			Size:     e.Size,
		})
	}
	return result, nil
}
