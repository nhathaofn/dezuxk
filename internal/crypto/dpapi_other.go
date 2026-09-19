//go:build !windows

package crypto

import "errors"

func masterKeyProtectionRequired() bool { return false }

func protectMasterKey([]byte) ([]byte, error) {
	return nil, errors.New("Windows DPAPI is unavailable on this platform")
}

func unprotectMasterKey([]byte) ([]byte, error) {
	return nil, errors.New("Windows DPAPI is unavailable on this platform")
}
