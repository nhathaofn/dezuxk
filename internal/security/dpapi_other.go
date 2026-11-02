//go:build !windows

package security

// DPAPICipher implements Cipher stub for non-windows platforms.
type DPAPICipher struct{}

func NewDPAPICipher() *DPAPICipher {
	return &DPAPICipher{}
}

func (c *DPAPICipher) Encrypt(plaintext []byte) ([]byte, error) {
	return nil, ErrDPAPIUnsupported
}

func (c *DPAPICipher) Decrypt(ciphertext []byte) ([]byte, error) {
	return nil, ErrDPAPIUnsupported
}

func EncryptDPAPI(data []byte) ([]byte, error) {
	return nil, ErrDPAPIUnsupported
}

func DecryptDPAPI(cipher []byte) ([]byte, error) {
	return nil, ErrDPAPIUnsupported
}
