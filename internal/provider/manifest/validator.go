// Package manifest handles provider schema validation and dynamic loading.
package manifest

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"dezuxk/internal/domain/providers"
)

var (
	// ErrNilProvider is returned when provider is nil.
	ErrNilProvider = errors.New("manifest: provider cannot be nil")
	// ErrEmptyID is returned when provider ID is empty.
	ErrEmptyID = errors.New("manifest: provider ID cannot be empty")
	// ErrInvalidID is returned when provider ID contains invalid characters or exceeds 128 chars.
	ErrInvalidID = errors.New("manifest: provider ID must be alphanumeric and may contain hyphens or underscores")
	// ErrEmptyName is returned when provider name is empty.
	ErrEmptyName = errors.New("manifest: name cannot be empty")
	// ErrEmptyBaseURL is returned when base_url is empty.
	ErrEmptyBaseURL = errors.New("manifest: base_url cannot be empty")
	// ErrInvalidBaseURL is returned when base_url is not a valid absolute HTTP or HTTPS URL with a host.
	ErrInvalidBaseURL = errors.New("manifest: base_url must be a valid HTTP or HTTPS URL with a host")
	// ErrEmptyAuthDomain is returned when auth_domain is empty.
	ErrEmptyAuthDomain = errors.New("manifest: auth_domain cannot be empty")
	// ErrInvalidAuthDomain is returned when auth_domain is not a valid domain pattern.
	ErrInvalidAuthDomain = errors.New("manifest: auth_domain must be a valid domain pattern")
	// ErrEmptyAllowedDomains is returned when allowed_domains is empty.
	ErrEmptyAllowedDomains = errors.New("manifest: allowed_domains must contain at least one domain")
	// ErrInvalidAllowedDomain is returned when an allowed domain pattern is invalid.
	ErrInvalidAllowedDomain = errors.New("manifest: invalid allowed_domain pattern")
	// ErrInvalidProviderStatus is returned when status is neither active nor disabled.
	ErrInvalidProviderStatus = errors.New("manifest: status must be 'active' or 'disabled'")
	// ErrInvalidProbeRules is returned when probe_rules contains empty values or invalid regex.
	ErrInvalidProbeRules = errors.New("manifest: probe_rules contains invalid expressions or regex patterns")
)

var (
	idRegex     = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	domainRegex = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z0-9-]{2,}$`)
)

// Validator validates provider manifest schemas.
type Validator interface {
	Validate(p *providers.Provider) error
}

// SchemaValidator implements strict validation for provider manifests.
type SchemaValidator struct{}

// NewValidator creates a new SchemaValidator instance.
func NewValidator() *SchemaValidator {
	return &SchemaValidator{}
}

func isValidDomainOrHost(d string) bool {
	clean := strings.TrimSpace(d)
	clean = strings.TrimPrefix(clean, "*.")
	clean = strings.TrimPrefix(clean, ".")
	if clean == "" {
		return false
	}
	host := clean
	if strings.Contains(clean, ":") {
		if h, _, err := net.SplitHostPort(clean); err == nil {
			host = h
		}
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	return domainRegex.MatchString(host)
}

// Validate performs strict validation on all required provider manifest fields.
func (v *SchemaValidator) Validate(p *providers.Provider) error {
	if p == nil {
		return ErrNilProvider
	}

	var errs []error

	// 1. Validate ID
	id := strings.TrimSpace(p.ID)
	if id == "" {
		errs = append(errs, ErrEmptyID)
	} else if !idRegex.MatchString(id) || len(id) > 128 {
		errs = append(errs, fmt.Errorf("%w: got %q (must match ^[a-zA-Z0-9_-]+$)", ErrInvalidID, p.ID))
	}

	// 2. Validate Name
	if strings.TrimSpace(p.Name) == "" {
		errs = append(errs, ErrEmptyName)
	}

	// 3. Validate BaseURL
	baseURL := strings.TrimSpace(p.BaseURL)
	if baseURL == "" {
		errs = append(errs, ErrEmptyBaseURL)
	} else {
		u, err := url.ParseRequestURI(baseURL)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %v", ErrInvalidBaseURL, err))
		} else if u.Scheme != "http" && u.Scheme != "https" {
			errs = append(errs, fmt.Errorf("%w: unsupported scheme %q (must be http or https)", ErrInvalidBaseURL, u.Scheme))
		} else if u.Host == "" || u.Hostname() == "" {
			errs = append(errs, fmt.Errorf("%w: missing host in URL %q", ErrInvalidBaseURL, p.BaseURL))
		}
	}

	// 4. Validate AuthDomain
	authDomain := strings.TrimSpace(p.AuthDomain)
	if authDomain == "" {
		errs = append(errs, ErrEmptyAuthDomain)
	} else {
		if !isValidDomainOrHost(authDomain) {
			errs = append(errs, fmt.Errorf("%w: got %q", ErrInvalidAuthDomain, p.AuthDomain))
		}
	}

	// 5. Validate AllowedDomains
	if len(p.AllowedDomains) == 0 {
		errs = append(errs, ErrEmptyAllowedDomains)
	} else {
		for idx, d := range p.AllowedDomains {
			dTrimmed := strings.TrimSpace(d)
			if dTrimmed == "" {
				errs = append(errs, fmt.Errorf("%w at index %d: domain cannot be blank", ErrInvalidAllowedDomain, idx))
				continue
			}
			if !isValidDomainOrHost(dTrimmed) {
				errs = append(errs, fmt.Errorf("%w at index %d: invalid domain %q", ErrInvalidAllowedDomain, idx, d))
			}
		}
	}

	// 6. Validate Status
	if p.Status != providers.ProviderStatusActive && p.Status != providers.ProviderStatusDisabled {
		errs = append(errs, fmt.Errorf("%w: got %q", ErrInvalidProviderStatus, p.Status))
	}

	// 7. Validate ProbeRules
	for idx, expr := range p.ProbeRules.IdentityExpressions {
		if strings.TrimSpace(expr) == "" {
			errs = append(errs, fmt.Errorf("%w: identity expression at index %d is empty", ErrInvalidProbeRules, idx))
		}
	}
	for key, pattern := range p.ProbeRules.TokenExtractors {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, fmt.Errorf("%w: token extractor key is empty", ErrInvalidProbeRules))
		}
		if strings.TrimSpace(pattern) == "" {
			errs = append(errs, fmt.Errorf("%w: token extractor pattern for %q is empty", ErrInvalidProbeRules, key))
		} else {
			if _, err := regexp.Compile(pattern); err != nil {
				errs = append(errs, fmt.Errorf("%w: token extractor regex for %q is invalid: %v", ErrInvalidProbeRules, key, err))
			}
		}
	}
	for idx, kw := range p.ProbeRules.DiscoveryKeywords {
		if strings.TrimSpace(kw) == "" {
			errs = append(errs, fmt.Errorf("%w: discovery keyword at index %d is empty", ErrInvalidProbeRules, idx))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
