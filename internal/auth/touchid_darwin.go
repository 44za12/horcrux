package auth

/*
#cgo CFLAGS: -x objective-c -fmodules -fblocks
#cgo LDFLAGS: -framework CoreFoundation -framework LocalAuthentication -framework Foundation

#include <stdlib.h>
#import <LocalAuthentication/LocalAuthentication.h>

int authenticateBiometrics(const char* reason) {
    LAContext *laContext = [[LAContext alloc] init];
    NSError *authError = nil;
    dispatch_semaphore_t sema = dispatch_semaphore_create(0);
    NSString *nsReason = [NSString stringWithUTF8String:reason];
    __block int result = 0;

    laContext.localizedFallbackTitle = @"";

    if ([laContext canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&authError]) {
        [laContext evaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics
                 localizedReason:nsReason
                           reply:^(BOOL success, NSError *error) {
             result = success ? 1 : 2;
             dispatch_semaphore_signal(sema);
         }];
    } else {
        dispatch_release(sema);
        return 0;
    }

    dispatch_semaphore_wait(sema, DISPATCH_TIME_FOREVER);
    dispatch_release(sema);
    return result;
}

int authenticateAny(const char* reason) {
    LAContext *laContext = [[LAContext alloc] init];
    NSError *authError = nil;
    dispatch_semaphore_t sema = dispatch_semaphore_create(0);
    NSString *nsReason = [NSString stringWithUTF8String:reason];
    __block int result = 0;

    if ([laContext canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:&authError]) {
        [laContext evaluatePolicy:LAPolicyDeviceOwnerAuthentication
                 localizedReason:nsReason
                           reply:^(BOOL success, NSError *error) {
             result = success ? 1 : 2;
             dispatch_semaphore_signal(sema);
         }];
    } else {
        dispatch_release(sema);
        return 0;
    }

    dispatch_semaphore_wait(sema, DISPATCH_TIME_FOREVER);
    dispatch_release(sema);
    return result;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func AuthenticateTouchID(reason string) error {
	cs := C.CString(reason)
	defer C.free(unsafe.Pointer(cs))

	result := C.authenticateBiometrics(cs)
	switch result {
	case 1:
		return nil
	case 2:
		return fmt.Errorf("authentication cancelled or failed")
	default:
		return fmt.Errorf("Touch ID not available on this device")
	}
}

func AuthenticateAny(reason string) error {
	cs := C.CString(reason)
	defer C.free(unsafe.Pointer(cs))

	result := C.authenticateAny(cs)
	switch result {
	case 1:
		return nil
	case 2:
		return fmt.Errorf("authentication cancelled or failed")
	default:
		return fmt.Errorf("device authentication not available")
	}
}
