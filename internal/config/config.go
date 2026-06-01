package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Config holds all file paths for a Horcrux vault instance.
// Use Default() to get the singleton, or create a local instance for tests.
type Config struct {
	DataDir       string
	PassesPath    string
	MainPassPath  string
	TotpPassPath  string
	ProvidersPath string
	ApiKeysPath   string
	FilesPath     string
	FilesChunksDir string
	AuditPath     string
	DistDir       string
}

var (
	global *Config
	mu     sync.RWMutex
)

// Default returns the singleton global Config, initializing it on first call.
func Default() *Config {
	mu.RLock()
	if global != nil {
		c := global
		mu.RUnlock()
		return c
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()
	if global == nil {
		global = newFromDir(getDefaultDataPath())
	}
	return global
}

// ResetForTest replaces the global Config — use ONLY in tests.
func ResetForTest(c *Config) {
	mu.Lock()
	global = c
	mu.Unlock()
}

func newFromDir(dir string) *Config {
	return &Config{
		DataDir:       dir,
		PassesPath:    fmt.Sprintf("%s/passes.hrcrx", dir),
		MainPassPath:  fmt.Sprintf("%s/mainpass.hrcrx", dir),
		TotpPassPath:  fmt.Sprintf("%s/totp.hrcrx", dir),
		ProvidersPath: fmt.Sprintf("%s/providers.hrcrx", dir),
		ApiKeysPath:   fmt.Sprintf("%s/apikeys.hrcrx", dir),
		FilesPath:      fmt.Sprintf("%s/files.hrcrx", dir),
		FilesChunksDir: fmt.Sprintf("%s/files", dir),
		AuditPath:      fmt.Sprintf("%s/audit.hrcrx", dir),
		DistDir:        fmt.Sprintf("%s/distributed", dir),
	}
}

// New returns a Config rooted at dir — useful for tests with t.TempDir().
func New(dir string) *Config {
	return newFromDir(dir)
}

func getDefaultDataPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	var dataDir string
	if runtime.GOOS == "windows" {
		dataDir = filepath.Join(homeDir, "AppData", "Local", "Horcrux")
	} else {
		dataDir = filepath.Join(homeDir, ".horcrux")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		panic(err)
	}
	return dataDir
}

// --- Backward-compatible globals ---
// These read from the Default() singleton. Existing code that references
// config.DataDir(), config.PassesPath() etc. continues to work unchanged.

func DataDir() string       { return Default().DataDir }
func PassesPath() string    { return Default().PassesPath }
func MainPassPath() string  { return Default().MainPassPath }
func TotpPassPath() string  { return Default().TotpPassPath }
func ProvidersPath() string { return Default().ProvidersPath }
func ApiKeysPath() string   { return Default().ApiKeysPath }
func FilesPath() string      { return Default().FilesPath }
func FilesChunksDir() string { return Default().FilesChunksDir }
func AuditPath() string      { return Default().AuditPath }
func DistDir() string        { return Default().DistDir }
