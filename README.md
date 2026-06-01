# Horcrux

### Distributed, Zero-Trust Secret Manager

A secret manager that splits your vault across multiple cloud providers using Shamir's Secret Sharing and Reed-Solomon erasure coding — no single point of failure, no single point of compromise.

---

## Features

- **Password, TOTP, API Key & File Management** — Securely store, retrieve, and organize all your secrets.
- **Distributed Backup** — Vault split across up to 7 cloud providers. Need M-of-N to recover — lose a provider, you're still safe.
- **Incremental Distribution** — Content-addressed segments mean only changed data is re-uploaded. GB-scale vaults distribute in seconds.
- **Touch ID Unlock** — Biometric unlock via macOS Keychain. Falls back gracefully to passphrase on Macs without Touch ID.
- **7 Storage Providers** — Local filesystem, Google Drive, Dropbox, S3/MinIO, USB drives, SSH/SFTP, and WebDAV.
- **Zero-Knowledge** — Providers see only opaque encrypted blobs. The vault passphrase never leaves your machine.
- **Fuzzy Search** — Find passwords and TOTP entries with approximate queries.
- **Import** — CSV passwords, 2FAS JSON TOTP exports.
- **Cross-Platform CLI** — Go binary runs on macOS, Linux, and Windows. GUI is macOS-native (Wails + Svelte).

---

## Quick Start

### macOS GUI

1. Download `Horcrux.app` from [Releases](https://github.com/44za12/horcrux/releases) and move to `/Applications`.
2. Launch, create a passphrase, and start adding passwords.
3. Add 2+ storage providers under Providers, then Distribute to back up your vault.

### CLI (macOS / Linux / Windows)

```bash
# Download and install
curl -L https://github.com/44za12/horcrux/releases/latest/download/horcrux-darwin-arm64 -o /usr/local/bin/horcrux
chmod +x /usr/local/bin/horcrux

# Initialize
horcrux init

# Add a password
horcrux pass addpass github.com user@email.com mypassword

# Get a password
horcrux pass getpass github.com user@email.com

# Add a provider and distribute
horcrux providers auth local
horcrux providers auth s3 --endpoint s3.amazonaws.com --bucket my-bucket
horcrux distribute

# Restore from providers
horcrux restore
```

---

## Cryptography

| Layer | Algorithm | Key |
|---|---|---|
| Vault files (at rest) | AES-256-GCM | Argon2id(passphrase) |
| Distribution segments | AES-256-GCM (deterministic) | Random 32-byte DEK |
| DEK protection | Shamir's Secret Sharing over GF(2⁸) | M-of-N threshold |
| Data fault tolerance | Reed-Solomon erasure coding | M data + N−M parity shards |
| Passphrase verification | PBKDF2 + HMAC-SHA256 | 100k iterations |

---

## Project Structure

```
horcrux/
├── cmd/cli/                    # CLI application (urfave/cli)
├── gui/                        # macOS GUI (Wails v2 + Svelte 4)
│   ├── app.go                  # Go backend methods
│   └── frontend/src/components/
├── docs/                       # Documentation
├── internal/
│   ├── auth/                   # Touch ID + Keychain (CGo)
│   ├── config/                 # Path configuration
│   ├── crypto/                 # Argon2id + AES-256-GCM
│   ├── distribute/             # Distribution engine
│   │   ├── distribute.go       # Distribute / Restore / GC
│   │   ├── segment/            # Content-addressed segments
│   │   └── manifest/           # Versioned manifest
│   ├── providers/              # Provider config CRUD
│   ├── shamir/                 # GF(256) Secret Sharing
│   └── vault/                  # Vault CRUD + chunked file store
│       └── filestore/          # Content-addressed file chunks
├── storage/                    # 7 provider implementations
└── scripts/                    # Build helpers
```

---

## Build From Source

```bash
# Prerequisites: Go 1.22+, Node 20+, Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# CLI
go build -o /usr/local/bin/horcrux ./cmd/cli/

# GUI
cd gui && wails build -nopackage
# App bundle at gui/build/bin/Horcrux
```

---

## License & Contributing

Contributions, issues, and feature requests are welcome.
