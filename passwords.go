package main

func addUpdatePassword(site string, username string, password string, passphrase string) {
	decryptedData, err := DecryptBSONFile(passespath, passphrase)
    if err != nil {
        panic(err)
    }
	_, ok := decryptedData[site]
	if ok {
		decryptedData[site][username] = password
	} else {
		decryptedData[site] = map[string]string{}
		decryptedData[site][username] = password
	}
	err = EncryptBSONFile(passespath, decryptedData, passphrase)
	if err != nil {
        panic(err)
    }
}

func removePassword(site string, username string, passphrase string) {
	decryptedData, err := DecryptBSONFile(passespath, passphrase)
    if err != nil {
        panic(err)
    }
	_, ok := decryptedData[site]
	if ok {
		delete(decryptedData[site], username)
	}
	err = EncryptBSONFile(passespath, decryptedData, passphrase)
	if err != nil {
        panic(err)
    }
}

func getPassword(site string, username string, passphrase string) string {
	decryptedData, err := DecryptBSONFile(passespath, passphrase)
    if err != nil {
        panic(err)
    }
	_, ok := decryptedData[site]
	if !ok {
		panic("The site doesn't exist in your passwords, add this password to get it.")
	}
	pass, ok := decryptedData[site][username]
	if !ok {
		panic("There are no passwords for the specified username, check the username again.")
	}
	return pass
}