package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type LocalProvider struct {
	BaseDir string
}

func NewLocalProvider(baseDir string) *LocalProvider {
	return &LocalProvider{BaseDir: baseDir}
}

func (l *LocalProvider) Name() string { return "local" }

func (l *LocalProvider) Authenticate(_ context.Context) error {
	return os.MkdirAll(l.BaseDir, 0700)
}

func (l *LocalProvider) Upload(_ context.Context, key string, data []byte) error {
	if err := os.MkdirAll(l.BaseDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(l.BaseDir, key), data, 0600)
}

func (l *LocalProvider) Download(_ context.Context, key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(l.BaseDir, key))
}

func (l *LocalProvider) Delete(_ context.Context, key string) error {
	return os.Remove(filepath.Join(l.BaseDir, key))
}

func (l *LocalProvider) Exists(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(filepath.Join(l.BaseDir, key))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (l *LocalProvider) List(_ context.Context, prefix string) ([]string, error) {
	entries, err := os.ReadDir(l.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			keys = append(keys, e.Name())
		}
	}
	return keys, nil
}
