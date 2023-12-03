package main

func InitBSONFiles(passphrase string) (string, error) {
    data := make(map[string]map[string]string)
    filePath := "passes.bson"
	randomStringForPassPhraseRecovery, err := encryptPassphraseAndStore(passphrase)
	_ = EncryptBSONFile(filePath, data, passphrase)
	if err != nil {
        panic(err)
    }
	filePath = "totp.bson"
    return randomStringForPassPhraseRecovery, EncryptBSONFile(filePath, data, passphrase)
}

func encryptPassphraseAndStore(passphrase string) (string, error) {
	mainpass := map[string]map[string]string{}
	mainpass["password"] = map[string]string{}
	randomStringForPassPhrase := generateRandomString(20)
	mainpass["password"][randomStringForPassPhrase] = passphrase
	return randomStringForPassPhrase, EncryptBSONFile("mainpass.bson", mainpass, randomStringForPassPhrase)
}

func decryptPassPhrase(randomStringForPassPhraseRecovery string) string {
	passphraseMap, err := DecryptBSONFile("mainpass.bson", randomStringForPassPhraseRecovery)
	if err != nil {
		panic(err)
	}
	return passphraseMap["password"][randomStringForPassPhraseRecovery]
}



