//go:build windows

package security

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modcrypt32             = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData   = modcrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = modcrypt32.NewProc("CryptUnprotectData")
)

// DPAPICipher implements Cipher using Windows Data Protection API.
type DPAPICipher struct{}

// NewDPAPICipher creates a new DPAPICipher.
func NewDPAPICipher() *DPAPICipher {
	return &DPAPICipher{}
}

// Encrypt encrypts plaintext with Windows DPAPI and prepends TagDPAPI.
func (c *DPAPICipher) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("dpapi: empty payload")
	}
	raw, err := EncryptDPAPI(plaintext)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1+len(raw))
	out[0] = TagDPAPI
	copy(out[1:], raw)
	return out, nil
}

// Decrypt decrypts a DPAPI ciphertext (with or without TagDPAPI prefix).
func (c *DPAPICipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("dpapi: empty ciphertext")
	}
	raw := ciphertext
	if raw[0] == TagDPAPI {
		raw = raw[1:]
	}
	return DecryptDPAPI(raw)
}

// EncryptDPAPI encrypts raw bytes using Windows DPAPI CryptProtectData.
func EncryptDPAPI(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("dpapi: empty payload")
	}
	var inBlob = windows.DataBlob{
		Size: uint32(len(data)),
		Data: &data[0],
	}
	var outBlob windows.DataBlob
	r1, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0, 0, 0, 0,
		uintptr(windows.CRYPTPROTECT_UI_FORBIDDEN),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("dpapi protect error: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))

	cipher := make([]byte, outBlob.Size)
	copy(cipher, unsafe.Slice(outBlob.Data, outBlob.Size))
	return cipher, nil
}

// DecryptDPAPI decrypts raw DPAPI ciphertext using CryptUnprotectData.
func DecryptDPAPI(cipher []byte) ([]byte, error) {
	if len(cipher) == 0 {
		return nil, errors.New("dpapi: empty ciphertext")
	}
	var inBlob = windows.DataBlob{
		Size: uint32(len(cipher)),
		Data: &cipher[0],
	}
	var outBlob windows.DataBlob
	r1, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0, 0, 0, 0,
		uintptr(windows.CRYPTPROTECT_UI_FORBIDDEN),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("dpapi unprotect error: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))

	plain := make([]byte, outBlob.Size)
	copy(plain, unsafe.Slice(outBlob.Data, outBlob.Size))
	return plain, nil
}
