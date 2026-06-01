<div align="center">

<img src="../logo.png" width="100" height="100" alt="Horcrux Logo" />

# Horcrux — Architecture

*How the pieces fit together.*

---

</div>

## The Problem

Password managers encrypt your vault and store it with a single provider — typically the vendor's own cloud, a self-hosted server, or a local file. The encryption keeps the provider from reading your data. But your vault is still one copy, in one place, under one party's control. If that provider loses data, has an outage, suspends your account, or their ciphertext is exfiltrated in a breach, you have a problem.

Horcrux takes a different approach: it **splits** your vault and scatters the pieces across multiple cloud providers that you choose and control. You need a threshold number of them to reconstruct your vault — no single provider holds anything meaningful on its own. You can lose providers without losing access to your data, and no single breach exposes your vault.

---

## Mental Model

Three layers stacked on top of each other:

```
┌─────────────────────────────────────────┐
│              Interface Layer             │
│         CLI (terminal) / GUI (app)       │
├─────────────────────────────────────────┤
│              Core Engine                 │
│   Vault ←→ Crypto ←→ Distribution       │
├─────────────────────────────────────────┤
│           Storage Providers              │
│  Local · Google Drive · Dropbox · S3 ·   │
│  USB · SSH · WebDAV                      │
└─────────────────────────────────────────┘
```

The **Interface Layer** is what the user touches. The **Core Engine** does all the real work — encrypting data, splitting keys, managing the vault. The **Storage Providers** are dumb pipes — they receive opaque blobs and store them. They never see plaintext.

---

## The Core Engine

### 1. Vault Manager

The vault is a set of encrypted files on disk, each holding a different category of data:

| File | Contents | Storage |
|------|----------|---------|
| `passes.hrcrx` | Site + username + password pairs | BSON, full-file encrypt |
| `totp.hrcrx` | TOTP service names and Base32 secrets | BSON, full-file encrypt |
| `apikeys.hrcrx` | Service + name + API key pairs | BSON, full-file encrypt |
| `files/index.hrcrx` | File metadata index | BSON, full-file encrypt |
| `files/chunks/<sha256>` | Content-addressed file chunks | 4MB chunks, individually encrypted |
| `providers.hrcrx` | Cloud provider configurations | JSON, full-file encrypt |
| `mainpass.hrcrx` | Passphrase verification hash | JSON, plain |

Passwords, TOTP codes, and API keys use the simple full-decrypt → modify → full-encrypt cycle. These files are tiny even at scale (millions of entries would still be under 100MB).

**Chunked File Store** — Arbitrary files are stored as content-addressed chunks (4MB default). Each chunk is individually encrypted with the vault passphrase. The index maps `filename → [sha256_chunk_hash...]`. This means:
- Adding a file only encrypts its chunks, not the entire store
- Identical content across files shares chunks automatically (hash-based dedup)
- GB-scale files don't require holding everything in memory

### 2. Crypto Engine

All encryption flows through one subsystem:

**Vault encryption (Argon2id):** A custom binary envelope stores Argon2id parameters alongside AES-256-GCM ciphertext. Parameters are embedded per-file, so the format can evolve without breaking old files.

**Legacy (PBKDF2):** Older files detected by absence of the envelope magic header. Read-only; rewritten to Argon2id on any write operation.

**Distribution encryption:** Segments are encrypted with a random 32-byte Data Encryption Key (DEK) using AES-256-GCM with deterministic nonces derived from the plaintext hash. This is convergent encryption — identical plaintext always produces identical ciphertext, enabling content-addressed deduplication across distributions.

### 3. Distribution System

This is where Horcrux differs from every other password manager. Distribution is a **two-tier content-addressed pipeline**:

```
┌─────────────────────────────────────────────────────────────┐
│ Tier 1 — Manifest (small, ~KB, always fully redistributed)  │
│                                                             │
│   Vault plaintext → Segments → Segment hashes               │
│   Manifest = {version, timestamp, segment_refs, file_index} │
│   DEK is Shamir-split, one share uploaded per provider      │
│   Manifest fully replicated to all providers                │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ Tier 2 — Data Segments (16MB, content-addressed, immutable) │
│                                                             │
│   Each segment: serialized vault entries                    │
│   Encrypted with DEK (deterministic nonce)                  │
│   Erasure-coded: M data shards + N-M parity shards         │
│   ALL shards uploaded to ALL providers                      │
│   Segment hash = SHA-256(plaintext) → unchanged = skip     │
└─────────────────────────────────────────────────────────────┘
```

**Incremental distribution:** The local distribution state tracks which segment hashes were uploaded in the previous distribution. On the next distribute, only segments with new hashes are encrypted, erasure-coded, and uploaded. Identical content produces the same hash → same ciphertext → same provider key → zero bytes transferred.

**Why both Shamir AND Reed-Solomon?** They solve different problems:
- **Shamir** protects the *encryption key* (DEK). Knowing one share tells you nothing. Need M to reconstruct.
- **Reed-Solomon** protects the *data*. Adds parity shards so missing fragments can be reconstructed from any M-of-N.

**Why all shards to all providers?** Earlier versions mapped shard[i] → provider[i], which broke if providers were reordered (e.g., after deleting `~/.horcrux` and re-adding providers in a different order). Now every provider stores every shard, so any M providers can reconstruct regardless of ordering or availability. The space cost is modest: each provider stores `N × (segment_size / M)` bytes per segment.

### Threshold Calculation

```
M = max(2, min(3, N - 2))
```

| Providers | Threshold | Can Lose |
|-----------|-----------|----------|
| 3 | 2 | 1 |
| 4 | 2 | 2 |
| 5 | 3 | 2 |
| 6 | 3 | 3 |
| 7 | 3 | 4 |

---

## The Interface Layer

### GUI (macOS Native App)

Built with Wails v2 — Go backend, Svelte frontend. The Go backend holds the passphrase in memory for the session.

**Unlock flow:**
- **Biometric:** Touch ID authenticates the user first. On success, the passphrase is read from macOS Keychain. Touch ID and keychain are decoupled — the keychain entry has no biometric constraint, so storage never fails on ad-hoc signed builds.
- **Manual passphrase:** Fallback when biometrics aren't available or configured. After first successful unlock, the passphrase is stored in Keychain for future Touch ID unlocks.

**Lock:** Clears the in-memory passphrase. On next unlock, `HasBiometricKey()` checks both biometric availability and keychain entry existence. If both true, Touch ID is offered. If either is false, passphrase input is shown directly.

### CLI

A terminal interface using `urfave/cli`. Follows the same unlock flow. All commands are subcommand-structured.

---

## Storage Providers

Providers implement a simple seven-method interface:

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

The `List` method enables garbage collection and manifest version discovery by scanning provider contents.

| Provider | Transport | Auth |
|----------|-----------|------|
| Local | Filesystem (0600) | None |
| Google Drive | REST API v3 | OAuth2 PKCE |
| Dropbox | REST API v2 | OAuth2 PKCE |
| S3 / MinIO | MinIO SDK | Access key + secret key |
| USB | Filesystem (0600) | None (writable check) |
| SSH / SFTP | crypto/ssh + sftp | Password or key pair |
| WebDAV | HTTP (PUT/GET/DELETE/PROPFIND) | Basic auth |

---

## Authentication Architecture

The passphrase is the root secret. Touch ID and Keychain are **decoupled** — Touch ID authenticates the user, Keychain stores the passphrase without biometric constraints. This avoids the keychain access control issues that plague ad-hoc signed macOS apps.

```
Touch ID → authenticate user → read passphrase from Keychain → verify → unlock
```

- **Keychain storage:** `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` — encrypted at rest when device is locked, readable without biometric prompt.
- **Biometric check:** `LAPolicyDeviceOwnerAuthenticationWithBiometrics` — called separately before keychain read. If unavailable (no Touch ID sensor), the app skips biometrics entirely.
- **Passphrase verification:** Argon2id hash stored in `mainpass.hrcrx` for fast verification.

---

## Data Lifecycle

### First Launch

```
User enters passphrase
    → Store Argon2id verification hash
    → Create empty encrypted vault files
    → Store passphrase in Keychain
    → Unlock
```

### Normal Operation

```
User adds a password
    → Decrypt passes.hrcrx (Argon2id → AES-256-GCM)
    → Insert into BSON map
    → Re-encrypt and overwrite
```

### Distribute (Incremental Backup)

```
User clicks "Distribute"
    → Decrypt vault files into plaintext
    → Pack into content-addressed segments (16MB)
    → Diff against previous distribution state
    → Generate new DEK, Shamir-split
    → For NEW segments only:
        → Encrypt with DEK (deterministic nonce)
        → Erasure-encode → N shards
        → Upload all shards to all providers
    → Upload versioned manifest + DEK shares to all providers
    → Save local distribution state
```

### Restore (Recovery)

```
User clicks "Restore"
    → Find latest manifest version across providers
    → Download manifest from any provider
    → Collect M-of-N DEK shares, reconstruct DEK
    → For each segment in manifest:
        → Search all providers for each shard index
        → Collect M shards, erasure-decode
        → Decrypt with DEK
    → Write vault files (re-encrypt with current passphrase)
    → Rebuild file store index
```

---

## Key Design Decisions

**BSON for vault storage.** Efficient binary serialization for structured data. JSON used only for config files where human-readability matters.

**One file per category.** Limits blast radius of corruption. Allows partial recovery. Decrypting passwords doesn't require decrypting files.

**Content-addressed segments for distribution.** Enables incremental uploads — unchanged data produces identical ciphertext, so it's never re-uploaded. GB-scale vaults distribute in seconds after the initial upload.

**All shards to all providers.** Eliminates positional coupling between provider order and shard assignment. Any M providers can reconstruct, regardless of how they were configured.

**Deterministic segment encryption.** Nonce derived from plaintext hash. Convergently secure — same plaintext always produces same ciphertext, enabling cross-distribution dedup.

**Separate Touch ID from Keychain.** Keychain stores without biometric constraints (avoids ad-hoc signing issues). Touch ID called as explicit separate step before keychain read.

**Wails over Electron.** Smaller binary, lower memory footprint, native macOS integration for biometrics and Keychain.

---

<div align="center">

*Horcrux — Split your secrets across the world, recoverable by you alone.*

</div>
