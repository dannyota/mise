package httpapi_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"

	"danny.vn/mise/pkg/httpapi"
)

// testRole is a placeholder RLS role for spec generation only — GenerateSpec
// never inspects it (see its doc comment), so any valid-looking string works
// here; the drift test doesn't need pkg/config to resolve a real one.
const testRole = "mise_public"

// TestOpenAPISpecMatchesCommitted is the anti-drift gate (M2-14): it
// regenerates the OpenAPI spec in-process — via the exact same
// httpapi.GenerateSpec cmd/openapi-gen/main.go calls — and diffs it against
// the committed api/openapi.yaml. A developer who changes a Register
// operation or an Input/Output struct without re-running
// `task contract:gen` (or `go run ./cmd/openapi-gen`) fails this test
// locally, before ever reaching CI's separate `git diff --exit-code` step
// (.github/workflows/go.yml) — the two checks are deliberately redundant:
// this one is fast feedback inside `go test ./...`; CI's step is the final
// backstop against a stale commit slipping through.
func TestOpenAPISpecMatchesCommitted(t *testing.T) {
	got, err := httpapi.GenerateSpec(testRole)
	if err != nil {
		t.Fatalf("generating openapi spec: %v", err)
	}

	want, err := os.ReadFile(committedSpecPath(t))
	if err != nil {
		t.Fatalf("reading committed api/openapi.yaml: %v", err)
	}

	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("api/openapi.yaml is stale relative to the Go source — regenerate with "+
			"`task contract:gen` (or `go run ./cmd/openapi-gen`) and commit the result (-want +got):\n%s", diff)
	}
}

// committedSpecPath resolves <repoRoot>/api/openapi.yaml from this source
// file's own path via runtime.Caller (mirrors internal/testdb.migrationsDir
// and pkg/mcp/contract_test.go's schemaPath), so the test works regardless
// of the invoking working directory.
func committedSpecPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolving openapi_drift_test.go source path via runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "api", "openapi.yaml")
}
