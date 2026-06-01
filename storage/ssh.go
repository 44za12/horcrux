package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type SSHProvider struct {
	Host       string
	Port       string
	Username   string
	Password   string
	KeyPath    string
	RemotePath string

	client *sftp.Client
}

func NewSSHProvider(host, port, username, password, keyPath, remotePath string) *SSHProvider {
	if port == "" {
		port = "22"
	}
	if remotePath == "" {
		remotePath = ".horcrux"
	}
	return &SSHProvider{
		Host:       host,
		Port:       port,
		Username:   username,
		Password:   password,
		KeyPath:    keyPath,
		RemotePath: remotePath,
	}
}

func (s *SSHProvider) Name() string { return "ssh" }

func (s *SSHProvider) Authenticate(ctx context.Context) error {
	var authMethods []ssh.AuthMethod

	if s.KeyPath != "" {
		keyPath := expandHome(s.KeyPath)
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("reading SSH key %s: %w", s.KeyPath, err)
		}

		var signer ssh.Signer
		if s.Password != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(s.Password))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return fmt.Errorf("parsing SSH key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else if s.Password != "" {
		authMethods = append(authMethods, ssh.Password(s.Password))
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no authentication method configured (provide password or key path)")
	}

	hostKeyCallback, err := knownhosts.New(expandHome("~/.ssh/known_hosts"))
	if err != nil {
		return fmt.Errorf("loading known_hosts for SSH host verification: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            s.Username,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoED25519,
			ssh.CertAlgoED25519v01,
			ssh.KeyAlgoECDSA256,
			ssh.CertAlgoECDSA256v01,
			ssh.KeyAlgoECDSA384,
			ssh.CertAlgoECDSA384v01,
			ssh.KeyAlgoECDSA521,
			ssh.CertAlgoECDSA521v01,
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSASHA512,
			ssh.CertAlgoRSASHA256v01,
			ssh.CertAlgoRSASHA512v01,
		},
		Timeout: 10 * time.Second,
	}

	addr := net.JoinHostPort(s.Host, s.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", addr, err)
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("opening SFTP session: %w", err)
	}

	s.client = client

	s.client.Mkdir(s.RemotePath)
	return nil
}

func (s *SSHProvider) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

func (s *SSHProvider) Upload(_ context.Context, key string, data []byte) error {
	if s.client == nil {
		return fmt.Errorf("not authenticated")
	}

	remotePath := path.Join(s.RemotePath, key)
	f, err := s.client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("creating remote file: %w", err)
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func (s *SSHProvider) Download(_ context.Context, key string) ([]byte, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	remotePath := path.Join(s.RemotePath, key)
	f, err := s.client.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("opening remote file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, f); err != nil {
		return nil, fmt.Errorf("reading remote file: %w", err)
	}
	return buf.Bytes(), nil
}

func (s *SSHProvider) Delete(_ context.Context, key string) error {
	if s.client == nil {
		return fmt.Errorf("not authenticated")
	}
	return s.client.Remove(path.Join(s.RemotePath, key))
}

func (s *SSHProvider) Exists(_ context.Context, key string) (bool, error) {
	if s.client == nil {
		return false, fmt.Errorf("not authenticated")
	}
	_, err := s.client.Stat(path.Join(s.RemotePath, key))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, nil
}

func (s *SSHProvider) List(_ context.Context, prefix string) ([]string, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not authenticated")
	}
	entries, err := s.client.ReadDir(s.RemotePath)
	if err != nil {
		return nil, fmt.Errorf("listing remote directory: %w", err)
	}
	var keys []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			keys = append(keys, e.Name())
		}
	}
	return keys, nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		usr, err := user.Current()
		if err == nil {
			return filepath.Join(usr.HomeDir, p[2:])
		}
		home := os.Getenv("HOME")
		if home != "" {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
