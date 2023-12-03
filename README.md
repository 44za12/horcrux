# horcrux: CLI Password Manager

horcrux is a command-line interface (CLI) password manager designed for simplicity and security. It offers an intuitive way to manage your passwords and TOTP configurations directly from the terminal.

## Features

- **Password Management**: Securely store and retrieve passwords.
- **TOTP Support**: Manage Time-based One-Time Passwords (TOTP) configurations.
- **Fuzzy Search**: Quickly find passwords and TOTP configurations using approximate search queries.
- **Password Recovery**: Recover your forgotten master passphrase.
- **Import Functionalities**: Import passwords and TOTP configurations from external sources like CSV and JSON files.

## Technical Details

horcrux employs a range of cryptographic techniques and algorithms to ensure the security of your data:

- **AES Encryption**: For the secure storage of passwords and TOTP configurations, Horcrux uses Advanced Encryption Standard (AES) in GCM mode. This choice provides both confidentiality and integrity of stored data.
- **PBKDF2 for Key Derivation**: The encryption key for AES is derived using PBKDF2 (Password-Based Key Derivation Function 2) with a SHA-256 hash function. This adds a layer of security against brute-force attacks by making it computationally expensive to try multiple passwords.
- **BSON Data Format**: Data is serialized in BSON (Binary JSON) format before being encrypted. BSON is chosen for its efficiency in both space and speed, which is beneficial for storing structured data.
- **HMAC-SHA1 for TOTP**: Time-based One-Time Passwords (TOTPs) are generated using HMAC-SHA1 algorithm, complying with the TOTP standard (RFC 6238). This method is widely adopted for two-factor authentication.
- **Fuzzy Search**: For user convenience, Horcrux provides a fuzzy search feature, allowing users to find their stored credentials even with approximate queries. It uses a fuzzy string matching algorithm to search through the titles and usernames.
- **Cross-Platform Compatibility**: Horcrux is built in Go, ensuring it runs smoothly across different operating systems including Windows, macOS, and Linux.

## Installation

Horcrux is available for Windows, macOS, and Linux. You can install it by downloading the appropriate binary for your operating system from the GitHub Releases page.

## Downloading the Binary

1. **Go to the Releases Page**: Visit the [Horcrux Releases page](https://github.com/[44za12]/horcrux/releases) on GitHub.
   
2. **Download the Binary**: Download the binary for your operating system:
   - For Windows: `horcrux-windows-amd64.exe`
   - For macOS: `horcrux-darwin-amd64`
   - For Linux: `horcrux-linux-amd64`

3. **Make the Binary Executable** (macOS/Linux):
   - Open a terminal.
   - Navigate to the directory where you downloaded Horcrux.
   - Run the command `chmod +x horcrux-darwin-amd64` or `chmod +x horcrux-linux-amd64` to make the file executable.

## Installation Steps

### Windows

- After downloading, you can run `horcrux-windows-amd64.exe` directly from the command prompt.

### macOS

- Move `horcrux-darwin-amd64` to a directory in your `PATH` (e.g., `/usr/local/bin`).
- Rename the file for convenience: `mv horcrux-darwin-amd64 horcrux`.
- Run `horcrux` from the terminal.

### Linux

- Move `horcrux-linux-amd64` to a directory in your `PATH` (e.g., `/usr/local/bin`).
- Rename the file for convenience: `mv horcrux-linux-amd64 horcrux`.
- Run `horcrux` from the terminal.

## Usage

### Initial Setup

- To initialize horcrux, run: `horcrux init`. You'll be prompted to set a passphrase.

### Password Commands

- **Add a Password**: `horcrux pass addpass [site] [username] [password]`
- **Remove a Password**: `horcrux pass removepass [site] [username]`
- **Retrieve a Password**: `horcrux pass getpass [site] [username]`
- **Import Passwords from CSV**: `horcrux pass importcsv [CSV file path]`
- **Fuzzy Search for Passwords**: `horcrux pass fuzzysearch [search query]`

### TOTP Commands

- **Add a TOTP Configuration**: `horcrux totp addtotp [service] [secretKey]`
- **Get a Current TOTP Code**: `horcrux totp gettotp [service]`
- **Remove a TOTP Service**: `horcrux totp removetotp [service]`
- **Fuzzy Search for TOTP Configurations**: `horcrux totp fuzzysearch [search query]`
- **Import TOTP Configurations from JSON**: `horcrux totp importtotp [JSON file path]`

### Recovering Forgotten Password

- If you forget your passphrase, use `horcrux recoverpass` to recover it.

## Importing Passwords from iCloud Keychain

To import passwords from iCloud Keychain into horcrux:

Go to System Preferences > Passwords and authenticate with your admin password or Touch ID. Then, click the three-dotted Menu button in the bottom toolbar, and choose the “Export Passwords” option.

1. **Export Passwords from iCloud Keychain**: 
   - On your Mac, go to System Preferences > Passwords.
   - Click the three-dotted Menu button in the bottom toolbar, and choose the “Export Passwords” option.
   - Save the file preferably in the same directory as horcrux.

2. **Import into horcrux**:
   - Use the command: `horcrux pass importcsv [path to your exported CSV file]`.
   - This will import your passwords from the Keychain into horcrux.

## Contributing

Contributions, issues, and feature requests are welcome.