package storagemock_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

)

// mockProvider implements storage.Provider for testing the distribute/restore pipeline.
type mockProvider struct {
	name      string
	data      map[string][]byte
	mu        sync.Mutex
	authErr   error
	uploadErr error
	downErr   error
}

func newMockProvider(name string) *mockProvider {
	return &mockProvider{name: name, data: make(map[string][]byte)}
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Authenticate(_ context.Context) error { return m.authErr }

func (m *mockProvider) Upload(_ context.Context, key string, data []byte) error {
	if m.uploadErr != nil {
		return m.uploadErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}

func (m *mockProvider) Download(_ context.Context, key string) ([]byte, error) {
	if m.downErr != nil {
		return nil, m.downErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.data[key]
	if !ok {
		return nil, errors.New("not found")
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (m *mockProvider) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockProvider) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok, nil
}

func TestProviderInterface(t *testing.T) {
	ctx := context.Background()
	p := newMockProvider("test-provider")

	// Name
	if p.Name() != "test-provider" {
		t.Errorf("expected 'test-provider', got '%s'", p.Name())
	}

	// Authenticate
	if err := p.Authenticate(ctx); err != nil {
		t.Errorf("unexpected auth error: %v", err)
	}

	// Upload + Exists
	testData := []byte("encrypted vault fragment")
	if err := p.Upload(ctx, "fragment.hrcrx", testData); err != nil {
		t.Fatal(err)
	}
	exists, err := p.Exists(ctx, "fragment.hrcrx")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("file should exist after upload")
	}

	// Download
	downloaded, err := p.Download(ctx, "fragment.hrcrx")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(downloaded, testData) {
		t.Error("downloaded data doesn't match uploaded")
	}

	// Download should return a copy (not the original slice)
	downloaded[0] = 0xFF
	downloaded2, _ := p.Download(ctx, "fragment.hrcrx")
	if bytes.Equal(downloaded, downloaded2) {
		t.Error("download should return independent copy")
	}

	// Delete
	if err := p.Delete(ctx, "fragment.hrcrx"); err != nil {
		t.Fatal(err)
	}
	exists, _ = p.Exists(ctx, "fragment.hrcrx")
	if exists {
		t.Error("file should not exist after delete")
	}

	// Download non-existent
	_, err = p.Download(ctx, "nonexistent.hrcrx")
	if err == nil {
		t.Error("expected error downloading non-existent file")
	}
}

func TestMockAuthError(t *testing.T) {
	p := newMockProvider("bad-auth")
	p.authErr = errors.New("authentication failed")

	if err := p.Authenticate(context.Background()); err == nil {
		t.Error("expected auth error")
	}
}

func TestMockUploadError(t *testing.T) {
	p := newMockProvider("bad-upload")
	p.uploadErr = errors.New("disk full")

	if err := p.Upload(context.Background(), "key", []byte("data")); err == nil {
		t.Error("expected upload error")
	}
}

func TestMockDownloadError(t *testing.T) {
	p := newMockProvider("bad-download")
	p.downErr = errors.New("connection refused")

	_, err := p.Download(context.Background(), "key")
	if err == nil {
		t.Error("expected download error")
	}
}

func TestMultipleProviders(t *testing.T) {
	ctx := context.Background()
	p1 := newMockProvider("gdrive")
	p2 := newMockProvider("dropbox")
	p3 := newMockProvider("s3")

	data := []byte("shard data")

	// Upload to all three
	for _, p := range []*mockProvider{p1, p2, p3} {
		if err := p.Upload(ctx, "shard.hrcrx", data); err != nil {
			t.Fatalf("upload to %s: %v", p.Name(), err)
		}
	}

	// Download from all three — should match
	for _, p := range []*mockProvider{p1, p2, p3} {
		downloaded, err := p.Download(ctx, "shard.hrcrx")
		if err != nil {
			t.Fatalf("download from %s: %v", p.Name(), err)
		}
		if !bytes.Equal(downloaded, data) {
			t.Errorf("data mismatch for %s", p.Name())
		}
	}

	// Delete from p2, verify others intact
	p2.Delete(ctx, "shard.hrcrx")
	exists, _ := p1.Exists(ctx, "shard.hrcrx")
	if !exists {
		t.Error("p1 should still have the file after p2 delete")
	}
	exists, _ = p2.Exists(ctx, "shard.hrcrx")
	if exists {
		t.Error("p2 should not have the file after delete")
	}
}

func TestConcurrentAccess(t *testing.T) {
	p := newMockProvider("concurrent")
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "fragment.hrcrx"
			if err := p.Upload(ctx, key, []byte("data")); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent access error: %v", err)
	}
}
