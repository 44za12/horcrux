package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type USBProvider struct {
	MountPath string
}

func NewUSBProvider(mountPath string) *USBProvider {
	return &USBProvider{MountPath: mountPath}
}

func (u *USBProvider) Name() string { return "usb" }

func (u *USBProvider) Authenticate(_ context.Context) error {
	info, err := os.Stat(u.MountPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("mount point '%s' not found. Is the drive connected?", u.MountPath)
		}
		return fmt.Errorf("accessing mount point '%s': %w", u.MountPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("'%s' is not a directory", u.MountPath)
	}

	testFile := filepath.Join(u.MountPath, ".horcrux_mount_test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return fmt.Errorf("drive at '%s' is not writable. Is it read-only?", u.MountPath)
	}
	os.Remove(testFile)

	horcruxDir := filepath.Join(u.MountPath, ".horcrux")
	return os.MkdirAll(horcruxDir, 0700)
}

func (u *USBProvider) Upload(_ context.Context, key string, data []byte) error {
	dir := filepath.Join(u.MountPath, ".horcrux")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mount point not accessible: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, key), data, 0600)
}

func (u *USBProvider) Download(_ context.Context, key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(u.MountPath, ".horcrux", key))
}

func (u *USBProvider) Delete(_ context.Context, key string) error {
	return os.Remove(filepath.Join(u.MountPath, ".horcrux", key))
}

func (u *USBProvider) Exists(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(filepath.Join(u.MountPath, ".horcrux", key))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (u *USBProvider) List(_ context.Context, prefix string) ([]string, error) {
	dir := filepath.Join(u.MountPath, ".horcrux")
	entries, err := os.ReadDir(dir)
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
