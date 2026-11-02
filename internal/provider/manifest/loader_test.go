package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/provider/manifest"
)

const sampleValidJSON = `{
  "id": "flow",
  "name": "Google Flow",
  "base_url": "https://flow.google.com",
  "auth_domain": ".google.com",
  "allowed_domains": [
    ".google.com",
    "flow.google.com"
  ],
  "probe_rules": {
    "identityExpressions": [
      "[data-email]"
    ],
    "discoveryKeywords": [
      "BatchGenerateImages"
    ]
  },
  "status": "active"
}`

const sampleProvider2JSON = `{
  "id": "gemini",
  "name": "Google Gemini",
  "base_url": "https://gemini.google.com",
  "auth_domain": ".google.com",
  "allowed_domains": [
    ".google.com"
  ],
  "status": "active"
}`

func TestLoader_LoadBytes_Success(t *testing.T) {
	loader := manifest.NewLoader(nil)
	p, err := loader.LoadBytes([]byte(sampleValidJSON))
	if err != nil {
		t.Fatalf("unexpected error loading valid JSON: %v", err)
	}

	if p.ID != "flow" {
		t.Errorf("expected ID 'flow', got %q", p.ID)
	}
	if p.Name != "Google Flow" {
		t.Errorf("expected Name 'Google Flow', got %q", p.Name)
	}
	if p.BaseURL != "https://flow.google.com" {
		t.Errorf("expected BaseURL 'https://flow.google.com', got %q", p.BaseURL)
	}
	if p.Status != providers.ProviderStatusActive {
		t.Errorf("expected Status 'active', got %q", p.Status)
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Errorf("expected non-zero timestamps, got CreatedAt=%v UpdatedAt=%v", p.CreatedAt, p.UpdatedAt)
	}
}

func TestLoader_LoadBytes_Empty(t *testing.T) {
	loader := manifest.NewLoader(nil)
	_, err := loader.LoadBytes([]byte("   "))
	if err == nil {
		t.Fatal("expected error for empty bytes, got nil")
	}
}

func TestLoader_LoadBytes_MalformedJSON(t *testing.T) {
	loader := manifest.NewLoader(nil)
	_, err := loader.LoadBytes([]byte(`{ "id": "flow", invalid json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestLoader_LoadBytes_ValidationError(t *testing.T) {
	loader := manifest.NewLoader(nil)
	invalidJSON := `{ "id": "flow invalid id", "name": "Flow", "base_url": "ftp://bad", "auth_domain": "google.com", "allowed_domains": ["google.com"], "status": "active" }`
	_, err := loader.LoadBytes([]byte(invalidJSON))
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestLoader_LoadFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "flow.json")
	if err := os.WriteFile(filePath, []byte(sampleValidJSON), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	loader := manifest.NewLoader(nil)
	p, err := loader.LoadFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error loading file: %v", err)
	}
	if p.ID != "flow" {
		t.Errorf("expected ID 'flow', got %q", p.ID)
	}
}

func TestLoader_LoadFile_NotFound(t *testing.T) {
	loader := manifest.NewLoader(nil)
	_, err := loader.LoadFile("non_existent_file.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoader_LoadDirectory_MultipleManifests(t *testing.T) {
	tmpDir := t.TempDir()

	// Write 2 valid manifests
	if err := os.WriteFile(filepath.Join(tmpDir, "b_gemini.json"), []byte(sampleProvider2JSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "a_flow.json"), []byte(sampleValidJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a non-json file that should be ignored
	if err := os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("not a json"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := manifest.NewLoader(nil)
	provs, err := loader.LoadDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error loading directory: %v", err)
	}

	if len(provs) != 2 {
		t.Fatalf("expected 2 providers loaded, got %d", len(provs))
	}

	// Check deterministic sorting by filename
	if provs[0].ID != "flow" {
		t.Errorf("expected first provider 'flow', got %q", provs[0].ID)
	}
	if provs[1].ID != "gemini" {
		t.Errorf("expected second provider 'gemini', got %q", provs[1].ID)
	}
}

func TestLoader_LoadDirectory_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	loader := manifest.NewLoader(nil)

	provs, err := loader.LoadDirectory(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error loading empty directory: %v", err)
	}
	if len(provs) != 0 {
		t.Fatalf("expected 0 providers, got %d", len(provs))
	}
}

func TestLoader_LoadDirectory_DuplicateID(t *testing.T) {
	tmpDir := t.TempDir()

	// Two files defining provider with id "flow"
	if err := os.WriteFile(filepath.Join(tmpDir, "1_flow.json"), []byte(sampleValidJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "2_flow.json"), []byte(sampleValidJSON), 0644); err != nil {
		t.Fatal(err)
	}

	loader := manifest.NewLoader(nil)
	_, err := loader.LoadDirectory(tmpDir)
	if err == nil {
		t.Fatal("expected duplicate ID error, got nil")
	}
}

func TestLoader_LoadEmbedded_Success(t *testing.T) {
	loader := manifest.NewLoader(nil)
	provs, err := loader.LoadEmbedded()
	if err != nil {
		t.Fatalf("unexpected error loading embedded manifests: %v", err)
	}

	if len(provs) == 0 {
		t.Fatal("expected at least 1 embedded provider, got 0")
	}

	foundFlow := false
	for _, p := range provs {
		if p.ID == "flow" {
			foundFlow = true
			if p.Status != providers.ProviderStatusActive {
				t.Errorf("expected flow status 'active', got %q", p.Status)
			}
			if p.BaseURL != "https://flow.google.com" {
				t.Errorf("expected flow BaseURL 'https://flow.google.com', got %q", p.BaseURL)
			}
		}
	}
	if !foundFlow {
		t.Error("expected embedded manifests to contain provider 'flow'")
	}
}

func TestLoader_LoadWithFallback_DirHasFiles(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "gemini.json"), []byte(sampleProvider2JSON), 0644); err != nil {
		t.Fatal(err)
	}

	loader := manifest.NewLoader(nil)
	provs, err := loader.LoadWithFallback(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(provs) != 1 || provs[0].ID != "gemini" {
		t.Fatalf("expected 1 provider 'gemini', got %+v", provs)
	}
}

func TestLoader_LoadWithFallback_DirNotExist(t *testing.T) {
	loader := manifest.NewLoader(nil)
	provs, err := loader.LoadWithFallback("non_existent_manifests_directory_xyz")
	if err != nil {
		t.Fatalf("unexpected error in fallback: %v", err)
	}

	if len(provs) == 0 {
		t.Fatal("expected embedded providers in fallback, got 0")
	}
	if provs[0].ID != "flow" {
		t.Errorf("expected embedded provider 'flow', got %q", provs[0].ID)
	}
}

func TestLoader_LoadWithFallback_DirEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	loader := manifest.NewLoader(nil)

	provs, err := loader.LoadWithFallback(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error in fallback for empty dir: %v", err)
	}

	if len(provs) == 0 {
		t.Fatal("expected fallback to embedded when dir is empty, got 0")
	}
	if provs[0].ID != "flow" {
		t.Errorf("expected embedded provider 'flow', got %q", provs[0].ID)
	}
}

func TestLoader_LoadWithFallback_DirHasMalformedManifest(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "broken.json"), []byte(`{ not json }`), 0644); err != nil {
		t.Fatal(err)
	}

	loader := manifest.NewLoader(nil)
	_, err := loader.LoadWithFallback(tmpDir)
	if err == nil {
		t.Fatal("expected error when directory contains malformed manifest, but got nil")
	}
}
