package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"

	"horcrux/internal/auth"
	"horcrux/internal/config"
	"horcrux/internal/crypto"
	"horcrux/internal/distribute"
	"horcrux/internal/providers"
	"horcrux/internal/vault"

	"horcrux/storage"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh/terminal"
)

func getPassphraseInput(prompt string) string {
	fmt.Print(prompt)
	passphraseBytes, err := terminal.ReadPassword(0)
	if err != nil {
		log.Fatal("Failed to read passphrase")
	}
	fmt.Println()
	return string(passphraseBytes)
}

func getPassphrase() string {
	if auth.HasLocalKey() {
		err := auth.AuthenticateTouchID("Unlock your Horcrux vault")
		if err == nil {
			pass, err := auth.GetPassphraseLocal()
			if err == nil {
				return pass
			}
		}
		fmt.Println("Touch ID unavailable or cancelled, enter passphrase.")
	}
	return getPassphraseInput("Enter passphrase: ")
}

func initCommand(c *cli.Context) error {
	passphrase := getPassphraseInput("Enter your passphrase for initialization: ")
	recoveryString, err := vault.InitBSONFiles(passphrase)
	if err != nil {
		return err
	}
	fmt.Printf("Initialization successful. Recovery phrase: %s\n", recoveryString)
	return nil
}

func addPassCommand(c *cli.Context) error {
	if c.NArg() < 3 {
		return fmt.Errorf("missing arguments: [site] [username] [password]")
	}
	site := c.Args().Get(0)
	username := c.Args().Get(1)
	password := c.Args().Get(2)

	passphrase := getPassphrase()
	if err := vault.AddUpdatePasswordOnly(site, username, password, passphrase); err != nil {
		return err
	}
	fmt.Println("Password added successfully")
	return nil
}

func removePassCommand(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("missing arguments: [site] [username]")
	}
	site := c.Args().Get(0)
	username := c.Args().Get(1)

	passphrase := getPassphrase()
	if err := vault.RemovePassword(site, username, passphrase); err != nil {
		return err
	}
	fmt.Println("Password removed successfully")
	return nil
}

func getPassCommand(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("missing arguments: [site] [username]")
	}
	site := c.Args().Get(0)
	username := c.Args().Get(1)

	passphrase := getPassphrase()
	password, _, err := vault.GetPassword(site, username, passphrase)
	if err != nil {
		return err
	}
	fmt.Printf("Password for %s:%s is '%s'\n", site, username, password)
	return nil
}

func recoverPass(c *cli.Context) error {
	return fmt.Errorf("recovery is no longer supported: the master passphrase is never stored. Reset requires re-initialization")
}

func completionCommand(c *cli.Context) error {
	shell := c.Args().First()
	if shell == "" {
		return fmt.Errorf("shell type required: bash, zsh, or fish")
	}
	switch shell {
	case "bash":
		fmt.Println("# To load completions:")
		fmt.Println("#   source <(horcrux completion bash)")
		fmt.Println("")
		// urfave/cli v2 EnableBashCompletion provides completion via --generate-bash-completion flag
		// This wrapper script invokes that mechanism
		fmt.Println(`_horcrux_bash_completion() {
    COMPREPLY=($(horcrux --generate-bash-completion "${COMP_WORDS[@]}" 2>/dev/null))
}
complete -F _horcrux_bash_completion horcrux`)
	case "zsh":
		fmt.Println("# To load completions:")
		fmt.Println("#   source <(horcrux completion zsh)")
		fmt.Println("")
		fmt.Println(`_horcrux_zsh_completion() {
    local words
    words=(${(z)BUFFER})
    reply=($(horcrux --generate-bash-completion "${words[@]}" 2>/dev/null))
}
compctl -K _horcrux_zsh_completion horcrux`)
	case "fish":
		fmt.Println("# To load completions:")
		fmt.Println("#   horcrux completion fish | source")
		fmt.Println("")
		fmt.Println(`function __horcrux_completions
    commandline -opc | xargs horcrux --generate-bash-completion 2>/dev/null
end
complete -f -c horcrux -a '(__horcrux_completions)'`)
	default:
		return fmt.Errorf("unsupported shell: %s (use bash, zsh, or fish)", shell)
	}
	return nil
}

func changePassphraseCommand(c *cli.Context) error {
	fmt.Println("Changing your vault passphrase...")
	oldPassphrase := getPassphraseInput("Enter current passphrase: ")
	if !vault.VerifyPassphrase(oldPassphrase) {
		return fmt.Errorf("incorrect passphrase")
	}
	newPassphrase := getPassphraseInput("Enter new passphrase: ")
	confirmPassphrase := getPassphraseInput("Confirm new passphrase: ")
	if newPassphrase != confirmPassphrase {
		return fmt.Errorf("passphrases do not match")
	}
	if err := vault.ChangePassphrase(oldPassphrase, newPassphrase); err != nil {
		return fmt.Errorf("changing passphrase: %w", err)
	}
	fmt.Println("Passphrase changed successfully. Re-distribute your vault if you have active fragments.")
	return nil
}

func getTotpCommand(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing argument: [service]")
	}
	service := c.Args().Get(0)

	passphrase := getPassphrase()
	totp, err := vault.GetTOTP(service, passphrase)
	if err != nil {
		return err
	}
	fmt.Printf("Current TOTP for %s is: %06d\n", service, totp)
	return nil
}

func addTotpCommand(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("missing arguments: [service] [secretKey]")
	}
	service := c.Args().Get(0)
	secretKey := c.Args().Get(1)

	passphrase := getPassphrase()
	if err := vault.AddUpdateTOTP(service, secretKey, passphrase); err != nil {
		return err
	}
	fmt.Println("TOTP configuration added successfully")
	return nil
}

func removeTotpCommand(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing arguments: [service]")
	}
	service := c.Args().Get(0)

	passphrase := getPassphrase()
	if err := vault.RemoveTOTP(service, passphrase); err != nil {
		return err
	}
	fmt.Println("TOTP configuration removed successfully")
	return nil
}

func importFromCSV(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing argument: [CSV file path]")
	}
	filePath := c.Args().Get(0)
	passphrase := getPassphrase()
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %s", err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV file: %s", err)
	}
	for i, record := range records {
		if i == 0 {
			continue
		}
		if len(record) < 4 {
			continue
		}
		title := record[0]
		url := record[1]
		username := record[2]
		password := record[3]
		var notes string
		if len(record) >= 5 {
			notes = record[4]
		}
		site := title
		if site == "" {
			site = url
		}
		if site != "" && username != "" && password != "" {
			if err := vault.AddUpdatePassword(site, username, password, notes, passphrase); err != nil {
				return err
			}
		}
	}
	fmt.Println("Passwords imported successfully")
	return nil
}

type TOTPImportService struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
	OTP    struct {
		Account string `json:"account"`
	} `json:"otp"`
}

type TOTPImport struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Services      []TOTPImportService `json:"services"`
}

func importTotpCommand(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing argument: [JSON file path]")
	}
	jsonFilePath := c.Args().Get(0)

	jsonData, err := os.ReadFile(jsonFilePath)
	if err != nil {
		return fmt.Errorf("error reading JSON file: %s", err)
	}

	var totpImport TOTPImport
	if err := json.Unmarshal(jsonData, &totpImport); err != nil {
		return fmt.Errorf("error unmarshaling JSON: %s", err)
	}

	passphrase := getPassphrase()

	for _, service := range totpImport.Services {
		serviceName := service.Name
		if service.OTP.Account != "" {
			serviceName += " (" + service.OTP.Account + ")"
		}
		if err := vault.AddUpdateTOTP(serviceName, service.Secret, passphrase); err != nil {
			return err
		}
	}

	fmt.Println("TOTP configurations imported successfully")
	return nil
}

func fuzzySearchPassCommand(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing argument: [search query]")
	}
	query := c.Args().Get(0)
	passphrase := getPassphrase()
	storedData, err := crypto.DecryptBSONFile(config.PassesPath(), passphrase)
	if err != nil {
		return err
	}
	for site, credentials := range storedData {
		if fuzzy.MatchFold(query, site) {
			fmt.Printf("Site: %s\n", site)
			for username := range credentials {
				fmt.Printf("  Username: %s\n", username)
				fmt.Printf("  Password: %s\n", credentials[username])
			}
		} else {
			for username := range credentials {
				if fuzzy.MatchFold(query, username) {
					fmt.Printf("Site: %s\n", site)
					fmt.Printf("  Username: %s\n", username)
					fmt.Printf("  Password: %s\n", credentials[username])
				}
			}
		}
	}
	return nil
}

func fuzzySearchTOTPCommand(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing argument: [search query]")
	}
	query := c.Args().Get(0)
	passphrase := getPassphrase()
	storedData, err := crypto.DecryptBSONFile(config.TotpPassPath(), passphrase)
	if err != nil {
		return err
	}
	for service := range storedData["totp"] {
		if fuzzy.MatchFold(query, service) {
			code, _ := vault.GenerateTOTP(storedData["totp"][service])
			fmt.Printf("Site: %s, OTP: %06d\n", service, code)
		}
	}
	return nil
}

func distributeCommand(c *cli.Context) error {
	passphrase := getPassphrase()

	pf, err := providers.LoadConfig(passphrase)
	if err != nil {
		return fmt.Errorf("loading providers config: %w", err)
	}

	if len(pf.Providers) == 0 {
		return fmt.Errorf("no providers configured. Run 'horcrux providers auth' to add providers")
	}

	nonLocal := providers.CountNonLocal(pf)
	if len(pf.Providers) < 3 {
		return fmt.Errorf("need at least 3 providers total (including local), but only have %d. Add more with 'horcrux providers auth'", len(pf.Providers))
	}
	if nonLocal < 2 {
		return fmt.Errorf("need at least 2 non-local providers, but only have %d. Add more with 'horcrux providers auth'", nonLocal)
	}

	n := len(pf.Providers)
	m := providers.CalculateThreshold(n)
	fmt.Printf("Distributing vault across %d providers (threshold %d)...\n", n, m)
	if err := distribute.Distribute(passphrase); err != nil {
		return err
	}
	fmt.Println("Done! Vault distributed successfully.")
	return nil
}

func restoreCommand(c *cli.Context) error {
	passphrase := getPassphrase()
	fmt.Println("Restoring vault from distributed shares...")
	if err := distribute.Restore(passphrase); err != nil {
		return err
	}
	fmt.Println("Done! Vault restored successfully.")
	return nil
}

func providersAuthCommand(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing argument: <gdrive|dropbox|s3|usb|ssh|webdav|local>")
	}
	providerType := c.Args().Get(0)

	providerName := c.String("name")
	if providerName == "" {
		providerName = providerType
	}

	passphrase := getPassphrase()

	pf, err := providers.LoadConfig(passphrase)
	if err != nil {
		return fmt.Errorf("loading providers config: %w", err)
	}

	if _, exists := pf.Providers[providerName]; exists {
		return fmt.Errorf("provider '%s' already exists. Use a different --name or remove it first", providerName)
	}

	ctx := context.Background()

	switch providerType {
	case "gdrive":
		fmt.Println("Authenticating with Google Drive...")
		fmt.Println("You need a Google Cloud OAuth 2.0 Client ID for 'Desktop application'.")
		fmt.Println("Create one at: https://console.cloud.google.com/apis/credentials")
		clientID := c.String("client-id")
		if clientID == "" {
			clientID = os.Getenv("HORCRUX_GDRIVE_CLIENT_ID")
		}
		if clientID == "" {
			fmt.Print("OAuth Client ID: ")
			fmt.Scanln(&clientID)
		}
		clientSecret := c.String("client-secret")
		if clientSecret == "" {
			clientSecret = os.Getenv("HORCRUX_GDRIVE_CLIENT_SECRET")
		}
		if clientSecret == "" {
			fmt.Print("OAuth Client Secret: ")
			clientSecret = getPassphraseInput("")
		}
		token, err := storage.RunGDriveAuth(clientID, clientSecret)
		if err != nil {
			return fmt.Errorf("Google Drive auth failed: %w", err)
		}
		p := storage.NewGDriveProvider(token, clientID, clientSecret)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("Google Drive verification failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:         "gdrive",
			Token:        token,
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}

	case "dropbox":
		fmt.Println("Authenticating with Dropbox...")
		token, err := storage.RunDropboxAuth()
		if err != nil {
			return fmt.Errorf("Dropbox auth failed: %w", err)
		}
		p := storage.NewDropboxProvider(token)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("Dropbox verification failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:  "dropbox",
			Token: token,
		}

	case "local":
		if providerName != "local" {
			return fmt.Errorf("local provider cannot be renamed, it must be named 'local'")
		}
		for name, pc := range pf.Providers {
			if pc.Type == "local" && name != "local" {
				return fmt.Errorf("local provider already exists as '%s'. Remove it first", name)
			}
		}
		path := c.String("path")
		if path == "" {
			path = config.DistDir()
		}
		p := storage.NewLocalProvider(path)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("local provider setup failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type: "local",
			Path: path,
		}
		fmt.Printf("Local provider configured at %s\n", path)

	case "s3":
		endpoint := c.String("endpoint")
		if endpoint == "" {
			fmt.Print("S3 Endpoint (e.g. s3.amazonaws.com): ")
			fmt.Scanln(&endpoint)
		}
		region := c.String("region")
		if region == "" {
			fmt.Print("Region [us-east-1]: ")
			fmt.Scanln(&region)
		}
		bucket := c.String("bucket")
		if bucket == "" {
			fmt.Print("Bucket name: ")
			fmt.Scanln(&bucket)
		}
		accessKey := c.String("access-key")
		if accessKey == "" {
			fmt.Print("Access Key ID: ")
			fmt.Scanln(&accessKey)
		}
		secretKey := c.String("secret-key")
		if secretKey == "" {
			fmt.Print("Secret Access Key: ")
			secretKey = getPassphraseInput("")
		}
		if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
			return fmt.Errorf("endpoint, bucket, access key, and secret key are required")
		}
		if region == "" {
			region = "us-east-1"
		}
		p := storage.NewS3Provider(endpoint, region, bucket, accessKey, secretKey)
		fmt.Printf("Connecting to S3 at %s/%s...\n", endpoint, bucket)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("S3 connection failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:      "s3",
			Endpoint:  endpoint,
			Region:    region,
			Bucket:    bucket,
			AccessKey: accessKey,
			SecretKey: secretKey,
		}

	case "usb":
		mountPath := c.String("path")
		if mountPath == "" {
			fmt.Print("USB drive mount path (e.g. /Volumes/USBDRIVE): ")
			fmt.Scanln(&mountPath)
		}
		if mountPath == "" {
			return fmt.Errorf("mount path is required")
		}
		p := storage.NewUSBProvider(mountPath)
		fmt.Printf("Checking USB drive at %s...\n", mountPath)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("USB drive check failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type: "usb",
			Path: mountPath,
		}
		fmt.Printf("USB provider configured at %s\n", mountPath)

	case "ssh":
		host := c.String("host")
		if host == "" {
			fmt.Print("SSH Host: ")
			fmt.Scanln(&host)
		}
		port := c.String("port")
		if port == "" {
			fmt.Print("Port [22]: ")
			fmt.Scanln(&port)
		}
		username := c.String("username")
		if username == "" {
			fmt.Print("Username: ")
			fmt.Scanln(&username)
		}
		var password, keyPath string
		authChoice := c.String("auth")
		if authChoice == "" {
			fmt.Print("Auth method (password/key): ")
			fmt.Scanln(&authChoice)
		}
		switch authChoice {
		case "key":
			keyPath = c.String("key-path")
			if keyPath == "" {
				fmt.Print("Path to private key (e.g. ~/.ssh/id_rsa): ")
				fmt.Scanln(&keyPath)
			}
			keyPass := c.String("password")
			if keyPass == "" {
				fmt.Print("Key passphrase (leave empty if none): ")
				keyPass = getPassphraseInput("")
			}
			password = keyPass
		default:
			password = c.String("password")
			if password == "" {
				fmt.Print("Password: ")
				password = getPassphraseInput("")
			}
		}
		remotePath := c.String("remote-path")
		if remotePath == "" {
			fmt.Print("Remote directory [.horcrux]: ")
			fmt.Scanln(&remotePath)
		}
		if host == "" || username == "" || (password == "" && keyPath == "") {
			return fmt.Errorf("host, username, and auth credentials are required")
		}
		p := storage.NewSSHProvider(host, port, username, password, keyPath, remotePath)
		fmt.Printf("Connecting to %s@%s:%s...\n", username, host, port)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("SSH connection failed: %w", err)
		}
		p.Close()
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:       "ssh",
			Host:       host,
			Port:       port,
			Username:   username,
			Password:   password,
			KeyPath:    keyPath,
			RemotePath: remotePath,
		}

	case "webdav":
		endpoint := c.String("endpoint")
		if endpoint == "" {
			fmt.Print("WebDAV URL (e.g. https://nextcloud.example.com/remote.php/dav/files/user): ")
			fmt.Scanln(&endpoint)
		}
		username := c.String("username")
		if username == "" {
			fmt.Print("Username: ")
			fmt.Scanln(&username)
		}
		password := c.String("password")
		if password == "" {
			fmt.Print("Password or app token: ")
			password = getPassphraseInput("")
		}
		if endpoint == "" || username == "" || password == "" {
			return fmt.Errorf("endpoint, username, and password are required")
		}
		p := storage.NewWebDAVProvider(endpoint, username, password)
		fmt.Printf("Connecting to WebDAV at %s...\n", endpoint)
		if err := p.Authenticate(ctx); err != nil {
			return fmt.Errorf("WebDAV connection failed: %w", err)
		}
		pf.Providers[providerName] = providers.ProviderConfig{
			Type:     "webdav",
			Endpoint: endpoint,
			Username: username,
			Password: password,
		}

	default:
		return fmt.Errorf("unknown provider '%s'. Supported: gdrive, dropbox, s3, usb, ssh, webdav, local", providerType)
	}

	if err := providers.SaveConfig(pf, passphrase); err != nil {
		return fmt.Errorf("saving providers config: %w", err)
	}

	fmt.Printf("%s provider '%s' configured successfully.\n", providerType, providerName)
	return nil
}

func providersListCommand(c *cli.Context) error {
	passphrase := getPassphrase()

	pf, err := providers.LoadConfig(passphrase)
	if err != nil {
		return fmt.Errorf("loading providers config: %w", err)
	}

	fmt.Printf("Providers (%d configured):\n\n", len(pf.Providers))

	if len(pf.Providers) == 0 {
		fmt.Println("No providers configured.")
		fmt.Println("Run 'horcrux providers auth <gdrive|dropbox|s3|usb|ssh|webdav|local>' to add one.")
		return nil
	}

	names := make([]string, 0, len(pf.Providers))
	for name := range pf.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		pc := pf.Providers[name]
		status := "not authenticated"
		if pc.Token != nil && pc.Token.Valid() {
			status = "authenticated"
		} else if pc.Type == "local" {
			status = "ready"
		} else if pc.Type == "s3" && pc.AccessKey != "" {
			status = "ready"
		} else if pc.Type == "usb" && pc.Path != "" {
			status = "ready"
		} else if pc.Type == "ssh" && pc.Host != "" {
			status = "ready"
		} else if pc.Type == "webdav" && pc.Endpoint != "" {
			status = "ready"
		} else if pc.Token != nil && pc.Token.RefreshToken != "" {
			status = "token expired (will refresh on next use)"
		}
		fmt.Printf("  %-15s  type=%-8s  status=%s\n", name, pc.Type, status)
	}

	nonLocal := providers.CountNonLocal(pf)
	if nonLocal >= 2 {
		n := len(pf.Providers)
		m := providers.CalculateThreshold(n)
		fmt.Printf("\nDistribute: %d shares, threshold %d (need %d providers to restore)\n", n, m, m)
	} else {
		fmt.Printf("\n⚠  Need at least 2 non-local providers to distribute (have %d)\n", nonLocal)
	}
	return nil
}

func providersRemoveCommand(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing argument: <provider name>")
	}
	name := c.Args().Get(0)

	passphrase := getPassphrase()

	pf, err := providers.LoadConfig(passphrase)
	if err != nil {
		return fmt.Errorf("loading providers config: %w", err)
	}

	pc, ok := pf.Providers[name]
	if !ok {
		return fmt.Errorf("provider '%s' not found. Use 'providers list' to see configured providers", name)
	}

	delete(pf.Providers, name)

	nonLocal := providers.CountNonLocal(pf)
	if nonLocal < 2 {
		fmt.Printf("Warning: only %d non-local provider(s) remaining. Need at least 2 to distribute.\n", nonLocal)
	}

	if err := providers.SaveConfig(pf, passphrase); err != nil {
		return fmt.Errorf("saving providers config: %w", err)
	}

	fmt.Printf("Provider '%s' (%s) removed.\n", name, pc.Type)
	return nil
}
