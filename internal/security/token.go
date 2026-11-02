// Package security provides authentication primitives and token verification.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

// TokenBytes is the entropy of generated tokens (256 bit).
const TokenBytes = 32

// NewToken returns a cryptographically secure random hex token.
func NewToken() (string, error) {
	b := make([]byte, TokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("security: generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// TokenEqual compares two tokens in constant time to prevent timing attacks.
// An empty expected or got token never matches.
func TokenEqual(expected, got string) bool {
	if expected == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}

// BearerToken extracts the token from the "Authorization: Bearer <token>" HTTP header.
func BearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
