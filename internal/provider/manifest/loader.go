package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/provider/embedded"
)

// Loader loads and validates provider manifests from multiple sources.
type Loader interface {
	LoadFile(path string) (*providers.Provider, error)
	LoadDirectory(dir string) ([]*providers.Provider, error)
	LoadBytes(data []byte) (*providers.Provider, error)
	LoadEmbedded() ([]*providers.Provider, error)
	LoadWithFallback(dir string) ([]*providers.Provider, error)
}

// LoaderImpl implements the Loader interface.
type LoaderImpl struct {
	validator Validator
	embedFS   fs.FS
}

// LoaderOption configures the LoaderImpl.
type LoaderOption func(*LoaderImpl)

// WithFS sets a custom filesystem for embedded manifest loading (useful for testing).
func WithFS(embedFS fs.FS) LoaderOption {
	return func(l *LoaderImpl) {
		l.embedFS = embedFS
	}
}

// NewLoader creates a new manifest Loader instance.
func NewLoader(validator Validator, opts ...LoaderOption) *LoaderImpl {
	if validator == nil {
		validator = NewValidator()
	}
	l := &LoaderImpl{
		validator: validator,
		embedFS:   embedded.ManifestsFS,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// LoadBytes unmarshals and validates provider manifest JSON data.
func (l *LoaderImpl) LoadBytes(data []byte) (*providers.Provider, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, errors.New("manifest: empty manifest data")
	}

	var p providers.Provider
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("manifest: unmarshal json: %w", err)
	}

	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}

	if err := l.validator.Validate(&p); err != nil {
		return nil, fmt.Errorf("manifest: validation failed: %w", err)
	}

	return &p, nil
}

// LoadFile reads and validates a provider manifest from the specified file path.
func (l *LoaderImpl) LoadFile(path string) (*providers.Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: read file %q: %w", path, err)
	}

	p, err := l.LoadBytes(data)
	if err != nil {
		return nil, fmt.Errorf("manifest: load %q: %w", path, err)
	}

	return p, nil
}

// LoadDirectory scans a directory and loads all *.json provider manifests.
func (l *LoaderImpl) LoadDirectory(dir string) ([]*providers.Provider, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("manifest: stat directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("manifest: path %q is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("manifest: read directory %q: %w", dir, err)
	}

	var jsonFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			jsonFiles = append(jsonFiles, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(jsonFiles)

	if len(jsonFiles) == 0 {
		return []*providers.Provider{}, nil
	}

	seenIDs := make(map[string]string) // id -> filepath
	var loaded []*providers.Provider

	for _, file := range jsonFiles {
		p, err := l.LoadFile(file)
		if err != nil {
			return nil, err
		}

		if existingFile, exists := seenIDs[p.ID]; exists {
			return nil, fmt.Errorf("manifest: duplicate provider ID %q in %q (previously loaded from %q)", p.ID, file, existingFile)
		}
		seenIDs[p.ID] = file
		loaded = append(loaded, p)
	}

	return loaded, nil
}

// LoadEmbedded loads all embedded provider manifests.
func (l *LoaderImpl) LoadEmbedded() ([]*providers.Provider, error) {
	if l.embedFS == nil {
		return nil, errors.New("manifest: embedded filesystem not configured")
	}

	entries, err := fs.ReadDir(l.embedFS, "manifests")
	if err != nil {
		return nil, fmt.Errorf("manifest: read embedded manifests: %w", err)
	}

	var jsonNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			jsonNames = append(jsonNames, entry.Name())
		}
	}
	sort.Strings(jsonNames)

	seenIDs := make(map[string]string)
	var loaded []*providers.Provider

	for _, name := range jsonNames {
		data, err := fs.ReadFile(l.embedFS, "manifests/"+name)
		if err != nil {
			return nil, fmt.Errorf("manifest: read embedded file %q: %w", name, err)
		}

		p, err := l.LoadBytes(data)
		if err != nil {
			return nil, fmt.Errorf("manifest: load embedded %q: %w", name, err)
		}

		if existing, exists := seenIDs[p.ID]; exists {
			return nil, fmt.Errorf("manifest: duplicate embedded provider ID %q in %q (previously in %q)", p.ID, name, existing)
		}
		seenIDs[p.ID] = name
		loaded = append(loaded, p)
	}

	return loaded, nil
}

// LoadWithFallback attempts to load manifests from dir, falling back to embedded manifests
// if dir does not exist or contains no *.json files.
func (l *LoaderImpl) LoadWithFallback(dir string) ([]*providers.Provider, error) {
	if dir != "" {
		ps, err := l.LoadDirectory(dir)
		if err == nil && len(ps) > 0 {
			return ps, nil
		}
		if err != nil && !os.IsNotExist(err) {
			// If dir exists and failed, do not silently swallow syntax or validation errors!
			if _, statErr := os.Stat(dir); statErr == nil {
				return nil, err
			}
		}
	}

	return l.LoadEmbedded()
}
