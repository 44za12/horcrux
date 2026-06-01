package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"horcrux/internal/config"
	"horcrux/internal/crypto"
	"horcrux/storage"

	"golang.org/x/oauth2"
)

type ProviderConfig struct {
	Type         string        `json:"type"`
	Token        *oauth2.Token `json:"token,omitempty"`
	ClientID     string        `json:"client_id,omitempty"`
	ClientSecret string        `json:"client_secret,omitempty"`
	Path         string        `json:"path,omitempty"`
	Endpoint     string        `json:"endpoint,omitempty"`
	Region       string        `json:"region,omitempty"`
	Bucket       string        `json:"bucket,omitempty"`
	AccessKey    string        `json:"access_key,omitempty"`
	SecretKey    string        `json:"secret_key,omitempty"`
	Host         string        `json:"host,omitempty"`
	Port         string        `json:"port,omitempty"`
	Username     string        `json:"username,omitempty"`
	Password     string        `json:"password,omitempty"`
	KeyPath      string        `json:"key_path,omitempty"`
	RemotePath   string        `json:"remote_path,omitempty"`
}

type ProvidersFile struct {
	Providers map[string]ProviderConfig `json:"providers"`
}

func LoadConfig(passphrase string) (*ProvidersFile, error) {
	data, err := os.ReadFile(config.ProvidersPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &ProvidersFile{
				Providers: make(map[string]ProviderConfig),
			}, nil
		}
		return nil, err
	}

	plaintext, err := crypto.DecryptData(data, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypting providers config: %w", err)
	}

	var pf ProvidersFile
	if err := json.Unmarshal(plaintext, &pf); err != nil {
		return nil, fmt.Errorf("parsing providers config: %w", err)
	}
	if pf.Providers == nil {
		pf.Providers = make(map[string]ProviderConfig)
	}
	return &pf, nil
}

func SaveConfig(pf *ProvidersFile, passphrase string) error {
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	encrypted, err := crypto.EncryptData(data, passphrase)
	if err != nil {
		return err
	}
	return os.WriteFile(config.ProvidersPath(), encrypted, 0600)
}

func BuildProviders(pf *ProvidersFile) ([]storage.Provider, error) {
	names := make([]string, 0, len(pf.Providers))
	for name := range pf.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	var providers []storage.Provider
	for _, name := range names {
		pc := pf.Providers[name]
		p, err := buildProvider(name, pc)
		if err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}

	return providers, nil
}

func buildProvider(name string, pc ProviderConfig) (storage.Provider, error) {
	switch pc.Type {
	case "local":
		path := pc.Path
		if path == "" {
			path = config.DistDir()
		}
		return storage.NewLocalProvider(path), nil
	case "gdrive":
		return storage.NewGDriveProvider(pc.Token, pc.ClientID, pc.ClientSecret), nil
	case "dropbox":
		return storage.NewDropboxProvider(pc.Token), nil
	case "s3":
		region := pc.Region
		if region == "" {
			region = "us-east-1"
		}
		return storage.NewS3Provider(pc.Endpoint, region, pc.Bucket, pc.AccessKey, pc.SecretKey), nil
	case "usb":
		return storage.NewUSBProvider(pc.Path), nil
	case "ssh":
		return storage.NewSSHProvider(pc.Host, pc.Port, pc.Username, pc.Password, pc.KeyPath, pc.RemotePath), nil
	case "webdav":
		return storage.NewWebDAVProvider(pc.Endpoint, pc.Username, pc.Password), nil
	default:
		return nil, fmt.Errorf("unknown provider type '%s' for '%s'", pc.Type, name)
	}
}

func AuthenticateAll(providers []storage.Provider) error {
	ctx := context.Background()
	for _, p := range providers {
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("authenticating %s: %w", p.Name(), err)
		}
	}
	return nil
}

func CountNonLocal(pf *ProvidersFile) int {
	count := 0
	for _, pc := range pf.Providers {
		if pc.Type != "local" {
			count++
		}
	}
	return count
}

func CalculateThreshold(n int) int {
	m := n - 2
	if m < 2 {
		m = 2
	}
	if m > 3 {
		m = 3
	}
	return m
}

// SetupResult holds the output of a provider setup operation.
type SetupResult struct {
	Config ProviderConfig
	NeedsOAuth bool // true if caller must run an OAuth flow first
}

// SetupAndAuthenticate builds and authenticates a provider from the given config.
// For OAuth providers (gdrive, dropbox), the config must already contain a valid token.
// Returns the updated config (with tokens, etc.) ready to store.
func SetupAndAuthenticate(ctx context.Context, pc ProviderConfig) (ProviderConfig, error) {
	p, err := buildProvider("", pc)
	if err != nil {
		return pc, err
	}
	if err := p.Authenticate(ctx); err != nil {
		return pc, fmt.Errorf("verifying %s: %w", pc.Type, err)
	}
	// SSH connections should be closed after verification
	if closer, ok := p.(interface{ Close() error }); ok {
		closer.Close()
	}
	return pc, nil
}
