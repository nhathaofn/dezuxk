// Package security provides encryption abstractions and authentication primitives.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"runtime"
)

// Tag bytes identifying the encryption scheme used for serialized blobs.
const (
	TagDPAPI  byte = 0x01
	TagAESGCM byte = 0x02
)

// ErrDPAPIUnsupported indicates that DPAPI is only supported on Windows.
var ErrDPAPIUnsupported = errors.New("dpapi: unsupported platform (Windows only)")

// Cipher defines transparent symmetric encryption and decryption.
type Cipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// AESGCMCipher implements Cipher using AES-256-GCM with SHA-256 key derivation.
type AESGCMCipher struct {
	key [32]byte
}

// NewAESGCMCipher creates an AESGCMCipher with a master key derived via SHA-256.
func NewAESGCMCipher(masterKey string) (*AESGCMCipher, error) {
	if masterKey == "" {
		return nil, errors.New("aes_gcm: empty master key")
	}
	derived := sha256.Sum256([]byte(masterKey))
	return &AESGCMCipher{key: derived}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM.
// The output blob is formatted as: [TagAESGCM (1 byte)][Nonce (12 bytes)][Ciphertext + AuthTag].
func (c *AESGCMCipher) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("aes_gcm: empty payload")
	}

	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return nil, fmt.Errorf("aes_gcm: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes_gcm: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("aes_gcm: generate nonce: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, plaintext, nil)

	// Result: Tag (1) + Nonce (12) + Sealed
	out := make([]byte, 1+len(nonce)+len(sealed))
	out[0] = TagAESGCM
	copy(out[1:], nonce)
	copy(out[1+len(nonce):], sealed)

	return out, nil
}

// Decrypt decrypts an AES-256-GCM blob.
// Accepts blobs with or without the TagAESGCM prefix.
func (c *AESGCMCipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("aes_gcm: empty ciphertext")
	}

	data := ciphertext
	if data[0] == TagAESGCM {
		data = data[1:]
	}

	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return nil, fmt.Errorf("aes_gcm: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes_gcm: new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize+gcm.Overhead() {
		return nil, errors.New("aes_gcm: ciphertext too short")
	}

	nonce := data[:nonceSize]
	actualCiphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("aes_gcm: decrypt failed: %w", err)
	}

	return plaintext, nil
}

// AdaptiveCipher provides transparent encryption and adaptive decryption across
// Windows DPAPI and AES-256-GCM.
type AdaptiveCipher struct {
	aesGCM      *AESGCMCipher
	dpapi       *DPAPICipher
	preferDPAPI bool
}

// NewAdaptiveCipher creates an AdaptiveCipher with the specified master key and DPAPI preference.
func NewAdaptiveCipher(masterKey string, preferDPAPI bool) (*AdaptiveCipher, error) {
	aesCipher, err := NewAESGCMCipher(masterKey)
	if err != nil {
		return nil, err
	}
	dpapiCipher := NewDPAPICipher()

	// If runtime is not windows, force preferDPAPI to false
	if runtime.GOOS != "windows" {
		preferDPAPI = false
	}

	return &AdaptiveCipher{
		aesGCM:      aesCipher,
		dpapi:       dpapiCipher,
		preferDPAPI: preferDPAPI,
	}, nil
}

// Encrypt encrypts plaintext using the preferred cipher (DPAPI on Windows if preferred, otherwise AES-GCM).
func (a *AdaptiveCipher) Encrypt(plaintext []byte) ([]byte, error) {
	if a.preferDPAPI {
		blob, err := a.dpapi.Encrypt(plaintext)
		if err == nil {
			return blob, nil
		}
		// If DPAPI failed because it is unsupported or user context failed, fall back to AES-GCM
		if !errors.Is(err, ErrDPAPIUnsupported) {
			// If not just unsupported, still try AES-GCM
		}
	}
	return a.aesGCM.Encrypt(plaintext)
}

// Decrypt adaptively detects the encryption scheme and decrypts the ciphertext.
func (a *AdaptiveCipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("adaptive_cipher: empty ciphertext")
	}

	// 1. Check explicit tag byte
	switch ciphertext[0] {
	case TagAESGCM:
		return a.aesGCM.Decrypt(ciphertext)
	case TagDPAPI:
		if runtime.GOOS == "windows" {
			return a.dpapi.Decrypt(ciphertext)
		}
		// If on non-windows and encountered DPAPI tag, DPAPI is unsupported
		return nil, ErrDPAPIUnsupported
	}

	// 2. Untagged fallback: try DPAPI on Windows first, then AES-GCM
	if runtime.GOOS == "windows" {
		if plain, err := a.dpapi.Decrypt(ciphertext); err == nil {
			return plain, nil
		}
	}

	if plain, err := a.aesGCM.Decrypt(ciphertext); err == nil {
		return plain, nil
	}

	return nil, errors.New("adaptive_cipher: failed to decrypt with available ciphers")
}
