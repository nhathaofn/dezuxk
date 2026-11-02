package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/provider/manifest"
)

// TestAdversarial_Validator_URLSchemeBoundaries tests URL schemes and malformed hosts.
func TestAdversarial_Validator_URLSchemeBoundaries(t *testing.T) {
	v := manifest.NewValidator()

	invalidURLs := []struct {
		desc string
		url  string
	}{
		{"javascript scheme", "javascript:alert(1)"},
		{"ftp scheme", "ftp://ftp.example.com/api"},
		{"data scheme", "data:text/html;base64,PHNjcmlwdD4="},
		{"file scheme", "file:///etc/passwd"},
		{"ws scheme", "ws://echo.websocket.events"},
		{"wss scheme", "wss://echo.websocket.events"},
		{"missing scheme and slash", "example.com/api"},
		{"missing scheme with slash", "//example.com/api"},
		{"empty scheme http colon", "http:"},
		{"scheme only with slashes", "http://"},
		{"scheme with three slashes and path", "http:///api/v1"},
		{"scheme with port only no host", "http://:8080"},
		{"scheme with host containing spaces", "http://example .com"},
		{"scheme with newline", "http://example.com\n/api"},
		{"scheme with tab", "http://example.com\t/api"},
		{"scheme with null byte", "http://example.com\x00/api"},
		{"relative path only", "/api/v1/generate"},
		{"gopher scheme", "gopher://gopher.example.com"},
	}

	for _, tc := range invalidURLs {
		t.Run(tc.desc, func(t *testing.T) {
			p := validProvider()
			p.BaseURL = tc.url
			err := v.Validate(p)
			if err == nil {
				t.Fatalf("expected validation failure for invalid URL %q (%s), got nil", tc.url, tc.desc)
			}
			if !errors.Is(err, manifest.ErrInvalidBaseURL) {
				t.Errorf("expected ErrInvalidBaseURL for %q, got: %v", tc.url, err)
			}
		})
	}

	validURLs := []struct {
		desc string
		url  string
	}{
		{"https standard", "https://api.example.com"},
		{"http standard", "http://api.example.com"},
		{"http with port", "http://api.example.com:8080"},
		{"https with port", "https://api.example.com:8443"},
		{"https with path", "https://api.example.com/v1/images"},
		{"http localhost", "http://localhost"},
		{"http localhost with port", "http://localhost:3000"},
		{"http ipv4", "http://127.0.0.1:8080"},
		{"https ipv4", "https://192.168.1.1:9000"},
		{"https with query params", "https://api.example.com/v1?key=val"},
	}

	for _, tc := range validURLs {
		t.Run(tc.desc, func(t *testing.T) {
			p := validProvider()
			p.BaseURL = tc.url
			err := v.Validate(p)
			if err != nil {
				t.Fatalf("expected valid URL %q (%s) to pass, got: %v", tc.url, tc.desc, err)
			}
		})
	}
}

// TestAdversarial_Validator_IDBoundaries tests ID field boundary conditions.
func TestAdversarial_Validator_IDBoundaries(t *testing.T) {
	v := manifest.NewValidator()

	invalidIDs := []struct {
		desc string
		id   string
	}{
		{"empty string", ""},
		{"spaces only", "    "},
		{"newlines only", "\n\r\t"},
		{"spaces in middle", "flow provider"},
		{"unicode letters", "flow_tiếng_việt"},
		{"emoji", "provider_🚀"},
		{"dot in id", "flow.google"},
		{"colon in id", "flow:v1"},
		{"slash in id", "flow/v1"},
		{"backslash in id", "flow\\v1"},
		{"sql injection attempt", "flow'; DROP TABLE dynamic_providers;--"},
		{"path traversal attempt", "../../etc/passwd"},
		{"null byte in id", "flow\x00evil"},
		{"id 129 chars", strings.Repeat("a", 129)},
		{"id 500 chars", strings.Repeat("x", 500)},
	}

	for _, tc := range invalidIDs {
		t.Run(tc.desc, func(t *testing.T) {
			p := validProvider()
			p.ID = tc.id
			err := v.Validate(p)
			if err == nil {
				t.Fatalf("expected validation failure for ID %q (%s), got nil", tc.id, tc.desc)
			}
			if tc.id == "" || strings.TrimSpace(tc.id) == "" {
				if !errors.Is(err, manifest.ErrEmptyID) {
					t.Errorf("expected ErrEmptyID, got: %v", err)
				}
			} else {
				if !errors.Is(err, manifest.ErrInvalidID) {
					t.Errorf("expected ErrInvalidID, got: %v", err)
				}
			}
		})
	}

	validIDs := []struct {
		desc string
		id   string
	}{
		{"1 char", "a"},
		{"numeric only", "12345"},
		{"uppercase only", "FLOW"},
		{"mixed case", "GoogleFlowV1"},
		{"with dashes and underscores", "my-flow_provider-2026"},
		{"exact 128 chars", strings.Repeat("k", 128)},
	}

	for _, tc := range validIDs {
		t.Run(tc.desc, func(t *testing.T) {
			p := validProvider()
			p.ID = tc.id
			err := v.Validate(p)
			if err != nil {
				t.Fatalf("expected ID %q (%s) to pass, got: %v", tc.id, tc.desc, err)
			}
		})
	}
}

// TestAdversarial_Validator_NameBoundaries tests Name field boundary conditions.
func TestAdversarial_Validator_NameBoundaries(t *testing.T) {
	v := manifest.NewValidator()

	invalidNames := []struct {
		desc string
		name string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"tabs and newlines", "\t\r\n"},
	}

	for _, tc := range invalidNames {
		t.Run(tc.desc, func(t *testing.T) {
			p := validProvider()
			p.Name = tc.name
			err := v.Validate(p)
			if err == nil || !errors.Is(err, manifest.ErrEmptyName) {
				t.Fatalf("expected ErrEmptyName for %s, got: %v", tc.desc, err)
			}
		})
	}

	validNames := []struct {
		desc string
		name string
	}{
		{"1 char", "X"},
		{"unicode text", "Nhà cung cấp Google Flow"},
		{"1000 chars long", strings.Repeat("Long Provider Name ", 50)},
	}

	for _, tc := range validNames {
		t.Run(tc.desc, func(t *testing.T) {
			p := validProvider()
			p.Name = tc.name
			err := v.Validate(p)
			if err != nil {
				t.Fatalf("expected valid name (%s) to pass, got: %v", tc.desc, err)
			}
		})
	}
}

// TestAdversarial_Validator_StatusBoundaries tests Status field boundary conditions.
func TestAdversarial_Validator_StatusBoundaries(t *testing.T) {
	v := manifest.NewValidator()

	invalidStatuses := []string{
		"",
		" ",
		"ACTIVE",
		"Active",
		"DISABLED",
		"Disabled",
		"active ",
		" disabled",
		"enabled",
		"pending",
		"paused",
		"deleted",
		"inactive",
		"1",
		"true",
	}

	for _, st := range invalidStatuses {
		t.Run("status_"+st, func(t *testing.T) {
			p := validProvider()
			p.Status = providers.ProviderStatus(st)
			err := v.Validate(p)
			if err == nil || !errors.Is(err, manifest.ErrInvalidProviderStatus) {
				t.Fatalf("expected ErrInvalidProviderStatus for status %q, got: %v", st, err)
			}
		})
	}
}

// TestAdversarial_Validator_ProbeRulesEdgeCases tests regex errors and probe rule edge cases.
func TestAdversarial_Validator_ProbeRulesEdgeCases(t *testing.T) {
	v := manifest.NewValidator()

	malformedRegexes := []struct {
		desc    string
		pattern string
	}{
		{"unclosed group", "(unclosed"},
		{"unclosed bracket", "[a-z"},
		{"trailing backslash", "test\\"},
		{"invalid repeat", "*repeat"},
		{"invalid quantifier range", "a{5,2}"},
		{"nested quantifier", "a++"},
		{"invalid flag", "(?z:pattern)"},
	}

	for _, tc := range malformedRegexes {
		t.Run(tc.desc, func(t *testing.T) {
			p := validProvider()
			p.ProbeRules.TokenExtractors = map[string]string{
				"bad_token": tc.pattern,
			}
			err := v.Validate(p)
			if err == nil || !errors.Is(err, manifest.ErrInvalidProbeRules) {
				t.Fatalf("expected ErrInvalidProbeRules for malformed regex %q (%s), got: %v", tc.pattern, tc.desc, err)
			}
		})
	}

	t.Run("probe rules with empty identity expression element", func(t *testing.T) {
		p := validProvider()
		p.ProbeRules.IdentityExpressions = []string{"valid-selector", "  ", "another-valid"}
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrInvalidProbeRules) {
			t.Fatalf("expected ErrInvalidProbeRules, got: %v", err)
		}
	})

	t.Run("probe rules with empty discovery keyword element", func(t *testing.T) {
		p := validProvider()
		p.ProbeRules.DiscoveryKeywords = []string{"Keyword1", "\t\n"}
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrInvalidProbeRules) {
			t.Fatalf("expected ErrInvalidProbeRules, got: %v", err)
		}
	})
}

// TestAdversarial_Validator_DomainValidationEdgeCases tests domain patterns.
func TestAdversarial_Validator_DomainValidationEdgeCases(t *testing.T) {
	v := manifest.NewValidator()

	invalidDomains := []struct {
		desc   string
		domain string
	}{
		{"url scheme in domain", "https://google.com"},
		{"slash in domain", "google.com/auth"},
		{"double dots", "google..com"},
		{"leading dot only", "."},
		{"wildcard only", "*"},
		{"wildcard dot only", "*."},
		{"wildcard in middle", "sub.*.google.com"},
		{"spaces in domain", "google .com"},
		{"special symbols", "google$com"},
	}

	for _, tc := range invalidDomains {
		t.Run(tc.desc, func(t *testing.T) {
			p := validProvider()
			p.AuthDomain = tc.domain
			err := v.Validate(p)
			if err == nil || !errors.Is(err, manifest.ErrInvalidAuthDomain) {
				t.Fatalf("expected ErrInvalidAuthDomain for AuthDomain %q (%s), got: %v", tc.domain, tc.desc, err)
			}

			p2 := validProvider()
			p2.AllowedDomains = []string{tc.domain}
			err2 := v.Validate(p2)
			if err2 == nil || !errors.Is(err2, manifest.ErrInvalidAllowedDomain) {
				t.Fatalf("expected ErrInvalidAllowedDomain for AllowedDomains %q (%s), got: %v", tc.domain, tc.desc, err2)
			}
		})
	}
}
