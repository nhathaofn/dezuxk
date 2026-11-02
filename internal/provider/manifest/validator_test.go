package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/provider/manifest"
)

func validProvider() *providers.Provider {
	return &providers.Provider{
		ID:             "flow",
		Name:           "Google Flow",
		BaseURL:        "https://flow.google.com",
		AuthDomain:     ".google.com",
		AllowedDomains: []string{".google.com", "flow.google.com"},
		ProbeRules: providers.ProbeRules{
			IdentityExpressions: []string{"[data-email]", "meta[name=\"user-email\"]"},
			DiscoveryKeywords:   []string{"BatchGenerateImages", "FlowService"},
			TokenExtractors: map[string]string{
				"xsrf": `name="xsrf-token" value="([^"]+)"`,
			},
		},
		SecurityRules: map[string]any{"anti_bot": true},
		Status:        providers.ProviderStatusActive,
	}
}

func TestValidator_Valid(t *testing.T) {
	v := manifest.NewValidator()
	p := validProvider()

	if err := v.Validate(p); err != nil {
		t.Fatalf("expected valid provider to pass validation, got: %v", err)
	}
}

func TestValidator_NilProvider(t *testing.T) {
	v := manifest.NewValidator()
	err := v.Validate(nil)
	if err == nil {
		t.Fatal("expected error for nil provider, got nil")
	}
	if !errors.Is(err, manifest.ErrNilProvider) {
		t.Errorf("expected ErrNilProvider, got: %v", err)
	}
}

func TestValidator_IDRules(t *testing.T) {
	v := manifest.NewValidator()

	testCases := []struct {
		name      string
		id        string
		expectErr error
	}{
		{"empty", "", manifest.ErrEmptyID},
		{"whitespace", "   ", manifest.ErrEmptyID},
		{"valid alphanumeric", "flow123", nil},
		{"valid hyphens underscores", "flow-v1_dev", nil},
		{"invalid spaces", "flow provider", manifest.ErrInvalidID},
		{"invalid symbols at", "flow@google", manifest.ErrInvalidID},
		{"invalid symbols slash", "flow/google", manifest.ErrInvalidID},
		{"invalid symbols dot", "flow.google", manifest.ErrInvalidID},
		{"too long", strings.Repeat("a", 129), manifest.ErrInvalidID},
		{"exact max length", strings.Repeat("a", 128), nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProvider()
			p.ID = tc.id
			err := v.Validate(p)
			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("expected error containing %v, got nil", tc.expectErr)
				}
				if !errors.Is(err, tc.expectErr) {
					t.Errorf("expected error %v, got: %v", tc.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected valid ID, got error: %v", err)
				}
			}
		})
	}
}

func TestValidator_NameRules(t *testing.T) {
	v := manifest.NewValidator()

	for _, name := range []string{"", "   ", "\t\n"} {
		p := validProvider()
		p.Name = name
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrEmptyName) {
			t.Errorf("expected ErrEmptyName for %q, got: %v", name, err)
		}
	}
}

func TestValidator_BaseURLRules(t *testing.T) {
	v := manifest.NewValidator()

	testCases := []struct {
		name      string
		url       string
		expectErr error
	}{
		{"empty", "", manifest.ErrEmptyBaseURL},
		{"whitespace", "   ", manifest.ErrEmptyBaseURL},
		{"valid https", "https://api.example.com", nil},
		{"valid http", "http://localhost:8080", nil},
		{"invalid ftp", "ftp://ftp.example.com", manifest.ErrInvalidBaseURL},
		{"invalid file", "file:///etc/hosts", manifest.ErrInvalidBaseURL},
		{"missing scheme", "api.example.com", manifest.ErrInvalidBaseURL},
		{"missing host", "https://", manifest.ErrInvalidBaseURL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProvider()
			p.BaseURL = tc.url
			err := v.Validate(p)
			if tc.expectErr != nil {
				if err == nil || !errors.Is(err, tc.expectErr) {
					t.Errorf("expected error %v for %q, got: %v", tc.expectErr, tc.url, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected valid BaseURL for %q, got: %v", tc.url, err)
				}
			}
		})
	}
}

func TestValidator_AuthDomainRules(t *testing.T) {
	v := manifest.NewValidator()

	testCases := []struct {
		name      string
		domain    string
		expectErr error
	}{
		{"empty", "", manifest.ErrEmptyAuthDomain},
		{"whitespace", "   ", manifest.ErrEmptyAuthDomain},
		{"valid standard", ".google.com", nil},
		{"valid no leading dot", "google.com", nil},
		{"valid localhost", "localhost", nil},
		{"valid localhost port", "localhost:8080", nil},
		{"valid ip", "127.0.0.1", nil},
		{"invalid with path", "google.com/auth", manifest.ErrInvalidAuthDomain},
		{"invalid with spaces", "google .com", manifest.ErrInvalidAuthDomain},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProvider()
			p.AuthDomain = tc.domain
			err := v.Validate(p)
			if tc.expectErr != nil {
				if err == nil || !errors.Is(err, tc.expectErr) {
					t.Errorf("expected error %v for %q, got: %v", tc.expectErr, tc.domain, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected valid domain for %q, got: %v", tc.domain, err)
				}
			}
		})
	}
}

func TestValidator_AllowedDomainsRules(t *testing.T) {
	v := manifest.NewValidator()

	testCases := []struct {
		name      string
		domains   []string
		expectErr error
	}{
		{"nil slice", nil, manifest.ErrEmptyAllowedDomains},
		{"empty slice", []string{}, manifest.ErrEmptyAllowedDomains},
		{"contains blank string", []string{".google.com", "   "}, manifest.ErrInvalidAllowedDomain},
		{"contains invalid domain", []string{".google.com", "bad domain#"}, manifest.ErrInvalidAllowedDomain},
		{"valid domains with wildcard", []string{"*.google.com", "flow.google.com"}, nil},
		{"valid localhost and ip", []string{"localhost", "127.0.0.1"}, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProvider()
			p.AllowedDomains = tc.domains
			err := v.Validate(p)
			if tc.expectErr != nil {
				if err == nil || !errors.Is(err, tc.expectErr) {
					t.Errorf("expected error %v, got: %v", tc.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected valid domains, got: %v", err)
				}
			}
		})
	}
}

func TestValidator_StatusRules(t *testing.T) {
	v := manifest.NewValidator()

	// Valid statuses
	for _, st := range []providers.ProviderStatus{providers.ProviderStatusActive, providers.ProviderStatusDisabled} {
		p := validProvider()
		p.Status = st
		if err := v.Validate(p); err != nil {
			t.Errorf("expected status %q to be valid, got: %v", st, err)
		}
	}

	// Invalid statuses
	for _, st := range []providers.ProviderStatus{"", "enabled", "paused", "ACTIVE"} {
		p := validProvider()
		p.Status = st
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrInvalidProviderStatus) {
			t.Errorf("expected ErrInvalidProviderStatus for %q, got: %v", st, err)
		}
	}
}

func TestValidator_ProbeRules(t *testing.T) {
	v := manifest.NewValidator()

	t.Run("empty identity expression", func(t *testing.T) {
		p := validProvider()
		p.ProbeRules.IdentityExpressions = []string{"[data-user]", "  "}
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrInvalidProbeRules) {
			t.Errorf("expected ErrInvalidProbeRules for blank identity expression, got: %v", err)
		}
	})

	t.Run("empty token extractor key", func(t *testing.T) {
		p := validProvider()
		p.ProbeRules.TokenExtractors = map[string]string{
			"": "regex-pattern",
		}
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrInvalidProbeRules) {
			t.Errorf("expected ErrInvalidProbeRules for empty token extractor key, got: %v", err)
		}
	})

	t.Run("empty token extractor pattern", func(t *testing.T) {
		p := validProvider()
		p.ProbeRules.TokenExtractors = map[string]string{
			"csrf": "   ",
		}
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrInvalidProbeRules) {
			t.Errorf("expected ErrInvalidProbeRules for empty token extractor pattern, got: %v", err)
		}
	})

	t.Run("invalid regex pattern", func(t *testing.T) {
		p := validProvider()
		p.ProbeRules.TokenExtractors = map[string]string{
			"token": "[a-z(",
		}
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrInvalidProbeRules) {
			t.Errorf("expected ErrInvalidProbeRules for malformed regex, got: %v", err)
		}
	})

	t.Run("empty discovery keyword", func(t *testing.T) {
		p := validProvider()
		p.ProbeRules.DiscoveryKeywords = []string{"BatchGenerateImages", ""}
		err := v.Validate(p)
		if err == nil || !errors.Is(err, manifest.ErrInvalidProbeRules) {
			t.Errorf("expected ErrInvalidProbeRules for empty discovery keyword, got: %v", err)
		}
	})
}

func TestValidator_MultiErrors(t *testing.T) {
	v := manifest.NewValidator()
	p := &providers.Provider{
		ID:             "",
		Name:           "",
		BaseURL:        "ftp://invalid",
		AuthDomain:     "",
		AllowedDomains: nil,
		Status:         "unknown",
	}

	err := v.Validate(p)
	if err == nil {
		t.Fatal("expected multiple errors, got nil")
	}

	// Verify all errors were collected
	if !errors.Is(err, manifest.ErrEmptyID) {
		t.Errorf("expected ErrEmptyID in joined error, got: %v", err)
	}
	if !errors.Is(err, manifest.ErrEmptyName) {
		t.Errorf("expected ErrEmptyName in joined error, got: %v", err)
	}
	if !errors.Is(err, manifest.ErrInvalidBaseURL) {
		t.Errorf("expected ErrInvalidBaseURL in joined error, got: %v", err)
	}
	if !errors.Is(err, manifest.ErrEmptyAuthDomain) {
		t.Errorf("expected ErrEmptyAuthDomain in joined error, got: %v", err)
	}
	if !errors.Is(err, manifest.ErrEmptyAllowedDomains) {
		t.Errorf("expected ErrEmptyAllowedDomains in joined error, got: %v", err)
	}
	if !errors.Is(err, manifest.ErrInvalidProviderStatus) {
		t.Errorf("expected ErrInvalidProviderStatus in joined error, got: %v", err)
	}
}
