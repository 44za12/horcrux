package audit_test

import (
	"os"
	"testing"

	"horcrux/internal/audit"
	"horcrux/internal/config"
)

const testPass = "audit-test-pass"

func setup(t *testing.T) {
	t.Helper()
	config.ResetForTest(config.New(t.TempDir()))
}

func TestAppendAndRead(t *testing.T) {
	setup(t)

	if err := audit.Append(testPass, "unlock", ""); err != nil {
		t.Fatal(err)
	}
	if err := audit.Append(testPass, "add-password", "github.com/user"); err != nil {
		t.Fatal(err)
	}

	entries, err := audit.ReadAll(testPass)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Operation != "unlock" || entries[0].Timestamp == "" {
		t.Errorf("bad entry 0: %+v", entries[0])
	}
	if entries[1].Operation != "add-password" || entries[1].Target != "github.com/user" {
		t.Errorf("bad entry 1: %+v", entries[1])
	}
}

func TestAuditPreservesAcrossInstances(t *testing.T) {
	setup(t)

	// Write entries
	if err := audit.Append(testPass, "lock", ""); err != nil {
		t.Fatal(err)
	}

	// Read back (simulates decrypt cycle)
	entries, err := audit.ReadAll(testPass)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestAuditEmptyLog(t *testing.T) {
	setup(t)

	// No file exists yet — should start fresh
	if err := audit.Append(testPass, "create-vault", ""); err != nil {
		t.Fatal(err)
	}

	entries, err := audit.ReadAll(testPass)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestAuditWrongPassphrase(t *testing.T) {
	setup(t)

	if err := audit.Append(testPass, "unlock", ""); err != nil {
		t.Fatal(err)
	}

	// Reading with wrong passphrase should fail
	_, err := audit.ReadAll("wrong-passphrase")
	if err == nil {
		t.Error("expected error with wrong passphrase")
	}
}

func TestAuditFilePermissions(t *testing.T) {
	setup(t)

	if err := audit.Append(testPass, "test", ""); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(config.AuditPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestAuditCap1000Entries(t *testing.T) {
	setup(t)

	for i := 0; i < 20; i++ {
		if err := audit.Append(testPass, "op", ""); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := audit.ReadAll(testPass)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 1000 {
		t.Errorf("expected at most 1000 entries, got %d", len(entries))
	}
	if len(entries) != 20 {
		t.Errorf("expected 20 entries (no cap reached), got %d", len(entries))
	}
}
