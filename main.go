package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)



func main() {
    app := &cli.App{
        Name:  "Horcrux",
        Usage: "A CLI password manager",
        Commands: []*cli.Command{
            {
                Name:  "init",
                Usage: "Initialise application, setting a passphrase",
                Action: initCommand,
            },
			{
                Name:  "pass",
                Usage: "Commands related to passwords",
                Subcommands: []*cli.Command{
                    {
                        Name:    "addpass",
                        Usage:   "Add a new password",
                        Action:  addPassCommand,
                        ArgsUsage: "[site] [username] [password]",
						Aliases: []string{"a", "add"},
                    },
					{
						Name:  "importcsv",
						Usage: "Import passwords from a CSV file",
						Action:  importFromCSV,
						ArgsUsage: "[CSV file path]",
						Aliases: []string{"i"},
					},
                    {
                        Name:    "removepass",
                        Usage:   "Remove an existing password",
                        Action:  removePassCommand,
                        ArgsUsage: "[site] [username]",
						Aliases: []string{"r", "del"},
                    },
                    {
                        Name:    "getpass",
                        Usage:   "Get an existing password",
                        Action:  getPassCommand,
                        ArgsUsage: "[site] [username]",
						Aliases: []string{"g", "get"},
                    },
					{
						Name:  "fuzzysearch",
						Usage: "Search passwords using fuzzy matching",
						Action:  fuzzySearchPassCommand,
						ArgsUsage: "[search query]",
						Aliases: []string{"fz"},
					},
                    
                },
            },
            {
                Name:  "totp",
                Usage: "Commands related to TOTP",
                Subcommands: []*cli.Command{
					{
						Name:    "addtotp",
						Usage:   "Add a new TOTP configuration",
						Action:  addTotpCommand,
						ArgsUsage: "[service] [secretKey]",
						Aliases: []string{"a", "add"},
					},
					{
						Name:    "gettotp",
						Usage:   "Get current TOTP code",
						Action:  getTotpCommand,
						ArgsUsage: "[service]",
						Aliases: []string{"get", "g"},
					},
					{
						Name:    "removetotp",
						Usage:   "Remove totp service",
						Action:  removeTotpCommand,
						ArgsUsage: "[service]",
						Aliases: []string{"r", "del"},
					},
					{
						Name:  "fuzzysearch",
						Usage: "Search passwords using fuzzy matching",
						Action:  fuzzySearchTOTPCommand,
						ArgsUsage: "[search query]",
						Aliases: []string{"fz"},
					},
					{
						Name:  "importtotp",
						Usage: "Import TOTP configurations from a JSON file",
						Action:  importTotpCommand,
						ArgsUsage: "[JSON file path]",
						Aliases: []string{"i"},
					},
                },
            },
			{
				Name:    "recoverpass",
				Usage:   "Recover Forgotten Password",
				Action:  recoverPass,
			},
        },
    }

    err := app.Run(os.Args)
    if err != nil {
        log.Fatal(err)
    }
	
}
