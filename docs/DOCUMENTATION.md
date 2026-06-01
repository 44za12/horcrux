<div align="center">

<img src="../logo.png" width="120" height="120" alt="Horcrux Logo" />

# Horcrux

### Distributed, Zero-Trust Secret Manager

*A secret manager that splits your vault across multiple cloud providers using Shamir's Secret Sharing and Reed-Solomon erasure coding — no single point of failure, no single point of compromise.*

---

</div>

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Security Model](#security-model)
- [Data Flow Diagrams](#data-flow-diagrams)
- [Encryption Deep Dive](#encryption-deep-dive)
- [Shamir's Secret Sharing](#shamirs-secret-sharing)
- [Vault Data Model](#vault-data-model)
- [Storage Providers](#storage-providers)
- [GUI Architecture](#gui-architecture)
- [CLI Reference](#cli-reference)
- [Project Structure](#project-structure)
- [Tech Stack](#tech-stack)
- [Build & Install](#build--install)

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────┐
│ 🖥️ Client Layer                                               │
│   Wails v2 GUI (Svelte 4 + Go)  ·  CLI (urfave/cli v2)      │
├──────────────────────────────────────────────────────────────┤
│ 🔐 Authentication                                            │
│   Touch ID (LAContext) → macOS Keychain → Passphrase         │
│   (Touch ID and Keychain are decoupled — see below)          │
├──────────────────────────────────────────────────────────────┤
│ 🧠 Core Engine                                               │
│   Vault Manager (BSON CRUD + Chunked File Store)             │
│   Crypto Engine (Argon2id + AES-256-GCM)                     │
│   Distribution (Content-Addressed Segments + Manifest)       │
│   Shamir's Secret Sharing (GF-256 Lagrange)                  │
│   Reed-Solomon Erasure Coding                                │
├──────────────────────────────────────────────────────────────┤
│ ☁️ 7 Storage Providers                                       │
│   Local · Google Drive · Dropbox · S3/MinIO                  │
│   USB Drive · SSH/SFTP · WebDAV                              │
└──────────────────────────────────────────────────────────────┘
```

---

## Security Model

### Encryption Layers

| Layer | Algorithm | Key Derivation | Parameters | Purpose |
|-------|-----------|---------------|------------|---------|
| **Vault files** | AES-256-GCM | Argon2id | time=3, memory=64MB, threads=4 | Data at rest |
| **Keychain** | AES (system) | Device unlock | `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` | Local storage |
| **Touch ID** | LAContext | Biometric | `LAPolicyDeviceOwnerAuthenticationWithBiometrics` | User auth (separate from keychain) |
| **Passphrase verification** | Argon2id | Argon2id | time=1, memory=32MB, threads=4 | Fast verify on unlock |
| **Distribution segments** | AES-256-GCM | Random 32-byte DEK | Deterministic nonce from plaintext hash | Convergent encryption |
| **DEK protection** | Shamir's Secret Sharing | GF(2⁸) Lagrange | M-of-N threshold | Key security |
| **Data fault tolerance** | Reed-Solomon | — | M data + N−M parity | Provider failure tolerance |

### Key Principles

1. **Zero-knowledge servers**: Providers see only opaque encrypted blobs
2. **Threshold security**: M of N providers needed to restore (M = max(2, min(3, N-2)))
3. **Decoupled biometrics**: Touch ID authenticates; Keychain stores. No biometric constraint on keychain entries means storage never fails on ad-hoc signed builds.
4. **No hardcoded keys**: All keys derived from user passphrase or randomly generated per-operation
5. **Forward migration**: Legacy PBKDF2-encrypted files auto-detected and read transparently
6. **Content-addressed segments**: Identical plaintext produces identical ciphertext — incremental distribution with zero re-upload for unchanged data

---

## Data Flow Diagrams

### First Launch — Vault Creation

```
User → LockScreen → CreateVaultWithPassphrase(pass)
  → InitBSONFiles (creates empty .hrcrx files)
  → storeVerification (Argon2id hash)
  → StorePassphraseLocal (Keychain, no biometric constraint)
  → Unlock
```

### Unlock (Returning User)

**With Touch ID:**
```
Launch → IsInitialized() → HasBiometricKey()
  → AuthenticateTouchID() → system Touch ID prompt
  → GetPassphraseLocal() → read from Keychain
  → VerifyPassphrase → Unlock
```

**Without Touch ID (or biometric unavailable):**
```
Launch → IsInitialized() → HasBiometricKey() → false
  → Show passphrase input
  → UnlockWithPassphrase → StorePassphraseLocal (Keychain)
  → Unlock
```

### Distribute — Incremental Backup

```
User clicks "Distribute"
  → Decrypt vault files into plaintext
  → Pack into 16MB content-addressed segments
  → Compare segment hashes against previous distribution state
  → Generate DEK, Shamir-split into N shares

  For NEW segments only:
    → Encrypt with DEK (deterministic nonce from plaintext hash)
    → Erasure-encode into N shards (M data + N-M parity)
    → Upload ALL shards to ALL providers as seg.<hash>.<idx>.hrcrx

  → Upload versioned manifest (manifest.v<N>.hrcrx) to all providers
  → Upload DEK shares (dekshare.v<N>.hrcrx), one per provider
  → Save local distribution state

Unchanged segments: zero bytes uploaded.
```

### Restore — Recovery

```
User clicks "Restore"
  → Find latest manifest version across providers (using List)
  → Download manifest from any provider
  → Collect M-of-N DEK shares, Shamir-combine → DEK

  For each segment in manifest:
    → For each shard index (0..N-1), search all providers
    → Collect M shards, erasure-decode → ciphertext
    → Decrypt with DEK → plaintext entries

  → Write vault files (re-encrypt with current passphrase)
  → Rebuild file store index from chunk entries
```

---

## Encryption Deep Dive

### Envelope Format (Argon2id — current)

All vault files and file chunks use a custom binary envelope:

```
┌──────────┬──────────┬──────────┬─────┬──────────┬─────┬──────┬─────┬───────┬────────────┐
│ HCRX\x01 │ time(u32)│ mem(u32) │ thr │ keyLen   │sLen │ salt │ nLen │ nonce │ ciphertext │
│ 5 bytes  │ 4 bytes  │ 4 bytes  │ 1B  │ 4 bytes  │ 1B  │ 16B  │ 1B  │ 12B   │ + 16B tag  │
└──────────┴──────────┴──────────┴─────┴──────────┴─────┴──────┴─────┴───────┴────────────┘
      │
      └──► Argon2id(passphrase, salt, time, memory, threads) ──► 32-byte AES key
```

Parameters are embedded per-file — change them in a future version and old files still decrypt.

### Segment Encryption (Convergent)

Distribution segments use deterministic encryption for content-addressing:

```
nonce = SHA-256(plaintext)[:12]
ciphertext = AES-256-GCM(DEK, nonce, plaintext)

Same plaintext → same nonce → same ciphertext → same hash → skip upload.
```

### Legacy Format (PBKDF2 — read-only)

Auto-detected by absence of `HCRX\x01` header. Rewritten to Argon2id on any write.

---

## Shamir's Secret Sharing

The DEK is split using Shamir's Secret Sharing over GF(2⁸) with the standard AES irreducible polynomial (0x1B).

```
Split:
  DEK (32 bytes) → random polynomial degree m-1 → evaluate at x=1..n → N shares

Combine:
  Any M shares → Lagrange interpolation at x=0 → DEK
```

### Threshold Table

```
M = max(2, min(3, N - 2))
```

| Providers (N) | Threshold (M) | Tolerated Failures |
|:---:|:---:|:---:|
| 3 | 2 | 1 |
| 4 | 2 | 2 |
| 5 | 3 | 2 |
| 6 | 3 | 3 |
| 7 | 3 | 4 |

### Reed-Solomon Erasure Coding

Each segment is independently erasure-coded. M data shards + N−M parity shards = N total. Any M shards reconstruct the segment.

All shards are uploaded to all providers, so any M providers can supply the needed shards regardless of provider ordering.

---

## Vault Data Model

### File Layout on Disk

```
~/.horcrux/
├── passes.hrcrx         Passwords          (Argon2id + AES-256-GCM + BSON)
├── totp.hrcrx           TOTP secrets       (Argon2id + AES-256-GCM + BSON)
├── apikeys.hrcrx        API keys           (Argon2id + AES-256-GCM + BSON)
├── providers.hrcrx      Provider configs   (Argon2id + AES-256-GCM + JSON)
├── mainpass.hrcrx       Passphrase hash    (JSON: salt + Argon2id digest)
├── distribution-state.json  Local distribution tracking (encrypted)
├── distributed/         Local provider storage
└── files/               Chunked file store
    ├── index.hrcrx      File metadata index    (Argon2id + AES-256-GCM + BSON)
    └── chunks/          Content-addressed chunks
        ├── <sha256_1>   4MB chunk (individually encrypted)
        ├── <sha256_2>
        └── ...
```

### BSON Data Structures

**Passwords:** `map[site]map[username]value` — value is either `"password"` or `{"p":"password","n":"notes"}`

**TOTP:** `map["totp"]map[service]base32secret`

**API Keys:** `map[service]map[name]value`

**File Index:** `map[filename]{mime_type, total_size, chunk_hashes}`

---

## Storage Providers

### Interface

```go
type Provider interface {
    Name() string
    Authenticate(ctx context.Context) error
    Upload(ctx context.Context, key string, data []byte) error
    Download(ctx context.Context, key string) ([]byte, error)
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
    List(ctx context.Context, prefix string) ([]string, error)
}
```

`List` enables manifest version discovery and garbage collection by scanning provider contents for known key prefixes.

### Provider Details

| Provider | Storage | Auth | Notes |
|----------|---------|------|-------|
| **Local** | `BaseDir/key` (0600) | None | Default provider |
| **Google Drive** | Drive API v3 | OAuth2 PKCE | Auto-refreshes tokens |
| **Dropbox** | API v2 | OAuth2 PKCE | Auto-refreshes on 401 |
| **S3 / MinIO** | `horcrux/` prefix | Access + secret key | Auto-creates bucket |
| **USB** | `.horcrux/` subdir | Writable check | Validates mount |
| **SSH / SFTP** | Remote path | Password or key | crypto/ssh + sftp |
| **WebDAV** | `/horcrux/` path | Basic auth | PROPFIND, MKCOL, PUT, GET, DELETE |

### Provider Object Keys

| Object | Key Format | Per-Provider? |
|--------|-----------|---------------|
| Manifest | `manifest.v<N>.hrcrx` | Same on all providers (replicated) |
| DEK Share | `dekshare.v<N>.hrcrx` | Different share per provider (same key name) |
| Segment Shard | `seg.<sha256>.<idx>.hrcrx` | All shards on all providers |

---

## GUI Architecture

### Components

```
App.svelte (root)
├── LockScreen.svelte     — Touch ID / Create vault / Passphrase input
├── VaultList.svelte      — Password CRUD with search, reveal, copy
├── TotpList.svelte       — Live TOTP codes with countdown ring
├── ApiKeyList.svelte     — API key CRUD
├── FileList.svelte       — Encrypted file upload/download
├── Import.svelte         — CSV passwords, 2FAS JSON TOTP
├── Providers.svelte      — 7 provider types, dynamic forms
└── DistributeRestore.svelte — Stats, distribute, restore
```

### Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `⌘1` | Passwords |
| `⌘2` | Authenticator |
| `⌘3` | API Keys |
| `⌘4` | Files |
| `⌘5` | Import |
| `⌘6` | Providers |
| `⌘7` | Distribute |
| `⌘L` | Lock Vault |
| `⌘N` | Add New Entry |
| `⌘F` | Focus Search |

---

## CLI Reference

```
horcrux init                          Initialize vault
horcrux pass addpass <site> <user> <pass>
horcrux pass getpass <site> <user>
horcrux pass removepass <site> <user>
horcrux pass importcsv <file.csv>
horcrux pass fuzzysearch <query>

horcrux totp addtotp <service> <secret>
horcrux totp gettotp <service>
horcrux totp removetotp <service>
horcrux totp importtotp <file.json>
horcrux totp fuzzysearch <query>

horcrux distribute                    Distribute vault to providers
horcrux restore                       Restore vault from providers
horcrux change-passphrase             Change master passphrase

horcrux providers auth <type>         Add a provider (gdrive|dropbox|s3|usb|ssh|webdav|local)
horcrux providers list                List configured providers
horcrux providers remove <name>       Remove a provider
```

---

## Project Structure

```
horcrux/
├── cmd/cli/                          # CLI application
│   ├── main.go
│   └── commands.go
│
├── gui/                              # macOS GUI (Wails v2)
│   ├── main.go                       # Window config
│   ├── app.go                        # All Go methods bound to frontend
│   ├── wails.json
│   └── frontend/
│       └── src/
│           ├── App.svelte            # Root: sidebar + routing
│           └── components/           # 8 Svelte components
│
├── docs/                             # Documentation
│   ├── README.md
│   ├── ARCHITECTURE.md
│   └── DOCUMENTATION.md
│
├── internal/
│   ├── auth/                         # Authentication (macOS CGo)
│   │   ├── keychain_darwin.go        # Keychain (CGo + Security.framework)
│   │   └── touchid_darwin.go         # Touch ID (CGo + LocalAuthentication)
│   ├── config/
│   │   ├── config.go                 # Path configuration
│   │   └── config_test.go
│   ├── crypto/
│   │   ├── crypto.go                 # Argon2id + AES-256-GCM
│   │   └── crypto_test.go
│   ├── distribute/                   # Distribution engine
│   │   ├── distribute.go             # Distribute / Restore / GC
│   │   ├── distribute_test.go
│   │   ├── segment/                  # Content-addressed segments
│   │   │   ├── segment.go            # Segment type + crypto
│   │   │   └── packer.go             # Vault ↔ segments serialization
│   │   └── manifest/                 # Versioned manifest
│   │       └── manifest.go           # Manifest + DEK share management
│   ├── providers/
│   │   └── providers.go              # Provider config CRUD + threshold
│   ├── shamir/
│   │   ├── shamir.go                 # GF(256) Split + Combine
│   │   └── shamir_test.go
│   ├── vault/
│   │   ├── vault.go                  # Password, TOTP, API key, file CRUD
│   │   ├── vault_test.go
│   │   └── filestore/                # Chunked file store
│   │       ├── index.go              # Encrypted file metadata index
│   │       └── store.go              # Chunk CRUD, streaming, GC
│   ├── audit/                        # Operation audit log
│   └── storagemock/                  # Mock provider for testing
│
├── storage/                          # Provider implementations
│   ├── storage.go                    # Provider interface (7 methods)
│   ├── local.go
│   ├── gdrive.go
│   ├── dropbox.go
│   ├── s3.go
│   ├── usb.go
│   ├── ssh.go
│   └── webdav.go
│
├── scripts/
│   └── build-dmg.sh
│
├── .github/workflows/
│   ├── ci.yml
│   └── release.yml
│
├── go.mod
├── go.sum
└── logo.png
```

---

## Tech Stack

| Category | Technology | Version |
|----------|-----------|---------|
| **Backend** | Go | 1.22 |
| **GUI Framework** | Wails | v2.12 |
| **Frontend** | Svelte | 4 |
| **Build** | Vite | 5 |
| **Biometric** | Apple LocalAuthentication (CGo) | — |
| **Encryption** | AES-256-GCM + Argon2id | — |
| **Secret Sharing** | Shamir over GF(2⁸) | Custom |
| **Erasure Coding** | Reed-Solomon | klauspost v1.12 |
| **Serialization** | BSON | mongo-driver v1.13 |
| **S3** | MinIO SDK | v7.0 |
| **SSH** | golang.org/x/crypto/ssh | v0.33 |
| **OAuth2** | golang.org/x/oauth2 | v0.15 |
| **CLI** | urfave/cli | v2.26 |
| **Search** | fuzzysearch | v1.1 |
| **CI/CD** | GitHub Actions | — |

---

## Build & Install

### Prerequisites

```bash
# Go 1.22+
go version

# Node.js 20+
node --version

# Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# macOS: Xcode Command Line Tools
xcode-select --install
```

### Build CLI

```bash
CGO_ENABLED=1 go build -o /usr/local/bin/horcrux ./cmd/cli/
```

### Build GUI (macOS)

```bash
cd gui
wails build -nopackage

# Assemble app bundle
mkdir -p Horcrux.app/Contents/{MacOS,Resources}
cp build/bin/Horcrux Horcrux.app/Contents/MacOS/
cp frontend/src/logo.png Horcrux.app/Contents/Resources/iconfile.png

# Create Info.plist (see docs/ARCHITECTURE.md for template)

# Install
cp -R Horcrux.app /Applications/
```

### Development Mode

```bash
cd gui
wails dev
```

---

<div align="center">

*Horcrux — Your secrets, split across the world, recoverable by you alone.*

</div>
