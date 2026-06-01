package auth

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework CoreFoundation -framework Foundation -framework Security -framework LocalAuthentication

#include <stdlib.h>
#include <string.h>
#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#import <Security/Security.h>

static char* cfStringCopyUTF8(CFStringRef value) {
	if (value == NULL) {
		return NULL;
	}
	CFIndex length = CFStringGetLength(value);
	CFIndex maxSize = CFStringGetMaximumSizeForEncoding(length, kCFStringEncodingUTF8) + 1;
	char *buffer = (char*)malloc(maxSize);
	if (buffer == NULL) {
		return NULL;
	}
	if (!CFStringGetCString(value, buffer, maxSize, kCFStringEncodingUTF8)) {
		free(buffer);
		return NULL;
	}
	return buffer;
}

// Checks whether biometric authentication (Touch ID / Face ID) is available on this device.
// Returns 1 if available, 0 otherwise.
int biometricsAvailable(void) {
	LAContext *ctx = [[LAContext alloc] init];
	NSError *err = nil;
	BOOL can = [ctx canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&err];
	[ctx release];
	return can ? 1 : 0;
}

static void setKeychainError(char **err, OSStatus status) {
	if (err == NULL) {
		return;
	}
	CFStringRef message = SecCopyErrorMessageString(status, NULL);
	if (message == NULL) {
		*err = strdup("keychain operation failed");
		return;
	}
	*err = cfStringCopyUTF8(message);
	CFRelease(message);
}

// Stores passphrase in keychain without biometric constraint.
// Touch ID authentication is handled separately before reading.
// The entry is protected by kSecAttrAccessibleWhenUnlockedThisDeviceOnly
// (encrypted at rest when device is locked).
int storePassphraseInKeychain(const char *service, const char *account, const char *passphrase, char **err) {
	@autoreleasepool {
		NSString *svc = [NSString stringWithUTF8String:service];
		NSString *acct = [NSString stringWithUTF8String:account];
		NSData *secret = [[NSString stringWithUTF8String:passphrase] dataUsingEncoding:NSUTF8StringEncoding];

		NSDictionary *deleteQuery = @{
			(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
			(__bridge id)kSecAttrService: svc,
			(__bridge id)kSecAttrAccount: acct,
		};
		SecItemDelete((__bridge CFDictionaryRef)deleteQuery);

		NSDictionary *addQuery = @{
			(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
			(__bridge id)kSecAttrService: svc,
			(__bridge id)kSecAttrAccount: acct,
			(__bridge id)kSecValueData: secret,
			(__bridge id)kSecAttrAccessible: (__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
		};

		OSStatus status = SecItemAdd((__bridge CFDictionaryRef)addQuery, NULL);
		if (status != errSecSuccess) {
			setKeychainError(err, status);
			return 0;
		}
		return 1;
	}
}

// Reads passphrase from keychain WITHOUT triggering biometric prompt.
// Caller must have already authenticated via Touch ID.
char* getPassphraseFromKeychain(const char *service, const char *account, char **err) {
	@autoreleasepool {
		NSString *svc = [NSString stringWithUTF8String:service];
		NSString *acct = [NSString stringWithUTF8String:account];
		NSDictionary *query = @{
			(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
			(__bridge id)kSecAttrService: svc,
			(__bridge id)kSecAttrAccount: acct,
			(__bridge id)kSecReturnData: @YES,
			(__bridge id)kSecMatchLimit: (__bridge id)kSecMatchLimitOne,
		};

		CFTypeRef result = NULL;
		OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
		if (status != errSecSuccess) {
			setKeychainError(err, status);
			return NULL;
		}

		NSData *data = (__bridge NSData*)result;
		NSString *secret = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
		char *copiedSecret = strdup([secret UTF8String]);
		[secret release];
		CFRelease(result);
		return copiedSecret;
	}
}

int hasPassphraseInKeychain(const char *service, const char *account) {
	@autoreleasepool {
		NSString *svc = [NSString stringWithUTF8String:service];
		NSString *acct = [NSString stringWithUTF8String:account];
		NSDictionary *query = @{
			(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
			(__bridge id)kSecAttrService: svc,
			(__bridge id)kSecAttrAccount: acct,
			(__bridge id)kSecReturnAttributes: @YES,
			(__bridge id)kSecMatchLimit: (__bridge id)kSecMatchLimitOne,
		};
		CFTypeRef result = NULL;
		OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
		if (result != NULL) {
			CFRelease(result);
		}
		return status == errSecSuccess ? 1 : 0;
	}
}

void deletePassphraseFromKeychain(const char *service, const char *account) {
	@autoreleasepool {
		NSString *svc = [NSString stringWithUTF8String:service];
		NSString *acct = [NSString stringWithUTF8String:account];
		NSDictionary *query = @{
			(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
			(__bridge id)kSecAttrService: svc,
			(__bridge id)kSecAttrAccount: acct,
		};
		SecItemDelete((__bridge CFDictionaryRef)query);
	}
}
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"horcrux/internal/config"
)

const keyFile = "unlock.hrcrx"
const keychainService = "com.horcrux.vault"
const keychainAccount = "vault-passphrase"

func keyPath() string {
	return config.DataDir() + "/" + keyFile
}

func StorePassphraseLocal(passphrase string) error {
	service := C.CString(keychainService)
	account := C.CString(keychainAccount)
	secret := C.CString(passphrase)
	secretLen := C.size_t(len(passphrase))
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	defer func() {
		C.memset(unsafe.Pointer(secret), 0, secretLen)
		C.free(unsafe.Pointer(secret))
	}()

	var errMsg *C.char
	ok := C.storePassphraseInKeychain(service, account, secret, &errMsg)
	if errMsg != nil {
		defer C.free(unsafe.Pointer(errMsg))
	}
	if ok != 1 {
		if errMsg == nil {
			return fmt.Errorf("storing passphrase in keychain failed")
		}
		return fmt.Errorf("storing passphrase in keychain: %s", C.GoString(errMsg))
	}
	return nil
}

// GetPassphraseLocal reads the passphrase from keychain without triggering Touch ID.
// Caller must have already authenticated via AuthenticateTouchID.
func GetPassphraseLocal() (string, error) {
	service := C.CString(keychainService)
	account := C.CString(keychainAccount)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))

	var errMsg *C.char
	secret := C.getPassphraseFromKeychain(service, account, &errMsg)
	if errMsg != nil {
		defer C.free(unsafe.Pointer(errMsg))
	}
	if secret == nil {
		if errMsg == nil {
			return "", fmt.Errorf("reading passphrase from keychain failed")
		}
		return "", fmt.Errorf("reading passphrase from keychain: %s", C.GoString(errMsg))
	}
	result := C.GoString(secret)
	C.memset(unsafe.Pointer(secret), 0, C.strlen(secret))
	C.free(unsafe.Pointer(secret))
	return result, nil
}

func HasLocalKey() bool {
	service := C.CString(keychainService)
	account := C.CString(keychainAccount)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	return C.hasPassphraseInKeychain(service, account) == 1
}

func BiometricsAvailable() bool {
	return C.biometricsAvailable() == 1
}

func DeleteLocalKey() {
	service := C.CString(keychainService)
	account := C.CString(keychainAccount)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	C.deletePassphraseFromKeychain(service, account)
	os.Remove(keyPath())
}
