package proxy

import (
	"fmt"
	"net/http"
	"strings"

	"dezuxk/internal/domain/providers"
)

// InjectCredentials injects decrypted cookies and session tokens into an outgoing HTTP request.
func InjectCredentials(req *http.Request, sec *providers.AccountSecrets, p *providers.Provider) {
	if req == nil || sec == nil {
		return
	}

	// 1. Inject Cookies
	if len(sec.Cookies) > 0 {
		var cookieParts []string
		for _, c := range sec.Cookies {
			if c.Name == "" {
				continue
			}
			// If provider specifies allowed domains, optionally verify domain match
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		if len(cookieParts) > 0 {
			req.Header.Set("Cookie", strings.Join(cookieParts, "; "))
		}
	}

	// 2. Inject Tokens
	if len(sec.Tokens) > 0 {
		for key, val := range sec.Tokens {
			if val == "" {
				continue
			}
			switch strings.ToLower(key) {
			case "authorization", "bearer", "bearer_token", "access_token":
				if !strings.HasPrefix(strings.ToLower(val), "bearer ") {
					req.Header.Set("Authorization", "Bearer "+val)
				} else {
					req.Header.Set("Authorization", val)
				}
			case "api_key", "x-goog-api-key":
				req.Header.Set("x-goog-api-key", val)
			default:
				req.Header.Set(key, val)
			}
		}
	}
}
