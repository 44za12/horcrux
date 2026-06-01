package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"

	"horcrux/internal/audit"
	"horcrux/internal/auth"
	"horcrux/internal/config"
	"horcrux/internal/distribute"
	"horcrux/internal/providers"
	"horcrux/internal/vault"

	"horcrux/storage"
)

type App struct {
	ctx              context.Context
	passphrase       []byte
	cachedPassphrase []byte
	autoLockMinutes  int
}

const autoLockPrefsFile = "prefs.json"

type prefsData struct {
	AutoLockMinutes int `json:"auto_lock_minutes"`
}

// captureBytes copies a string into a []byte for persistent storage.
// The caller should retain no other reference to the original string.
func captureBytes(s string) []byte {
	return []byte(s)
}

func (a *App) passString() string {
	if len(a.passphrase) == 0 {
		return ""
	}
	return string(a.passphrase)
}

func NewApp() *App {
	return &App{
		autoLockMinutes: 5, // default
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.loadPrefs()
}

func prefsPath() string {
	return config.DataDir() + "/" + autoLockPrefsFile
}

func (a *App) loadPrefs() {
	data, err := os.ReadFile(prefsPath())
	if err != nil {
		return // defaults are fine
	}
	var p prefsData
	if json.Unmarshal(data, &p) == nil && p.AutoLockMinutes > 0 {
		a.autoLockMinutes = p.AutoLockMinutes
	}
}

func (a *App) savePrefs() error {
	p := prefsData{AutoLockMinutes: a.autoLockMinutes}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding prefs: %w", err)
	}
	return os.WriteFile(prefsPath(), data, 0600)
}

func (a *App) SetAutoLockTimeout(minutes int) error {
	if minutes < 0 || minutes > 240 {
		return fmt.Errorf("auto-lock timeout must be 0-240 minutes (0 = disabled)")
	}
	a.autoLockMinutes = minutes
	return a.savePrefs()
}

func (a *App) GetAutoLockTimeout() int {
	return a.autoLockMinutes
}

func (a *App) IsInitialized() bool {
	_, err := os.Stat(config.MainPassPath())
	return err == nil
}

func (a *App) HasBiometricKey() bool {
	if len(a.cachedPassphrase) != 0 {
		return true
	}
	return auth.BiometricsAvailable() && auth.HasLocalKey()
}

func (a *App) CreateVault() error {
	return fmt.Errorf("passphrase is required to create a vault")
}

func (a *App) CreateVaultWithPassphrase(passphrase string) error {
	if _, err := vault.InitBSONFiles(passphrase); err != nil {
		return err
	}
	a.passphrase = captureBytes(passphrase)
	a.cachedPassphrase = captureBytes(passphrase)
	_ = auth.StorePassphraseLocal(passphrase)
	_ = audit.Append(passphrase, "create-vault", "")
	return nil
}

func (a *App) UnlockWithBiometric() error {
	// Step 1: Authenticate with Touch ID
	if err := auth.AuthenticateTouchID("Unlock your Horcrux vault"); err != nil {
		return err
	}

	// Step 2: Read passphrase from keychain (no second prompt)
	key, err := auth.GetPassphraseLocal()
	if err != nil {
		return fmt.Errorf("touch ID not available — enter your passphrase to enable it")
	}

	if vault.VerifyPassphrase(key) {
		a.passphrase = captureBytes(key)
		a.cachedPassphrase = captureBytes(key)
		_ = audit.Append(key, "unlock-biometric", "")
		return nil
	}
	if _, err := vault.ListPasswords(key); err != nil {
		auth.DeleteLocalKey()
		return fmt.Errorf("stored key is invalid")
	}
	a.passphrase = captureBytes(key)
	a.cachedPassphrase = captureBytes(key)
	vault.MigrateVerification(key)
	return nil
}

func (a *App) UnlockWithPassphrase(passphrase string) error {
	if vault.VerifyPassphrase(passphrase) {
		a.passphrase = captureBytes(passphrase)
		a.cachedPassphrase = captureBytes(passphrase)
		_ = auth.StorePassphraseLocal(passphrase)
		_ = audit.Append(passphrase, "unlock-passphrase", "")
		return nil
	}
	if _, err := vault.ListPasswords(passphrase); err != nil {
		return fmt.Errorf("invalid passphrase")
	}
	a.passphrase = captureBytes(passphrase)
	a.cachedPassphrase = captureBytes(passphrase)
	vault.MigrateVerification(passphrase)
	_ = auth.StorePassphraseLocal(passphrase)
	return nil
}

func (a *App) ChangePassphrase(oldPassphrase, newPassphrase string) error {
	if len(a.passphrase) == 0 {
		return fmt.Errorf("vault is locked")
	}
	if err := vault.ChangePassphrase(oldPassphrase, newPassphrase); err != nil {
		return err
	}
	// Update in-memory passphrase and keychain
	for i := range a.passphrase {
		a.passphrase[i] = 0
	}
	a.passphrase = captureBytes(newPassphrase)
	for i := range a.cachedPassphrase {
		a.cachedPassphrase[i] = 0
	}
	a.cachedPassphrase = captureBytes(newPassphrase)
	_ = auth.StorePassphraseLocal(newPassphrase)
	return nil
}

func (a *App) Lock() {
	_ = audit.Append(a.passString(), "lock", "")
	for i := range a.passphrase {
		a.passphrase[i] = 0
	}
	a.passphrase = nil
	for i := range a.cachedPassphrase {
		a.cachedPassphrase[i] = 0
	}
	a.cachedPassphrase = nil
}

func (a *App) IsLocked() bool {
	return len(a.passString()) == 0
}

func (a *App) ListPasswords() ([]vault.PasswordEntry, error) {
	if len(a.passString()) == 0 {
		return nil, fmt.Errorf("vault is locked")
	}
	return vault.ListPasswords(a.passString())
}

func (a *App) GetPassword(site, username string) (string, error) {
	if len(a.passString()) == 0 {
		return "", fmt.Errorf("vault is locked")
	}
	pass, _, err := vault.GetPassword(site, username, a.passString())
	return pass, err
}

func (a *App) AddPassword(site, username, password, notes string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	return vault.AddUpdatePassword(site, username, password, notes, a.passString())
}

func (a *App) RemovePassword(site, username string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	return vault.RemovePassword(site, username, a.passString())
}

func (a *App) SearchPasswords(query string) ([]vault.PasswordEntry, error) {
	if len(a.passString()) == 0 {
		return nil, fmt.Errorf("vault is locked")
	}
	return vault.SearchPasswords(a.passString(), query)
}

func (a *App) ListTotpServices() ([]vault.TotpService, error) {
	if len(a.passString()) == 0 {
		return nil, fmt.Errorf("vault is locked")
	}
	return vault.ListTOTPServices(a.passString())
}

func (a *App) GetTotpCode(service string) (string, error) {
	if len(a.passString()) == 0 {
		return "", fmt.Errorf("vault is locked")
	}
	code, err := vault.GetTOTP(service, a.passString())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", code), nil
}

func (a *App) GetTotpSecondsRemaining() int {
	return vault.GetTotpSecondsRemaining()
}

func (a *App) AddTotp(service, secret string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	return vault.AddUpdateTOTP(service, secret, a.passString())
}

func (a *App) RemoveTotp(service string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	return vault.RemoveTOTP(service, a.passString())
}

type ProviderInfo struct {
	Name   string `json:"Name"`
	Type   string `json:"Type"`
	Status string `json:"Status"`
}

func providerStatus(pc providers.ProviderConfig) string {
	if pc.Token != nil && pc.Token.Valid() {
		return "authenticated"
	}
	if pc.Token != nil && pc.Token.RefreshToken != "" {
		return "token expired (will refresh)"
	}
	switch pc.Type {
	case "local":
		return "ready"
	case "s3":
		if pc.AccessKey != "" {
			return "ready"
		}
	case "usb":
		if pc.Path != "" {
			return "ready"
		}
	case "ssh":
		if pc.Host != "" {
			return "ready"
		}
	case "webdav":
		if pc.Endpoint != "" {
			return "ready"
		}
	}
	return "not configured"
}

func (a *App) ListProviders() ([]ProviderInfo, error) {
	if len(a.passString()) == 0 {
		return nil, fmt.Errorf("vault is locked")
	}
	pf, err := providers.LoadConfig(a.passString())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(pf.Providers))
	for name := range pf.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	var result []ProviderInfo
	for _, name := range names {
		pc := pf.Providers[name]
		result = append(result, ProviderInfo{Name: name, Type: pc.Type, Status: providerStatus(pc)})
	}
	return result, nil
}

func (a *App) RemoveProvider(name string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	pf, err := providers.LoadConfig(a.passString())
	if err != nil {
		return err
	}
	delete(pf.Providers, name)
	return providers.SaveConfig(pf, a.passString())
}

type ProviderTypes struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

func (a *App) GetProviderTypes() []ProviderTypes {
	return []ProviderTypes{
		{ID: "local", Name: "Local Storage"},
		{ID: "gdrive", Name: "Google Drive"},
		{ID: "dropbox", Name: "Dropbox"},
		{ID: "s3", Name: "S3 Compatible"},
		{ID: "usb", Name: "USB Drive"},
		{ID: "ssh", Name: "SSH / SFTP"},
		{ID: "webdav", Name: "WebDAV"},
	}
}

type AddProviderRequest struct {
	ProviderType string `json:"ProviderType"`
	Name         string `json:"Name"`
	ClientID     string `json:"ClientID,omitempty"`
	ClientSecret string `json:"ClientSecret,omitempty"`
	Endpoint     string `json:"Endpoint,omitempty"`
	Region       string `json:"Region,omitempty"`
	Bucket       string `json:"Bucket,omitempty"`
	AccessKey    string `json:"AccessKey,omitempty"`
	SecretKey    string `json:"SecretKey,omitempty"`
	Path         string `json:"Path,omitempty"`
	Host         string `json:"Host,omitempty"`
	Port         string `json:"Port,omitempty"`
	Username     string `json:"Username,omitempty"`
	Password     string `json:"Password,omitempty"`
	KeyPath      string `json:"KeyPath,omitempty"`
	RemotePath   string `json:"RemotePath,omitempty"`
}

func (a *App) AddProvider(req AddProviderRequest) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}

	providerName := req.Name
	if providerName == "" {
		providerName = req.ProviderType
	}

	pf, err := providers.LoadConfig(a.passString())
	if err != nil {
		return fmt.Errorf("loading providers: %w", err)
	}

	if _, exists := pf.Providers[providerName]; exists {
		return fmt.Errorf("provider '%s' already exists", providerName)
	}

	ctx := context.Background()

	switch req.ProviderType {
	case "gdrive":
		clientID := req.ClientID
		clientSecret := req.ClientSecret
		if clientID == "" {
			return fmt.Errorf("Google Drive requires client_id (set up at https://console.cloud.google.com/apis/credentials)")
		}
		token, err := storage.RunGDriveAuth(clientID, clientSecret)
		if err != nil {
			return fmt.Errorf("Google Drive auth failed: %w", err)
		}
		p := storage.NewGDriveProvider(token, clientID, clientSecret)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("Google Drive verification failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:         "gdrive",
			Token:        token,
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}

	case "dropbox":
		token, err := storage.RunDropboxAuth()
		if err != nil {
			return fmt.Errorf("Dropbox auth failed: %w", err)
		}
		p := storage.NewDropboxProvider(token)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("Dropbox verification failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:  "dropbox",
			Token: token,
		}

	case "local":
		if providerName != "local" {
			return fmt.Errorf("local provider must be named 'local'")
		}
		path := req.Path
		if path == "" {
			path = config.DistDir()
		}
		p := storage.NewLocalProvider(path)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("local setup failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type: "local",
			Path: path,
		}

	case "s3":
		if req.Endpoint == "" || req.Bucket == "" || req.AccessKey == "" || req.SecretKey == "" {
			return fmt.Errorf("endpoint, bucket, access key, and secret key are required")
		}
		region := req.Region
		if region == "" {
			region = "us-east-1"
		}
		p := storage.NewS3Provider(req.Endpoint, region, req.Bucket, req.AccessKey, req.SecretKey)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("S3 connection failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:      "s3",
			Endpoint:  req.Endpoint,
			Region:    region,
			Bucket:    req.Bucket,
			AccessKey: req.AccessKey,
			SecretKey: req.SecretKey,
		}

	case "usb":
		if req.Path == "" {
			return fmt.Errorf("mount path is required")
		}
		p := storage.NewUSBProvider(req.Path)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("USB check failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type: "usb",
			Path: req.Path,
		}

	case "ssh":
		if req.Host == "" || req.Username == "" {
			return fmt.Errorf("host and username are required")
		}
		port := req.Port
		if port == "" {
			port = "22"
		}
		remotePath := req.RemotePath
		if remotePath == "" {
			remotePath = ".horcrux"
		}
		p := storage.NewSSHProvider(req.Host, port, req.Username, req.Password, req.KeyPath, remotePath)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("SSH connection failed: %w", err)
		}
		p.Close()
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:       "ssh",
			Host:       req.Host,
			Port:       port,
			Username:   req.Username,
			Password:   req.Password,
			KeyPath:    req.KeyPath,
			RemotePath: remotePath,
		}

	case "webdav":
		if req.Endpoint == "" || req.Username == "" || req.Password == "" {
			return fmt.Errorf("endpoint, username, and password are required")
		}
		p := storage.NewWebDAVProvider(req.Endpoint, req.Username, req.Password)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("WebDAV connection failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:     "webdav",
			Endpoint: req.Endpoint,
			Username: req.Username,
			Password: req.Password,
		}

	default:
		return fmt.Errorf("unknown provider type '%s'", req.ProviderType)
	}

	if err := providers.SaveConfig(pf, a.passString()); err != nil {
		return fmt.Errorf("saving provider: %w", err)
	}
	return nil
}

type DistributeStatus struct {
	Total         int  `json:"Total"`
	Threshold     int  `json:"Threshold"`
	CanDistribute bool `json:"CanDistribute"`
	CanRestore    bool `json:"CanRestore"`
	Failures      int  `json:"Failures"`
}

func (a *App) GetDistributeStatus() (*DistributeStatus, error) {
	if len(a.passString()) == 0 {
		return nil, fmt.Errorf("vault is locked")
	}
	pf, err := providers.LoadConfig(a.passString())
	if err != nil {
		return nil, err
	}
	n := len(pf.Providers)
	m := providers.CalculateThreshold(n)
	return &DistributeStatus{
		Total:         n,
		Threshold:     m,
		CanDistribute: n >= 3,
		CanRestore:    n >= m && m > 0,
		Failures:      n - m,
	}, nil
}

func (a *App) Distribute() error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	if err := distribute.Distribute(a.passString()); err != nil {
		return err
	}
	_ = audit.Append(a.passString(), "distribute", "")
	return nil
}

func (a *App) Restore() error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	if err := distribute.Restore(a.passString()); err != nil {
		return err
	}
	_ = audit.Append(a.passString(), "restore", "")
	return nil
}

func (a *App) ImportCSV(content string) (int, error) {
	if len(a.passString()) == 0 {
		return 0, fmt.Errorf("vault is locked")
	}
	reader := csv.NewReader(strings.NewReader(content))
	records, err := reader.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("reading CSV: %w", err)
	}
	count := 0
	for i, record := range records {
		if i == 0 {
			continue
		}
		if len(record) < 4 {
			continue
		}
		title := record[0]
		url := record[1]
		username := record[2]
		password := record[3]
		var notes string
		if len(record) >= 5 {
			notes = record[4]
		}
		site := title
		if site == "" {
			site = url
		}
		if site != "" && username != "" && password != "" {
			if err := vault.AddUpdatePassword(site, username, password, notes, a.passString()); err != nil {
				return count, fmt.Errorf("row %d: %w", i+1, err)
			}
			count++
		}
		if len(record) >= 6 && strings.TrimSpace(record[5]) != "" {
			otpauth := strings.TrimSpace(record[5])
			name, secret, err := parseOTPAuthURI(otpauth)
			if err == nil && name != "" && secret != "" {
				if err := vault.AddUpdateTOTP(name, secret, a.passString()); err == nil {
					count++
				}
			}
		}
	}
	return count, nil
}

func (a *App) ImportTOTP(content string) (int, error) {
	if len(a.passString()) == 0 {
		return 0, fmt.Errorf("vault is locked")
	}
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "{") {
		return a.importTOTPJSON(content)
	}
	return a.importTOTPURIs(content)
}

type TwoFASExport struct {
	Services []struct {
		Name   string `json:"name"`
		Secret string `json:"secret"`
		Otp    struct {
			Account string `json:"account"`
		} `json:"otp"`
	} `json:"services"`
}

func (a *App) importTOTPJSON(content string) (int, error) {
	var export TwoFASExport
	if err := json.Unmarshal([]byte(content), &export); err != nil {
		return 0, fmt.Errorf("invalid JSON: %w", err)
	}
	count := 0
	for _, svc := range export.Services {
		if svc.Name == "" || svc.Secret == "" {
			continue
		}
		name := svc.Name
		if svc.Otp.Account != "" {
			name = svc.Name + " (" + svc.Otp.Account + ")"
		}
		if err := vault.AddUpdateTOTP(name, svc.Secret, a.passString()); err != nil {
			return count, fmt.Errorf("%s: %w", name, err)
		}
		count++
	}
	return count, nil
}

func (a *App) importTOTPURIs(content string) (int, error) {
	lines := strings.Split(content, "\n")
	count := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "otpauth://") {
			continue
		}
		name, secret, err := parseOTPAuthURI(line)
		if err != nil {
			return count, fmt.Errorf("line %d: %w", i+1, err)
		}
		if name == "" || secret == "" {
			continue
		}
		if err := vault.AddUpdateTOTP(name, secret, a.passString()); err != nil {
			return count, fmt.Errorf("%s: %w", name, err)
		}
		count++
	}
	return count, nil
}

func parseOTPAuthURI(uri string) (string, string, error) {
	uri = strings.TrimPrefix(uri, "otpauth://")
	slashIdx := strings.Index(uri, "/")
	if slashIdx < 0 {
		return "", "", fmt.Errorf("invalid otpauth URI: no path")
	}
	uri = uri[slashIdx+1:]
	qIdx := strings.Index(uri, "?")
	var label string
	var query string
	if qIdx >= 0 {
		label = uri[:qIdx]
		query = uri[qIdx+1:]
	} else {
		label = uri
		query = ""
	}
	params := parseQuery(query)
	secret := params["secret"]
	issuer := params["issuer"]
	name := label
	if issuer != "" && !strings.Contains(label, issuer) {
		name = issuer + " (" + label + ")"
	}
	return name, secret, nil
}

func parseQuery(q string) map[string]string {
	result := make(map[string]string)
	for _, pair := range strings.Split(q, "&") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}
	return result
}

func (a *App) GetDebugInfo() string {
	var b strings.Builder
	fmt.Fprintf(&b, "DataDir: %s\n", config.DataDir())
	fmt.Fprintf(&b, "ProvidersPath: %s\n", config.ProvidersPath())
	fmt.Fprintf(&b, "PassesPath: %s\n", config.PassesPath())
	fmt.Fprintf(&b, "KeychainHasEntry: %v\n", auth.HasLocalKey())

	if _, err := os.Stat(config.ProvidersPath()); err != nil {
		fmt.Fprintf(&b, "Providers file: NOT FOUND (%v)\n", err)
	} else {
		info, _ := os.Stat(config.ProvidersPath())
		fmt.Fprintf(&b, "Providers file: exists (%d bytes)\n", info.Size())
	}

	if len(a.passphrase) != 0 {
		pf, err := providers.LoadConfig(a.passString())
		if err != nil {
			fmt.Fprintf(&b, "LoadConfig error: %v\n", err)
		} else {
			fmt.Fprintf(&b, "Providers loaded: %d\n", len(pf.Providers))
			for name, pc := range pf.Providers {
				fmt.Fprintf(&b, "  - %s (%s)\n", name, pc.Type)
			}
		}
	} else {
		fmt.Fprintf(&b, "Not unlocked yet\n")
	}
	return b.String()
}

func (a *App) GeneratePassword() string {
	password, err := randomString("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+-=[]{}|;:,.<>?", 20)
	if err != nil {
		return ""
	}
	return password
}

func (a *App) IsCLIInstalled() bool {
	_, err := os.Stat("/usr/local/bin/horcrux")
	return err == nil
}

func (a *App) InstallCLI() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}
	binPath := "/usr/local/bin/horcrux"
	data, err := os.ReadFile(exe)
	if err != nil {
		return fmt.Errorf("reading binary: %w", err)
	}
	if err := os.WriteFile(binPath, data, 0755); err != nil {
		return fmt.Errorf("writing to /usr/local/bin (try running: sudo cp %s /usr/local/bin/horcrux): %w", exe, err)
	}
	return nil
}

func (a *App) ListApiKeys() ([]vault.ApiKeyEntry, error) {
	if len(a.passString()) == 0 {
		return nil, fmt.Errorf("vault is locked")
	}
	return vault.ListApiKeys(a.passString())
}

func (a *App) GetApiKey(service, name string) (string, error) {
	if len(a.passString()) == 0 {
		return "", fmt.Errorf("vault is locked")
	}
	key, _, err := vault.GetApiKey(service, name, a.passString())
	return key, err
}

func (a *App) AddApiKey(service, name, key, notes string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	return vault.AddUpdateApiKey(service, name, key, notes, a.passString())
}

func (a *App) RemoveApiKey(service, name string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	return vault.RemoveApiKey(service, name, a.passString())
}

func (a *App) ListFiles() ([]vault.FileEntry, error) {
	if len(a.passString()) == 0 {
		return nil, fmt.Errorf("vault is locked")
	}
	return vault.ListFiles(a.passString())
}

func (a *App) AddFile(filename, mimeType, contentB64 string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	content, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return fmt.Errorf("decoding file content: %w", err)
	}
	return vault.AddFile(filename, mimeType, content, a.passString())
}

func (a *App) GetFile(filename string) (string, error) {
	if len(a.passString()) == 0 {
		return "", fmt.Errorf("vault is locked")
	}
	data, mimeType, err := vault.GetFile(filename, a.passString())
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	result, _ := json.Marshal(map[string]string{"Data": encoded, "MimeType": mimeType})
	return string(result), nil
}

func (a *App) RemoveFile(filename string) error {
	if len(a.passString()) == 0 {
		return fmt.Errorf("vault is locked")
	}
	return vault.RemoveFile(filename, a.passString())
}

func (a *App) generateNewPassword() (string, error) {
	return randomString("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+-=[]{}|;:,.<>?", 20)
}

func randomString(charset string, length int) (string, error) {
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}
