package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"horcrux/internal/config"
	"horcrux/internal/crypto"
)

// Entry represents a single audit record — never contains secret values.
type Entry struct {
	Timestamp string `json:"ts"`
	Operation string `json:"op"`
	Target    string `json:"target,omitempty"`
}

// Append writes an audit entry to the encrypted audit log.
// The log is append-only: each write decrypts the existing log, appends,
// and re-encrypts. The passphrase must match the vault passphrase.
func Append(passphrase, operation, target string) error {
	entries, err := readAll(passphrase)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading audit log: %w", err)
	}

	entries = append(entries, Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Operation: operation,
		Target:    target,
	})

	// Keep only the last 1000 entries
	if len(entries) > 1000 {
		entries = entries[len(entries)-1000:]
	}

	return writeAll(passphrase, entries)
}

// ReadAll returns all audit entries decrypted.
func ReadAll(passphrase string) ([]Entry, error) {
	return readAll(passphrase)
}

func readAll(passphrase string) ([]Entry, error) {
	data, err := os.ReadFile(config.AuditPath())
	if err != nil {
		return nil, err
	}
	plaintext, err := crypto.DecryptData(data, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypting audit log: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(plaintext, &entries); err != nil {
		return nil, fmt.Errorf("parsing audit log: %w", err)
	}
	return entries, nil
}

func writeAll(passphrase string, entries []Entry) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encoding audit log: %w", err)
	}
	encrypted, err := crypto.EncryptData(data, passphrase)
	if err != nil {
		return fmt.Errorf("encrypting audit log: %w", err)
	}
	return os.WriteFile(config.AuditPath(), encrypted, 0600)
}
