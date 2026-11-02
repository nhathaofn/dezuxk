package manifest_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/provider/manifest"
)

const baseValidManifest = `{
  "id": "test-provider",
  "name": "Test Provider",
  "base_url": "https://api.example.com",
  "auth_domain": ".example.com",
  "allowed_domains": [".example.com"],
  "probe_rules": {
    "identityExpressions": ["[data-id]"],
    "discoveryKeywords": ["GenerateRPC"],
    "tokenExtractors": {
      "xsrf": "token=([^;]+)"
    }
  },
  "status": "active"
}`

// TestAdversarial_MalformedJSON tests loader resilience against corrupt, truncated,
// primitive, array, and non-object JSON payloads.
func TestAdversarial_MalformedJSON(t *testing.T) {
	loader := manifest.NewLoader(nil)

	testCases := []struct {
		name    string
		payload []byte
	}{
		{"nil slice", nil},
		{"empty slice", []byte{}},
		{"whitespace only", []byte("   \t\r\n  ")},
		{"truncated opening brace", []byte(`{ "id": "flow"`)},
		{"truncated field", []byte(`{ "id": `)},
		{"syntax error unquoted key", []byte(`{ id: "flow" }`)},
		{"trailing comma", []byte(`{ "id": "flow", }`)},
		{"single number primitive", []byte(`12345`)},
		{"single string primitive", []byte(`"hello world"`)},
		{"single boolean primitive", []byte(`true`)},
		{"json null", []byte(`null`)},
		{"json array of objects", []byte(`[{"id": "flow"}]`)},
		{"empty json object", []byte(`{}`)}, // Missing required fields -> validation error
		{"garbage non-ascii binary", []byte{0x00, 0xFF, 0xFE, 0x12, 0x34}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := loader.LoadBytes(tc.payload)
			if err == nil {
				t.Fatalf("expected error for malformed payload %q, got provider: %+v", tc.name, p)
			}
		})
	}
}

// TestAdversarial_InvalidUTF8 tests loader behavior when payload contains invalid UTF-8 byte sequences.
func TestAdversarial_InvalidUTF8(t *testing.T) {
	loader := manifest.NewLoader(nil)

	// JSON payload with invalid UTF-8 sequence embedded in string field
	invalidUTF8JSON := []byte("{\n  \"id\": \"flow\",\n  \"name\": \"Google \xFF\xFE Flow\",\n  \"base_url\": \"https://flow.google.com\",\n  \"auth_domain\": \".google.com\",\n  \"allowed_domains\": [\".google.com\"],\n  \"status\": \"active\"\n}")

	p, err := loader.LoadBytes(invalidUTF8JSON)
	// Go's standard json.Unmarshal may replace or error on invalid UTF-8, or validator may handle it.
	// We verify it does not panic and either validates safely or errors gracefully.
	if err != nil {
		t.Logf("Invalid UTF-8 rejected as expected: %v", err)
	} else if p != nil {
		t.Logf("Invalid UTF-8 preserved safely without panic: ID=%q, Name=%q", p.ID, p.Name)
	}
}

// TestAdversarial_UTF8BOM tests JSON files prefixed with UTF-8 Byte Order Mark (\xEF\xBB\xBF).
func TestAdversarial_UTF8BOM(t *testing.T) {
	loader := manifest.NewLoader(nil)

	bomPayload := append([]byte{0xEF, 0xBB, 0xBF}, []byte(baseValidManifest)...)

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "manifest_bom.json")
	if err := os.WriteFile(filePath, bomPayload, 0644); err != nil {
		t.Fatal(err)
	}

	p, err := loader.LoadFile(filePath)
	// Go's encoding/json does not natively strip BOM at offset 0 unless sanitized.
	// Verify behavior: either succeeds or reports clean unmarshal error (no crash/hang).
	t.Logf("UTF-8 BOM file loading result: provider=%v, err=%v", p != nil, err)
}

// TestAdversarial_OversizedManifest tests loader resilience when processing large files.
func TestAdversarial_OversizedManifest(t *testing.T) {
	loader := manifest.NewLoader(nil)

	// Create a provider with 2,000 allowed domains (~100KB manifest)
	domains := make([]string, 2000)
	for i := 0; i < 2000; i++ {
		domains[i] = fmt.Sprintf("sub%d.example.com", i)
	}

	p := &providers.Provider{
		ID:             "large-manifest",
		Name:           "Large Provider",
		BaseURL:        "https://example.com",
		AuthDomain:     "example.com",
		AllowedDomains: domains,
		Status:         providers.ProviderStatusActive,
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := loader.LoadBytes(data)
	if err != nil {
		t.Fatalf("unexpected failure on large manifest: %v", err)
	}
	if len(loaded.AllowedDomains) != 2000 {
		t.Errorf("expected 2000 domains, got %d", len(loaded.AllowedDomains))
	}
}

// TestAdversarial_IDBoundaryLimits tests ID string length limits (128 chars allowed, 129 chars rejected).
func TestAdversarial_IDBoundaryLimits(t *testing.T) {
	v := manifest.NewValidator()

	// 128 chars
	id128 := strings.Repeat("a", 128)
	p128 := &providers.Provider{
		ID:             id128,
		Name:           "Valid Limit",
		BaseURL:        "https://example.com",
		AuthDomain:     "example.com",
		AllowedDomains: []string{"example.com"},
		Status:         providers.ProviderStatusActive,
	}
	if err := v.Validate(p128); err != nil {
		t.Errorf("expected 128-char ID to be valid, got: %v", err)
	}

	// 129 chars
	id129 := strings.Repeat("a", 129)
	p129 := &providers.Provider{
		ID:             id129,
		Name:           "Invalid Limit",
		BaseURL:        "https://example.com",
		AuthDomain:     "example.com",
		AllowedDomains: []string{"example.com"},
		Status:         providers.ProviderStatusActive,
	}
	if err := v.Validate(p129); err == nil {
		t.Errorf("expected 129-char ID to be rejected, got nil")
	}
}

// TestAdversarial_DirectoryScan_DuplicateIDs tests duplicate ID collision detection across multiple files.
func TestAdversarial_DirectoryScan_DuplicateIDs(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "01_provider.json")
	file2 := filepath.Join(tmpDir, "02_provider.json")

	if err := os.WriteFile(file1, []byte(baseValidManifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte(baseValidManifest), 0644); err != nil {
		t.Fatal(err)
	}

	loader := manifest.NewLoader(nil)
	provs, err := loader.LoadDirectory(tmpDir)
	if err == nil {
		t.Fatalf("expected duplicate ID error across files, got: %+v", provs)
	}

	expectedSubstring := "duplicate provider ID"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Errorf("expected error message to contain %q, got: %q", expectedSubstring, err.Error())
	}
	t.Logf("Duplicate ID error properly detailed: %v", err)
}

// TestAdversarial_DirectoryScan_SymlinksAndNestedSubdirs tests that nested directories are ignored
// and don't cause traversal errors or panics.
func TestAdversarial_DirectoryScan_SymlinksAndNestedSubdirs(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Root valid manifest
	if err := os.WriteFile(filepath.Join(tmpDir, "root.json"), []byte(baseValidManifest), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Subdirectory with a manifest (should be ignored by flat directory scan)
	subDir := filepath.Join(tmpDir, "nested_subfolder")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.json"), []byte(baseValidManifest), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Subdirectory with .json extension (should be ignored because it is a directory)
	subDirWithJSONExt := filepath.Join(tmpDir, "fake_file.json")
	if err := os.Mkdir(subDirWithJSONExt, 0755); err != nil {
		t.Fatal(err)
	}

	// 4. Non-JSON files in directory
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("# Ignore me"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".DS_Store"), []byte("junk"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := manifest.NewLoader(nil)
	provs, err := loader.LoadDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error scanning directory with nested items: %v", err)
	}

	if len(provs) != 1 {
		t.Fatalf("expected exactly 1 provider loaded from root, got %d", len(provs))
	}
	if provs[0].ID != "test-provider" {
		t.Errorf("expected provider ID 'test-provider', got %q", provs[0].ID)
	}
}

// TestAdversarial_LoadWithFallback_DirIsFile tests that pointing manifests directory to a regular file
// returns an error rather than silently loading embedded fallbacks.
func TestAdversarial_LoadWithFallback_DirIsFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "regular_file.txt")
	if err := os.WriteFile(filePath, []byte("i am not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := manifest.NewLoader(nil)
	_, err := loader.LoadWithFallback(filePath)
	if err == nil {
		t.Fatal("expected error when fallback target is a file instead of directory, got nil")
	}
	t.Logf("Fallback error for file target: %v", err)
}

// TestAdversarial_Validator_EdgeCases tests an exhaustive matrix of edge cases for the schema validator.
func TestAdversarial_Validator_EdgeCases(t *testing.T) {
	v := manifest.NewValidator()

	t.Run("BaseURL Scheme Edge Cases", func(t *testing.T) {
		schemes := []struct {
			url   string
			valid bool
		}{
			{"http://example.com", true},
			{"https://example.com", true},
			{"https://example.com:8443/v1/api", true},
			{"ftp://example.com", false},
			{"file:///etc/passwd", false},
			{"javascript:void(0)", false},
			{"data:text/plain;base64,SGVsbG8=", false},
			{"ws://example.com", false},
			{"wss://example.com", false},
			{"http://", false},
			{"https://", false},
			{"https:///path/only", false},
		}

		for _, s := range schemes {
			p := &providers.Provider{
				ID:             "scheme-test",
				Name:           "Scheme Test",
				BaseURL:        s.url,
				AuthDomain:     "example.com",
				AllowedDomains: []string{"example.com"},
				Status:         providers.ProviderStatusActive,
			}
			err := v.Validate(p)
			if s.valid && err != nil {
				t.Errorf("expected %q to be valid, got: %v", s.url, err)
			}
			if !s.valid && err == nil {
				t.Errorf("expected %q to be rejected, got nil", s.url)
			}
		}
	})

	t.Run("AuthDomain and AllowedDomain Wildcards and Hosts", func(t *testing.T) {
		domains := []struct {
			domain string
			valid  bool
		}{
			{".google.com", true},
			{"google.com", true},
			{"*.google.com", true},
			{"api.sub.example.co.uk", true},
			{"localhost", true},
			{"localhost:8080", true},
			{"127.0.0.1", true},
			{"google..com", false},
			{".", false},
			{"*.", false},
			{"google.com/path", false},
			{"google.com:badport:extra", false},
			{" spaces .com ", false},
		}

		for _, d := range domains {
			p := &providers.Provider{
				ID:             "domain-test",
				Name:           "Domain Test",
				BaseURL:        "https://example.com",
				AuthDomain:     d.domain,
				AllowedDomains: []string{d.domain},
				Status:         providers.ProviderStatusActive,
			}
			err := v.Validate(p)
			if d.valid && err != nil {
				t.Errorf("expected domain %q to be valid, got: %v", d.domain, err)
			}
			if !d.valid && err == nil {
				t.Errorf("expected domain %q to be rejected, got nil", d.domain)
			}
		}
	})

	t.Run("Status Strict Case Sensitivity", func(t *testing.T) {
		statuses := []struct {
			status providers.ProviderStatus
			valid  bool
		}{
			{providers.ProviderStatusActive, true},
			{providers.ProviderStatusDisabled, true},
			{"Active", false},
			{"ACTIVE", false},
			{"Disabled", false},
			{"DISABLED", false},
			{"running", false},
			{"ready", false},
			{"", false},
		}

		for _, st := range statuses {
			p := &providers.Provider{
				ID:             "status-test",
				Name:           "Status Test",
				BaseURL:        "https://example.com",
				AuthDomain:     "example.com",
				AllowedDomains: []string{"example.com"},
				Status:         st.status,
			}
			err := v.Validate(p)
			if st.valid && err != nil {
				t.Errorf("expected status %q to be valid, got: %v", st.status, err)
			}
			if !st.valid && err == nil {
				t.Errorf("expected status %q to be rejected, got nil", st.status)
			}
		}
	})
}
