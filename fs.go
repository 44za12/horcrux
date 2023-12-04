package main

func InitBSONFiles(passphrase string) (string, error) {
    data := make(map[string]map[string]string)
	randomStringForPassPhraseRecovery, err := encryptPassphraseAndStore(passphrase)
	_ = EncryptBSONFile(passespath, data, passphrase)
	if err != nil {
        panic(err)
    }
    return randomStringForPassPhraseRecovery, EncryptBSONFile(totppasspath, data, passphrase)
}

func encryptPassphraseAndStore(passphrase string) (string, error) {
	mainpass := map[string]map[string]string{}
	mainpass["password"] = map[string]string{}
	randomStringForPassPhrase := generateRandomString(20)
	mainpass["password"][randomStringForPassPhrase] = passphrase
	return randomStringForPassPhrase, EncryptBSONFile(mainpasspath, mainpass, randomStringForPassPhrase)
}

func decryptPassPhrase(randomStringForPassPhraseRecovery string) string {
	passphraseMap, err := DecryptBSONFile(mainpasspath, randomStringForPassPhraseRecovery)
	if err != nil {
		panic(err)
	}
	return passphraseMap["password"][randomStringForPassPhraseRecovery]
}



