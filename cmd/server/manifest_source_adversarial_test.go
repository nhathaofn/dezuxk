package main_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestAdversarial_AST_DehardcodingRobustness challenges the static AST checker
// against evasion patterns (aliases, new(), renaming, commented markers).
func TestAdversarial_AST_DehardcodingRobustness(t *testing.T) {
	// The checker logic from manifest_source_test.go:
	inspectAST := func(src string) (hasSeedFunc bool, hasProviderLit bool) {
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
		if err != nil {
			return false, false
		}

		for _, decl := range node.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Name.Name == "seedDefaultProviders" {
					hasSeedFunc = true
				}
			}
		}

		ast.Inspect(node, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if sel, ok := lit.Type.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if ident.Name == "providers" && sel.Sel.Name == "Provider" {
						hasProviderLit = true
					}
				}
			}
			return true
		})

		return hasSeedFunc, hasProviderLit
	}

	t.Run("Catches_Original_SeedFunction", func(t *testing.T) {
		src := `package main
		func seedDefaultProviders() {}
		`
		hasSeed, _ := inspectAST(src)
		if !hasSeed {
			t.Error("expected AST to detect seedDefaultProviders function")
		}
	})

	t.Run("Catches_Original_CompositeLit", func(t *testing.T) {
		src := `package main
		func init() {
			p := &providers.Provider{ID: "flow"}
		}
		`
		_, hasLit := inspectAST(src)
		if !hasLit {
			t.Error("expected AST to detect providers.Provider composite literal")
		}
	})

	t.Run("Evasion_Renamed_Function", func(t *testing.T) {
		src := `package main
		func initDefaultProviders() {} // renamed from seedDefaultProviders!
		`
		hasSeed, _ := inspectAST(src)
		// Check that renaming bypasses the specific function name check
		if hasSeed {
			t.Error("did not expect match on renamed function")
		}
		t.Log("Observation: AST checker only matches exact function name 'seedDefaultProviders'. A renamed function like 'initDefaultProviders' is not flagged by the function name check alone.")
	})

	t.Run("Evasion_Aliased_Import", func(t *testing.T) {
		src := `package main
		import p "dezuxk/internal/domain/providers"
		func init() {
			prov := &p.Provider{ID: "flow"}
		}
		`
		_, hasLit := inspectAST(src)
		// Check whether aliased import evades the ast.SelectorExpr check
		if !hasLit {
			t.Log("Observation: AST checker checks ident.Name == 'providers'. Aliased package import 'p.Provider' evades the composite literal detector.")
		}
	})

	t.Run("Evasion_New_CallExpr", func(t *testing.T) {
		src := `package main
		func init() {
			p := new(providers.Provider)
			p.ID = "flow"
		}
		`
		_, hasLit := inspectAST(src)
		if !hasLit {
			t.Log("Observation: AST checker checks *ast.CompositeLit. Allocation via new(providers.Provider) is a *ast.CallExpr and is not flagged by the composite literal check.")
		}
	})

	t.Run("Defense_In_Depth_Lexical_Check", func(t *testing.T) {
		// Verify that lexical check acts as defense-in-depth against hardcoded values
		forbidden := []string{"flow.google.com", "Google Flow", "BatchGenerateImages", "FlowService", "[data-email]"}
		srcWithURL := `package main
		func init() {
			url := "https://flow.google.com"
		}
		`
		foundForbidden := false
		for _, f := range forbidden {
			if strings.Contains(srcWithURL, f) {
				foundForbidden = true
				break
			}
		}
		if !foundForbidden {
			t.Error("expected lexical check to catch hardcoded flow.google.com")
		}
	})
}
