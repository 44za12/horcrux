package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:                  "Horcrux",
		Usage:                 "A CLI password manager",
		EnableBashCompletion:  true,
		HideHelpCommand:       true,
		Suggest:               true,
		Commands: []*cli.Command{
			{
				Name:   "init",
				Usage:  "Initialise application, setting a passphrase",
				Action: initCommand,
			},
			{
				Name:  "pass",
				Usage: "Commands related to passwords",
				Subcommands: []*cli.Command{
					{
						Name:      "addpass",
						Usage:     "Add a new password",
						Action:    addPassCommand,
						ArgsUsage: "[site] [username] [password]",
						Aliases:   []string{"a", "add"},
					},
					{
						Name:      "importcsv",
						Usage:     "Import passwords from a CSV file",
						Action:    importFromCSV,
						ArgsUsage: "[CSV file path]",
						Aliases:   []string{"i"},
					},
					{
						Name:      "removepass",
						Usage:     "Remove an existing password",
						Action:    removePassCommand,
						ArgsUsage: "[site] [username]",
						Aliases:   []string{"r", "del"},
					},
					{
						Name:      "getpass",
						Usage:     "Get an existing password",
						Action:    getPassCommand,
						ArgsUsage: "[site] [username]",
						Aliases:   []string{"g", "get"},
					},
					{
						Name:      "fuzzysearch",
						Usage:     "Search passwords using fuzzy matching",
						Action:    fuzzySearchPassCommand,
						ArgsUsage: "[search query]",
						Aliases:   []string{"fz"},
					},
				},
			},
			{
				Name:  "totp",
				Usage: "Commands related to TOTP",
				Subcommands: []*cli.Command{
					{
						Name:      "addtotp",
						Usage:     "Add a new TOTP configuration",
						Action:    addTotpCommand,
						ArgsUsage: "[service] [secretKey]",
						Aliases:   []string{"a", "add"},
					},
					{
						Name:      "gettotp",
						Usage:     "Get current TOTP code",
						Action:    getTotpCommand,
						ArgsUsage: "[service]",
						Aliases:   []string{"get", "g"},
					},
					{
						Name:      "removetotp",
						Usage:     "Remove totp service",
						Action:    removeTotpCommand,
						ArgsUsage: "[service]",
						Aliases:   []string{"r", "del"},
					},
					{
						Name:      "fuzzysearch",
						Usage:     "Search passwords using fuzzy matching",
						Action:    fuzzySearchTOTPCommand,
						ArgsUsage: "[search query]",
						Aliases:   []string{"fz"},
					},
					{
						Name:      "importtotp",
						Usage:     "Import TOTP configurations from a JSON file",
						Action:    importTotpCommand,
						ArgsUsage: "[JSON file path]",
						Aliases:   []string{"i"},
					},
				},
			},
			{
				Name:   "recoverpass",
				Usage:  "Recover Forgotten Password",
				Action: recoverPass,
			},
			{
				Name:   "change-passphrase",
				Usage:  "Change the vault master passphrase",
				Action: changePassphraseCommand,
			},
			{
				Name:      "completion",
				Usage:     "Generate shell completion script",
				ArgsUsage: "<bash|zsh|fish>",
				Action:    completionCommand,
			},
			{
				Name:   "distribute",
				Usage:  "Distribute encrypted vault across configured providers",
				Action: distributeCommand,
			},
			{
				Name:   "restore",
				Usage:  "Restore vault from distributed shares",
				Action: restoreCommand,
			},
			{
				Name:  "providers",
				Usage: "Manage storage providers",
				Subcommands: []*cli.Command{
					{
						Name:      "auth",
						Usage:     "Authenticate a storage provider",
						Action:    providersAuthCommand,
						ArgsUsage: "<gdrive|dropbox|s3|usb|ssh|webdav|local>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "name",
								Usage: "Instance name (default: provider type). Required for multiple instances of same type",
							},
							&cli.StringFlag{
								Name:  "path",
								Usage: "Local/USB storage path",
							},
							&cli.StringFlag{
								Name:  "endpoint",
								Usage: "S3 endpoint (e.g. s3.amazonaws.com, s3.us-west-002.backblazeb2.com)",
							},
							&cli.StringFlag{
								Name:  "region",
								Usage: "S3 region (default: us-east-1)",
							},
							&cli.StringFlag{
								Name:  "bucket",
								Usage: "S3 bucket name",
							},
							&cli.StringFlag{
								Name:  "access-key",
								Usage: "S3 access key ID",
							},
							&cli.StringFlag{
								Name:  "secret-key",
								Usage: "S3 secret access key",
							},
							&cli.StringFlag{
								Name:  "host",
								Usage: "SSH host",
							},
							&cli.StringFlag{
								Name:  "port",
								Usage: "SSH port (default: 22)",
							},
							&cli.StringFlag{
								Name:  "username",
								Usage: "SSH username",
							},
							&cli.StringFlag{
								Name:  "password",
								Usage: "SSH password or key passphrase",
							},
							&cli.StringFlag{
								Name:  "auth",
								Usage: "SSH auth method: password or key",
							},
							&cli.StringFlag{
								Name:  "key-path",
								Usage: "Path to SSH private key",
							},
							&cli.StringFlag{
								Name:  "remote-path",
								Usage: "Remote directory path (default: .horcrux)",
							},
							&cli.StringFlag{
								Name:    "client-id",
								Usage:   "OAuth client ID (Google Drive)",
								EnvVars: []string{"HORCRUX_GDRIVE_CLIENT_ID"},
							},
							&cli.StringFlag{
								Name:    "client-secret",
								Usage:   "OAuth client secret (Google Drive)",
								EnvVars: []string{"HORCRUX_GDRIVE_CLIENT_SECRET"},
							},
						},
					},
					{
						Name:   "list",
						Usage:  "List configured providers",
						Action: providersListCommand,
					},
					{
						Name:      "remove",
						Usage:     "Remove a configured provider",
						Action:    providersRemoveCommand,
						ArgsUsage: "<provider name>",
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
