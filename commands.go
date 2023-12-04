package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/urfave/cli/v2"
)

func initCommand(c *cli.Context) error {
    passphrase := getPassphraseInput("Enter your passphrase for initialization: ")
    phraseForPassphraseRecovery, err := InitBSONFiles(passphrase)
    if err != nil {
        return err
    }
    fmt.Printf("Initialization successful. Recovery phrase: %s\n", phraseForPassphraseRecovery)
    return nil
}

func addPassCommand(c *cli.Context) error {
    if c.NArg() < 3 {
        return fmt.Errorf("missing arguments: [site] [username] [password]")
    }
    site := c.Args().Get(0)
    username := c.Args().Get(1)
    password := c.Args().Get(2)

    passphrase := getPassphraseInput("Enter your passphrase: ")
    addUpdatePassword(site, username, password, passphrase)

    fmt.Println("Password added successfully")
    return nil
}

func removePassCommand(c *cli.Context) error {
    if c.NArg() < 2 {
        return fmt.Errorf("missing arguments: [site] [username]")
    }
    site := c.Args().Get(0)
    username := c.Args().Get(1)

    passphrase := getPassphraseInput("Enter your passphrase: ")
    removePassword(site, username, passphrase)

    fmt.Println("Password removed successfully")
    return nil
}

func getPassCommand(c *cli.Context) error {
    if c.NArg() < 2 {
        return fmt.Errorf("missing arguments: [site] [username]")
    }
    site := c.Args().Get(0)
    username := c.Args().Get(1)

    passphrase := getPassphraseInput("Enter your passphrase: ")
    password := getPassword(site, username, passphrase)

    fmt.Printf("Password for %s:%s is '%s'\n", site, username, password)
    return nil
}

func recoverPass(c *cli.Context) error {
    if c.NArg() > 0 {
        return fmt.Errorf("not expecting arguments")
    }

    passphrase := getPassphraseInput("Enter your passphrase: ")
    password := decryptPassPhrase(passphrase)

    fmt.Printf("Your password is '%s'\n", password)
    return nil
}

func getTotpCommand(c *cli.Context) error {
    if c.NArg() < 1 {
        return fmt.Errorf("missing argument: [service]")
    }
    service := c.Args().Get(0)

    passphrase := getPassphraseInput("Enter your passphrase: ")
    totp := getTOTP(service, passphrase)
    fmt.Printf("Current TOTP for %s is: %06d\n", service, totp)
    return nil
}

func addTotpCommand(c *cli.Context) error {
    if c.NArg() < 2 {
        return fmt.Errorf("missing arguments: [service] [secretKey]")
    }
    service := c.Args().Get(0)
    secretKey := c.Args().Get(1)

    passphrase := getPassphraseInput("Enter your passphrase: ")
    addUpdateTOTP(service, secretKey, passphrase)
    fmt.Println("TOTP configuration added successfully")
    return nil
}

func removeTotpCommand(c *cli.Context) error {
    if c.NArg() < 1 {
        return fmt.Errorf("missing arguments: [service]")
    }
    service := c.Args().Get(0)

    passphrase := getPassphraseInput("Enter your passphrase: ")
    removeTOTP(service, passphrase)
    fmt.Println("TOTP configuration removed successfully")
    return nil
}

func importFromCSV(c *cli.Context) error {
    if c.NArg() < 1 {
        return fmt.Errorf("missing argument: [CSV file path]")
    }
    filePath := c.Args().Get(0)
    passphrase := getPassphraseInput("Enter your passphrase: ")
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

        site := record[0]
        username := record[2]
        password := record[3]
        addUpdatePassword(site, username, password, passphrase)
    }
    fmt.Println("Passwords imported successfully")
    return nil
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
    err = json.Unmarshal(jsonData, &totpImport)
    if err != nil {
        return fmt.Errorf("error unmarshaling JSON: %s", err)
    }

    passphrase := getPassphraseInput("Enter your passphrase: ")

    for _, service := range totpImport.Services {
        serviceName := service.Name
        if service.OTP.Account != "" {
            serviceName += " (" + service.OTP.Account + ")"
        }
        addUpdateTOTP(serviceName, service.Secret, passphrase)
    }

    fmt.Println("TOTP configurations imported successfully")
    return nil
}

func fuzzySearchPassCommand(c *cli.Context) error {
    if c.NArg() < 1 {
        return fmt.Errorf("missing argument: [search query]")
    }
    query := c.Args().Get(0)
    passphrase := getPassphraseInput("Enter your passphrase: ")
    storedData, err := DecryptBSONFile(passespath, passphrase)
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
    passphrase := getPassphraseInput("Enter your passphrase: ")
    storedData, err := DecryptBSONFile(totppasspath, passphrase)
    if err != nil {
        return err
    }
    for service := range storedData["totp"] {
        if fuzzy.MatchFold(query, service) {
            fmt.Printf("Site: %s, OTP: %d\n", service, generateTOTP(storedData["totp"][service]))
        }
    }
    return nil
}