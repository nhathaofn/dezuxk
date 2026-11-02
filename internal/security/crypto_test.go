package security_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"dezuxk/internal/security"
)

func TestAESGCMCipher_Roundtrip(t *testing.T) {
	cipher, err := security.NewAESGCMCipher("my-super-secret-master-key-12345")
	if err != nil {
		t.Fatalf("failed to create AESGCMCipher: %v", err)
	}

	testCases := []struct {
		name    string
		payload string
	}{
		{"ASCII string", "Hello, Dezuxk Gateway!"},
		{"Vietnamese Unicode", "Xin chào hệ thống Dezuxk Studio & Gateway đa nền tảng 🇻🇳"},
		{"Emoji characters", "🚀🎨⚡🔑🔐🛡️"},
		{"Complex JSON", `{"account_id":"acc-123","cookies":[{"name":"SID","value":"cookie_val"}]}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plaintext := []byte(tc.payload)
			encrypted, err := cipher.Encrypt(plaintext)
			if err != nil {
				t.Fatalf("encryption failed: %v", err)
			}

			// Verify TagAESGCM byte
			if len(encrypted) == 0 || encrypted[0] != security.TagAESGCM {
				t.Fatalf("expected encrypted blob to start with TagAESGCM (0x02), got 0x%02x", encrypted[0])
			}

			decrypted, err := cipher.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("decryption failed: %v", err)
			}

			if !bytes.Equal(plaintext, decrypted) {
				t.Fatalf("decrypted text mismatch.\nWant: %s\nGot:  %s", string(plaintext), string(decrypted))
			}
		})
	}
}

func TestAESGCMCipher_ErrorHandling(t *testing.T) {
	cipher, err := security.NewAESGCMCipher("test-master-key")
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}

	// Empty plaintext
	if _, err := cipher.Encrypt(nil); err == nil {
		t.Errorf("expected error when encrypting nil/empty plaintext")
	}

	// Empty ciphertext
	if _, err := cipher.Decrypt(nil); err == nil {
		t.Errorf("expected error when decrypting nil/empty ciphertext")
	}

	// Corrupted ciphertext
	encrypted, _ := cipher.Encrypt([]byte("valid plaintext"))
	corrupted := make([]byte, len(encrypted))
	copy(corrupted, encrypted)
	corrupted[len(corrupted)-1] ^= 0xFF // flip last byte

	if _, err := cipher.Decrypt(corrupted); err == nil {
		t.Errorf("expected error when decrypting corrupted ciphertext")
	}

	// Wrong master key
	otherCipher, _ := security.NewAESGCMCipher("wrong-master-key")
	if _, err := otherCipher.Decrypt(encrypted); err == nil {
		t.Errorf("expected error when decrypting with wrong master key")
	}
}

func TestDPAPICipher_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping DPAPI test on non-windows platform")
	}

	dpapi := security.NewDPAPICipher()
	plaintext := []byte("DPAPI sensitive test cookie data")

	encrypted, err := dpapi.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("DPAPI encrypt failed: %v", err)
	}

	if len(encrypted) == 0 || encrypted[0] != security.TagDPAPI {
		t.Fatalf("expected TagDPAPI (0x01), got 0x%02x", encrypted[0])
	}

	decrypted, err := dpapi.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("DPAPI decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("DPAPI decrypted mismatch: got %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestAdaptiveCipher_CrossDetection(t *testing.T) {
	masterKey := "adaptive-master-key-7890"

	// Adaptive cipher preferring AES-GCM
	adaptiveAES, err := security.NewAdaptiveCipher(masterKey, false)
	if err != nil {
		t.Fatalf("failed to create AdaptiveCipher: %v", err)
	}

	plaintext := []byte("secret session credentials across cipher formats")

	// Encrypt via AES
	blobAES, err := adaptiveAES.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt AES failed: %v", err)
	}

	// Decrypt via AdaptiveCipher
	decryptedAES, err := adaptiveAES.Decrypt(blobAES)
	if err != nil {
		t.Fatalf("decrypt AES failed: %v", err)
	}
	if !bytes.Equal(plaintext, decryptedAES) {
		t.Fatalf("mismatch decrypted AES")
	}

	// If on windows, test Adaptive cipher preferring DPAPI
	if runtime.GOOS == "windows" {
		adaptiveDPAPI, err := security.NewAdaptiveCipher(masterKey, true)
		if err != nil {
			t.Fatalf("failed to create AdaptiveCipher with DPAPI: %v", err)
		}

		blobDPAPI, err := adaptiveDPAPI.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("encrypt DPAPI failed: %v", err)
		}

		// Decrypt DPAPI blob using adaptiveDPAPI
		decDPAPI, err := adaptiveDPAPI.Decrypt(blobDPAPI)
		if err != nil {
			t.Fatalf("decrypt DPAPI failed: %v", err)
		}
		if !bytes.Equal(plaintext, decDPAPI) {
			t.Fatalf("mismatch decrypted DPAPI")
		}

		// Cross-decryption: adaptiveDPAPI must decrypt blobAES seamlessly!
		crossDecrypted, err := adaptiveDPAPI.Decrypt(blobAES)
		if err != nil {
			t.Fatalf("adaptiveDPAPI failed to decrypt AES blob: %v", err)
		}
		if !bytes.Equal(plaintext, crossDecrypted) {
			t.Fatalf("cross-decrypted mismatch")
		}
	}
}

func TestTokenPrimitives(t *testing.T) {
	// 1. NewToken
	tok1, err := security.NewToken()
	if err != nil {
		t.Fatalf("NewToken failed: %v", err)
	}
	tok2, err := security.NewToken()
	if err != nil {
		t.Fatalf("NewToken failed: %v", err)
	}
	if tok1 == tok2 {
		t.Fatalf("expected unique tokens, got collision: %s", tok1)
	}
	if len(tok1) != security.TokenBytes*2 {
		t.Errorf("expected token length %d, got %d", security.TokenBytes*2, len(tok1))
	}

	// 2. TokenEqual
	if !security.TokenEqual("secret-token-123", "secret-token-123") {
		t.Errorf("expected TokenEqual to return true for matching tokens")
	}
	if security.TokenEqual("secret-token-123", "secret-token-456") {
		t.Errorf("expected TokenEqual to return false for mismatching tokens")
	}
	if security.TokenEqual("", "") {
		t.Errorf("expected TokenEqual to return false for empty tokens")
	}
	if security.TokenEqual("secret-token", "") {
		t.Errorf("expected TokenEqual to return false for empty got token")
	}
	if security.TokenEqual("", "secret-token") {
		t.Errorf("expected TokenEqual to return false for empty expected token")
	}

	// 3. BearerToken extraction
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	if tok := security.BearerToken(req); tok != "" {
		t.Errorf("expected empty token for request without Authorization header, got %s", tok)
	}

	req.Header.Set("Authorization", "Bearer my-secret-api-key")
	if tok := security.BearerToken(req); tok != "my-secret-api-key" {
		t.Errorf("expected my-secret-api-key, got %s", tok)
	}

	req.Header.Set("Authorization", "bearer   spaced-api-key   ")
	if tok := security.BearerToken(req); tok != "spaced-api-key" {
		t.Errorf("expected spaced-api-key, got %s", tok)
	}

	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	if tok := security.BearerToken(req); tok != "" {
		t.Errorf("expected empty token for Basic auth header, got %s", tok)
	}
}
