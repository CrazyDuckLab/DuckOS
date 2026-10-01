// upnp/no_fixture_in_production_test.go: build-time guard
// against accidentally importing NftablesBackendFixture
// from production wiring.
//
// This test is in the upnp package (not the agent or
// cmd/duck-agent packages) so it compiles on the
// production host's toolchain and runs in the unit-test
// gate. If anyone re-introduces a reference to
// NftablesBackendFixture from non-test code, this test
// fails loudly.
//
// The walk covers cmd, internal/api, internal/agent,
// internal/engine and internal/upnp. The web/src tree is
// out of scope for this guard (web code does not import
// Go symbols).
package upnp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNftablesBackendFixtureOnlyReferencedFromTests asserts
// that the only file in the entire module that references
// NftablesBackendFixture is the file that defines it
// (nftables_linux_test.go). Production code must not import
// or instantiate the fixture.
func TestNftablesBackendFixtureOnlyReferencedFromTests(t *testing.T) {
	const selfPath = "nftables_linux_test.go"
	const fixtureIdent = "NftablesBackendFixture"
	roots := []string{
		"../../cmd",
		"../../internal/api",
		"../../internal/agent",
		"../../internal/engine",
		"../../internal/upnp",
	}
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if strings.HasSuffix(path, "/testdata") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			// Skip the file that defines the fixture itself.
			if strings.HasSuffix(path, selfPath) {
				return nil
			}
			// Skip the guard test itself.
			if strings.HasSuffix(path, "no_fixture_in_production_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(src), fixtureIdent) {
				t.Errorf("production code references %s: %s", fixtureIdent, path)
			}
			// Also fail if the file parses and references the
			// identifier in any AST node, even if the surrounding
			// string is a comment or a string literal.
			fset := token.NewFileSet()
			tree, perr := parser.ParseFile(fset, path, src, parser.AllErrors)
			if perr != nil {
				// Tolerate parse failures (eg generated files).
				return nil
			}
			ast.Inspect(tree, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok || ident.Name != fixtureIdent {
					return true
				}
				t.Errorf("production AST references %s: %s", fixtureIdent, path)
				return false
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}