package graph_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// bannedImportMarkers are substrings that must never appear in an import
// path pulled in by extract.go. Method A (all of pkg/graph) is deterministic
// extraction with no model/judge/network call (RISKS R6; M2-9, ARCH §4 ·
// DECISIONS 6) — confidential internal control text must never reach a
// model or the network on this path. "vertex"/"embed" catch the model
// clients (pkg/vertex, pkg/rag/embed); "net"/"http"/"cloud.google"/"pgx"
// catch the network- and DB-capable packages (net/http, the Google Cloud
// SDKs, pgx). Deliberately loose (substring, not exact-path) — the point is
// to over-flag rather than miss a disguised or renamed import.
var bannedImportMarkers = []string{
	"vertex",
	"embed",
	"net",
	"http",
	"cloud.google",
	"pgx",
}

// TestExtractGoImportsNoModelOrNetworkPackage is a belt-and-suspenders check
// alongside the "graph-pure" depguard rule in .golangci.yml: it re-derives
// extract.go's import list itself, straight from the AST, instead of
// trusting lint config — so it still catches a future edit that imports a
// model or network package even if that lint rule is ever loosened or
// removed. Parses only extract.go's own import declarations (not its
// transitive dependency graph), so it stays simple and hermetic: no
// go/packages loading, no module cache or network access. `go test` always
// runs with the package directory as its working directory, so the bare
// relative path resolves regardless of where `go test` itself was invoked
// from.
func TestExtractGoImportsNoModelOrNetworkPackage(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "extract.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parser.ParseFile(extract.go) = %v, want a parseable file", err)
	}

	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, marker := range bannedImportMarkers {
			if strings.Contains(path, marker) {
				t.Errorf("extract.go imports %q (matches banned marker %q) — "+
					"pkg/graph (Method A) must have no model or network import", path, marker)
				break
			}
		}
	}
}
