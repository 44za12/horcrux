package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"horcrux/internal/config"
)

func TestDefaultSingleton(t *testing.T) {
	c1 := config.Default()
	c2 := config.Default()
	if c1 != c2 {
		t.Error("Default() should return the same instance")
	}
	if c1.DataDir == "" {
		t.Error("DataDir should not be empty")
	}
	if c1.PassesPath == "" || c1.MainPassPath == "" || c1.TotpPassPath == "" {
		t.Error("vault file paths should not be empty")
	}
	if c1.ProvidersPath == "" || c1.ApiKeysPath == "" || c1.FilesPath == "" {
		t.Error("provider paths should not be empty")
	}
	if c1.AuditPath == "" {
		t.Error("audit path should not be empty")
	}
	if c1.DistDir == "" {
		t.Error("dist dir should not be empty")
	}
}

func TestNewIsolated(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	c := config.New(sub)
	if c.DataDir != sub {
		t.Errorf("expected DataDir %q, got %q", sub, c.DataDir)
	}
	expected := sub + "/passes.hrcrx"
	if c.PassesPath != expected {
		t.Errorf("expected PassesPath %q, got %q", expected, c.PassesPath)
	}
}

func TestResetForTest(t *testing.T) {
	original := config.Default()
	defer config.ResetForTest(original)

	tmp := config.New(t.TempDir())
	config.ResetForTest(tmp)

	// Default should now return the test instance
	curr := config.Default()
	if curr.DataDir != tmp.DataDir {
		t.Errorf("expected DataDir %q after ResetForTest, got %q", tmp.DataDir, curr.DataDir)
	}

	// Backup-compatible functions should reflect the test dir
	if config.DataDir() != tmp.DataDir {
		t.Error("DataDir() should reflect the test instance")
	}
	if config.PassesPath() != tmp.PassesPath {
		t.Error("PassesPath() should reflect the test instance")
	}

	// Restore
	config.ResetForTest(original)
	restored := config.Default()
	if restored.DataDir != original.DataDir {
		t.Error("Default() should return original after restore")
	}
}

func TestFunctionAliases(t *testing.T) {
	c := config.Default()
	if config.DataDir() != c.DataDir {
		t.Error("DataDir() mismatch")
	}
	if config.PassesPath() != c.PassesPath {
		t.Error("PassesPath() mismatch")
	}
	if config.MainPassPath() != c.MainPassPath {
		t.Error("MainPassPath() mismatch")
	}
	if config.TotpPassPath() != c.TotpPassPath {
		t.Error("TotpPassPath() mismatch")
	}
	if config.ProvidersPath() != c.ProvidersPath {
		t.Error("ProvidersPath() mismatch")
	}
	if config.ApiKeysPath() != c.ApiKeysPath {
		t.Error("ApiKeysPath() mismatch")
	}
	if config.FilesPath() != c.FilesPath {
		t.Error("FilesPath() mismatch")
	}
	if config.AuditPath() != c.AuditPath {
		t.Error("AuditPath() mismatch")
	}
	if config.DistDir() != c.DistDir {
		t.Error("DistDir() mismatch")
	}
}

func TestDataDirCreatesDirectory(t *testing.T) {
	// Just verify the default dir exists
	_, err := os.Stat(config.Default().DataDir)
	if err != nil {
		t.Fatalf("default DataDir should exist: %v", err)
	}
}
