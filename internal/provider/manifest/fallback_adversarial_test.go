package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"dezuxk/internal/provider/manifest"
)

// TestAdversarial_Fallback_Mechanics empirically tests all edge cases of manifest fallback.
func TestAdversarial_Fallback_Mechanics(t *testing.T) {
	loader := manifest.NewLoader(nil)

	t.Run("NonExistentDirectory_LoadsEmbeddedManifests", func(t *testing.T) {
		nonExistentDir := filepath.Join(t.TempDir(), "non_existent_manifests_path")
		provs, err := loader.LoadWithFallback(nonExistentDir)
		if err != nil {
			t.Fatalf("expected graceful fallback to embedded manifests, got err: %v", err)
		}
		if len(provs) == 0 {
			t.Fatal("expected at least 1 embedded provider loaded, got 0")
		}

		foundFlow := false
		for _, p := range provs {
			if p.ID == "flow" {
				foundFlow = true
			}
		}
		if !foundFlow {
			t.Errorf("expected embedded provider 'flow' in fallback results")
		}
	})

	t.Run("EmptyDirectory_LoadsEmbeddedManifests", func(t *testing.T) {
		emptyDir := t.TempDir()
		provs, err := loader.LoadWithFallback(emptyDir)
		if err != nil {
			t.Fatalf("expected fallback for empty directory, got: %v", err)
		}
		if len(provs) == 0 {
			t.Fatal("expected embedded manifests in fallback, got 0")
		}
	})

	t.Run("DirectoryWithOnlyNonJSONFiles_LoadsEmbeddedManifests", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Manifests"), 0644)
		_ = os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("id: flow"), 0644)

		provs, err := loader.LoadWithFallback(dir)
		if err != nil {
			t.Fatalf("expected fallback when no .json files exist, got: %v", err)
		}
		if len(provs) == 0 {
			t.Fatal("expected embedded fallback when directory has no .json files")
		}
	})

	t.Run("CustomManifestDirectory_PrecedenceOverEmbedded", func(t *testing.T) {
		dir := t.TempDir()
		customProviderJSON := `{
			"id": "custom-ai",
			"name": "Custom AI Provider",
			"base_url": "https://custom.ai.example.com",
			"auth_domain": ".example.com",
			"allowed_domains": [".example.com"],
			"status": "active"
		}`
		if err := os.WriteFile(filepath.Join(dir, "custom.json"), []byte(customProviderJSON), 0644); err != nil {
			t.Fatal(err)
		}

		provs, err := loader.LoadWithFallback(dir)
		if err != nil {
			t.Fatalf("unexpected error loading custom directory: %v", err)
		}

		// When custom directory has manifests, custom manifests take precedence.
		// Embedded manifests must NOT be appended or merged.
		if len(provs) != 1 {
			t.Fatalf("expected exactly 1 provider from custom dir, got %d", len(provs))
		}
		if provs[0].ID != "custom-ai" {
			t.Errorf("expected custom provider 'custom-ai', got %q", provs[0].ID)
		}
	})

	t.Run("CustomManifest_OverridesEmbeddedFlowID", func(t *testing.T) {
		dir := t.TempDir()
		customFlowJSON := `{
			"id": "flow",
			"name": "Custom Google Flow Override",
			"base_url": "https://custom-flow.internal.corp",
			"auth_domain": ".internal.corp",
			"allowed_domains": [".internal.corp"],
			"status": "active"
		}`
		if err := os.WriteFile(filepath.Join(dir, "flow.json"), []byte(customFlowJSON), 0644); err != nil {
			t.Fatal(err)
		}

		provs, err := loader.LoadWithFallback(dir)
		if err != nil {
			t.Fatalf("unexpected error loading override: %v", err)
		}

		if len(provs) != 1 {
			t.Fatalf("expected 1 provider, got %d", len(provs))
		}
		if provs[0].Name != "Custom Google Flow Override" {
			t.Errorf("expected custom overridden name, got %q", provs[0].Name)
		}
		if provs[0].BaseURL != "https://custom-flow.internal.corp" {
			t.Errorf("expected custom overridden BaseURL, got %q", provs[0].BaseURL)
		}
	})

	t.Run("DirectoryExistsWithCorruptJSON_LoadWithFallbackReturnsError", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{ broken json`), 0644)

		_, err := loader.LoadWithFallback(dir)
		if err == nil {
			t.Fatal("CRITICAL: LoadWithFallback silently masked syntax error in existing directory instead of returning error")
		}
	})

	t.Run("DirectoryExistsWithInvalidSchema_LoadWithFallbackReturnsError", func(t *testing.T) {
		dir := t.TempDir()
		invalidSchemaJSON := `{
			"id": "bad-prov",
			"name": "Bad Provider",
			"base_url": "ftp://bad-scheme",
			"status": "active"
		}`
		_ = os.WriteFile(filepath.Join(dir, "invalid.json"), []byte(invalidSchemaJSON), 0644)

		_, err := loader.LoadWithFallback(dir)
		if err == nil {
			t.Fatal("CRITICAL: LoadWithFallback silently masked schema validation error instead of returning error")
		}
	})
}
