package main_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findMainGoPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"main.go",
		filepath.Join("cmd", "server", "main.go"),
		filepath.Join("..", "..", "cmd", "server", "main.go"),
		filepath.Join("..", "cmd", "server", "main.go"),
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	t.Fatalf("could not locate cmd/server/main.go in candidates: %v", candidates)
	return ""
}

// TestNoHardcodedProviderManifestInSource performs static AST and lexical inspection
// of cmd/server/main.go to assert zero hardcoded provider structs, zero hardcoded URLs,
// and complete elimination of seedDefaultProviders.
func TestNoHardcodedProviderManifestInSource(t *testing.T) {
	mainPath := findMainGoPath(t)
	contentBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", mainPath, err)
	}
	content := string(contentBytes)

	// 1. Lexical checks: forbidden function and hardcoded values
	forbiddenStrings := []string{
		"seedDefaultProviders",
		"flow.google.com",
		"Google Flow",
		"BatchGenerateImages",
		"FlowService",
		"[data-email]",
		"meta[name=\"user-email\"]",
	}

	for _, forbidden := range forbiddenStrings {
		if strings.Contains(content, forbidden) {
			t.Errorf("FORBIDDEN HARDCODED VALUE FOUND in %s: %q must be completely removed from entrypoint", mainPath, forbidden)
		}
	}

	// 2. AST inspection
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, mainPath, contentBytes, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse %s as Go source: %v", mainPath, err)
	}

	// Check 2a: No function named seedDefaultProviders
	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Name.Name == "seedDefaultProviders" {
				pos := fset.Position(fn.Pos())
				t.Errorf("FORBIDDEN FUNCTION %s defined at line %d: must be removed", fn.Name.Name, pos.Line)
			}
		}
	}

	// Check 2b: No struct literals instantiating *providers.Provider or providers.Provider
	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if sel, ok := lit.Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "providers" && sel.Sel.Name == "Provider" {
					pos := fset.Position(lit.Pos())
					t.Errorf("FORBIDDEN HARDCODED STRUCT LITERAL providers.Provider instantiated at line %d: providers must be loaded dynamically", pos.Line)
				}
			}
		}
		return true
	})

	// Check 3: Verify dynamic manifest loader and registry wiring in main.go
	requiredWiring := []string{
		"manifest.NewLoader",
		"registry.NewRegistry",
		"LoadDirectory",
		"RegisterBatch",
	}
	for _, req := range requiredWiring {
		if !strings.Contains(content, req) {
			t.Errorf("MISSING DYNAMIC BOOTSTRAPPING WIRING in %s: expected call to %q", mainPath, req)
		}
	}
}
