package vault_test

import (
	"encoding/json"
	"os"
	"testing"

	"horcrux/internal/config"
	"horcrux/internal/vault"
)

func setupTempDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	config.ResetForTest(config.New(dir))
	return dir
}

const testPassphrase = "test-vault-passphrase-12345"

func TestInitAndVerify(t *testing.T) {
	setupTempDataDir(t)

	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	if !vault.VerifyPassphrase(testPassphrase) {
		t.Error("VerifyPassphrase returned false for correct passphrase")
	}

	if vault.VerifyPassphrase("wrong-passphrase") {
		t.Error("VerifyPassphrase returned true for wrong passphrase")
	}
}

func TestPasswordCRUD(t *testing.T) {
	setupTempDataDir(t)

	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	// Add password with notes
	if err := vault.AddUpdatePassword("github.com", "user@email.com", "secret123", "personal account", testPassphrase); err != nil {
		t.Fatal(err)
	}

	// Retrieve
	pass, notes, err := vault.GetPassword("github.com", "user@email.com", testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if pass != "secret123" {
		t.Errorf("expected password 'secret123', got '%s'", pass)
	}
	if notes != "personal account" {
		t.Errorf("expected notes 'personal account', got '%s'", notes)
	}

	// List
	list, err := vault.ListPasswords(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}

	// Search
	results, err := vault.SearchPasswords(testPassphrase, "github")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}

	// Remove
	if err := vault.RemovePassword("github.com", "user@email.com", testPassphrase); err != nil {
		t.Fatal(err)
	}
	_, _, err = vault.GetPassword("github.com", "user@email.com", testPassphrase)
	if err == nil {
		t.Error("expected error getting removed password, got nil")
	}
}

func TestPasswordWithEmptyNotes(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	if err := vault.AddUpdatePassword("example.com", "admin", "pass123", "", testPassphrase); err != nil {
		t.Fatal(err)
	}

	pass, notes, err := vault.GetPassword("example.com", "admin", testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if pass != "pass123" {
		t.Errorf("expected 'pass123', got '%s'", pass)
	}
	if notes != "" {
		t.Errorf("expected empty notes, got '%s'", notes)
	}
}

func TestPasswordNotesJSONRoundTrip(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	// Notes with special characters that look like JSON
	specialNotes := `{"key": "value", "nested": [1, 2, 3]}`
	if err := vault.AddUpdatePassword("test.io", "user", "mypass", specialNotes, testPassphrase); err != nil {
		t.Fatal(err)
	}

	pass, notes, err := vault.GetPassword("test.io", "user", testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if pass != "mypass" {
		t.Errorf("expected 'mypass', got '%s'", pass)
	}
	if notes != specialNotes {
		t.Errorf("expected notes round-trip, got '%s'", notes)
	}
}

func TestTOTPCRUD(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	if err := vault.AddUpdateTOTP("GitHub", "JBSWY3DPEHPK3PXP", testPassphrase); err != nil {
		t.Fatal(err)
	}

	code, err := vault.GetTOTP("GitHub", testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if code < 0 || code > 999999 {
		t.Errorf("TOTP code out of range: %d", code)
	}

	services, err := vault.ListTOTPServices(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if services[0].Name != "GitHub" {
		t.Errorf("expected service 'GitHub', got '%s'", services[0].Name)
	}

	// Secret should be masked in listing
	if services[0].Secret != "" {
		t.Error("TOTP secret should be empty in listing")
	}

	if err := vault.RemoveTOTP("GitHub", testPassphrase); err != nil {
		t.Fatal(err)
	}
	_, err = vault.GetTOTP("GitHub", testPassphrase)
	if err == nil {
		t.Error("expected error getting removed TOTP")
	}
}

func TestApiKeyCRUD(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	if err := vault.AddUpdateApiKey("OpenAI", "Production", "sk-abc123", "GPT-4 key", testPassphrase); err != nil {
		t.Fatal(err)
	}

	key, notes, err := vault.GetApiKey("OpenAI", "Production", testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if key != "sk-abc123" {
		t.Errorf("expected 'sk-abc123', got '%s'", key)
	}
	if notes != "GPT-4 key" {
		t.Errorf("expected 'GPT-4 key', got '%s'", notes)
	}

	keys, err := vault.ListApiKeys(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	// Key value should be masked
	if keys[0].Key != "" {
		t.Error("API key value should be empty in listing")
	}

	if err := vault.RemoveApiKey("OpenAI", "Production", testPassphrase); err != nil {
		t.Fatal(err)
	}
	_, _, err = vault.GetApiKey("OpenAI", "Production", testPassphrase)
	if err == nil {
		t.Error("expected error getting removed API key")
	}
}

func TestFileCRUD(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	content := []byte("Hello, secure world!")
	if err := vault.AddFile("greeting.txt", "text/plain", content, testPassphrase); err != nil {
		t.Fatal(err)
	}

	data, mimeType, err := vault.GetFile("greeting.txt", testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Hello, secure world!" {
		t.Errorf("file content mismatch: got '%s'", string(data))
	}
	if mimeType != "text/plain" {
		t.Errorf("expected 'text/plain', got '%s'", mimeType)
	}

	files, err := vault.ListFiles(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "greeting.txt" || files[0].Size != 20 {
		t.Errorf("file listing mismatch: %+v", files[0])
	}

	if err := vault.RemoveFile("greeting.txt", testPassphrase); err != nil {
		t.Fatal(err)
	}
	_, _, err = vault.GetFile("greeting.txt", testPassphrase)
	if err == nil {
		t.Error("expected error getting removed file")
	}
}

func TestMigrateVerificationPreservesV2(t *testing.T) {
	_ = setupTempDataDir(t)

	// First init creates a v2 verification file
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	// MigrateVerification should be a no-op on an already-v2 file
	if err := vault.MigrateVerification(testPassphrase); err != nil {
		t.Fatal(err)
	}

	// Verify version is still 2
	data, _ := os.ReadFile(config.MainPassPath())
	var vd map[string]interface{}
	json.Unmarshal(data, &vd)
	if version, ok := vd["version"]; !ok || version.(float64) != 2 {
		t.Errorf("expected version 2 after migration, got %+v", vd)
	}
}

func TestGenerateTOTPFromKnownSecret(t *testing.T) {
	// RFC 6238 test vector: secret "12345678901234567890" Base32 = GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ
	// We just verify we get a 6-digit code
	code, err := vault.GenerateTOTP("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if code < 0 || code > 999999 {
		t.Errorf("TOTP code out of range: %d", code)
	}
}

func TestVaultPersistence(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	// Add data
	vault.AddUpdatePassword("site1.com", "user1", "pass1", "", testPassphrase)
	vault.AddUpdatePassword("site2.com", "user2", "pass2", "notes2", testPassphrase)

	// Re-read (simulates app restart — new process, same files)
	list, err := vault.ListPasswords(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 entries after persistence check, got %d", len(list))
	}
}

func TestGetPasswordWrongSite(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	_, _, err := vault.GetPassword("nonexistent.com", "user", testPassphrase)
	if err == nil {
		t.Error("expected error for nonexistent site")
	}
}

func TestGetPasswordWrongUsername(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	vault.AddUpdatePassword("example.com", "realuser", "pass", "", testPassphrase)

	_, _, err := vault.GetPassword("example.com", "fakeuser", testPassphrase)
	if err == nil {
		t.Error("expected error for wrong username")
	}
}

func TestListEmptyVault(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	passwords, _ := vault.ListPasswords(testPassphrase)
	if len(passwords) != 0 {
		t.Error("expected empty password list for new vault")
	}

	totps, _ := vault.ListTOTPServices(testPassphrase)
	if len(totps) != 0 {
		t.Error("expected empty TOTP list for new vault")
	}

	apiKeys, _ := vault.ListApiKeys(testPassphrase)
	if len(apiKeys) != 0 {
		t.Error("expected empty API key list for new vault")
	}

	files, _ := vault.ListFiles(testPassphrase)
	if len(files) != 0 {
		t.Error("expected empty file list for new vault")
	}
}

func TestUpdatePassword(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	// Add then update
	vault.AddUpdatePassword("example.com", "user", "oldpass", "oldnotes", testPassphrase)
	vault.AddUpdatePassword("example.com", "user", "newpass", "newnotes", testPassphrase)

	pass, notes, err := vault.GetPassword("example.com", "user", testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if pass != "newpass" {
		t.Errorf("expected updated password 'newpass', got '%s'", pass)
	}
	if notes != "newnotes" {
		t.Errorf("expected updated notes 'newnotes', got '%s'", notes)
	}
}

func TestChangePassphrase(t *testing.T) {
	setupTempDataDir(t)
	if _, err := vault.InitBSONFiles(testPassphrase); err != nil {
		t.Fatal(err)
	}

	// Add data before rotation
	vault.AddUpdatePassword("site.com", "user", "mypass", "mynotes", testPassphrase)
	vault.AddUpdateTOTP("GitHub", "JBSWY3DPEHPK3PXP", testPassphrase)

	newPass := "new-vault-passphrase-67890"

	// Rotate
	if err := vault.ChangePassphrase(testPassphrase, newPass); err != nil {
		t.Fatal(err)
	}

	// Old passphrase should fail
	if vault.VerifyPassphrase(testPassphrase) {
		t.Error("old passphrase should no longer verify")
	}

	// New passphrase should verify
	if !vault.VerifyPassphrase(newPass) {
		t.Error("new passphrase should verify")
	}

	// Data should be accessible with new passphrase
	pass, notes, err := vault.GetPassword("site.com", "user", newPass)
	if err != nil {
		t.Fatal(err)
	}
	if pass != "mypass" || notes != "mynotes" {
		t.Errorf("data mismatch after rotation: pass=%s notes=%s", pass, notes)
	}

	code, err := vault.GetTOTP("GitHub", newPass)
	if err != nil {
		t.Fatal(err)
	}
	if code < 0 || code > 999999 {
		t.Error("TOTP generation failed after rotation")
	}
}

func TestTotpSecondsRemaining(t *testing.T) {
	remaining := vault.GetTotpSecondsRemaining()
	if remaining < 0 || remaining > 30 {
		t.Errorf("seconds remaining should be 0-30, got %d", remaining)
	}
}
