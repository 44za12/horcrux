# Horcrux: CLI Password Manager

Horcrux is a command-line interface (CLI) password manager designed for simplicity and security. It offers an intuitive way to manage your passwords and TOTP configurations directly from the terminal.

## Features

- **Password Management**: Securely store and retrieve passwords.
- **TOTP Support**: Manage Time-based One-Time Passwords (TOTP) configurations.
- **Fuzzy Search**: Quickly find passwords and TOTP configurations using approximate search queries.
- **Password Recovery**: Recover your forgotten master passphrase.
- **Import Functionalities**: Import passwords and TOTP configurations from external sources like CSV and JSON files.

## Usage

### Initial Setup

- To initialize Horcrux, run: `Horcrux init`. You'll be prompted to set a passphrase.

### Password Commands

- **Add a Password**: `Horcrux pass addpass [site] [username] [password]`
- **Remove a Password**: `Horcrux pass removepass [site] [username]`
- **Retrieve a Password**: `Horcrux pass getpass [site] [username]`
- **Import Passwords from CSV**: `Horcrux pass importcsv [CSV file path]`
- **Fuzzy Search for Passwords**: `Horcrux pass fuzzysearch [search query]`

### TOTP Commands

- **Add a TOTP Configuration**: `Horcrux totp addtotp [service] [secretKey]`
- **Get a Current TOTP Code**: `Horcrux totp gettotp [service]`
- **Remove a TOTP Service**: `Horcrux totp removetotp [service]`
- **Fuzzy Search for TOTP Configurations**: `Horcrux totp fuzzysearch [search query]`
- **Import TOTP Configurations from JSON**: `Horcrux totp importtotp [JSON file path]`

### Recovering Forgotten Password

- If you forget your passphrase, use `Horcrux recoverpass` to recover it.

## Importing Passwords from iCloud Keychain

To import passwords from iCloud Keychain into Horcrux:

1. **Export Passwords from iCloud Keychain**: 
   - On your Mac, open the Keychain Access application.
   - Select the items you want to export.
   - Right-click and choose `Export Items...`. Save the file as a `.csv`.

2. **Import into Horcrux**:
   - Use the command: `Horcrux pass importcsv [path to your exported CSV file]`.
   - This will import your passwords from the Keychain into Horcrux.

## Contributing

Contributions, issues, and feature requests are welcome.