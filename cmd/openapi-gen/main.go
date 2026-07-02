// Command openapi-gen generates api/openapi.yaml from the same huma
// operations cmd/serving mounts at /api/v1 (pkg/httpapi.Register) — the
// provider side of API-CONTRACT.md §5's "one schema, generated from Go"
// decision. It never opens a database: pkg/httpapi.GenerateSpec's repo
// argument is only ever invoked per HTTP request, and OpenAPI generation
// only inspects the Input/Output Go types via reflection (huma.Register).
//
// Run via `task contract:gen` (Taskfile.yml) or `go run ./cmd/openapi-gen`.
// The anti-drift gate — pkg/httpapi/openapi_drift_test.go plus the
// `git diff --exit-code` step in .github/workflows/go.yml — fails if the
// committed api/openapi.yaml ever falls out of sync with this output.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"danny.vn/mise/pkg/config"
	"danny.vn/mise/pkg/httpapi"
)

func main() {
	if err := run(); err != nil {
		slog.Error("openapi-gen failed", "error", err)
		os.Exit(1)
	}
}

// run generates the spec and writes it to <repoRoot>/api/openapi.yaml.
func run() error {
	spec, err := httpapi.GenerateSpec(config.Role())
	if err != nil {
		return err
	}

	out, err := outputPath()
	if err != nil {
		return err
	}

	// out is derived from this source file's own path via runtime.Caller,
	// never request/user input.
	//nolint:gosec
	if err := os.WriteFile(out, spec, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}
	slog.Info("wrote openapi spec", "path", out)
	return nil
}

// outputPath resolves <repoRoot>/api/openapi.yaml from this source file's
// own path via runtime.Caller (mirrors internal/testdb.migrationsDir), so
// `go run ./cmd/openapi-gen` writes the same file regardless of the
// invoking working directory.
func outputPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolving openapi-gen main.go source path via runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "api", "openapi.yaml"), nil
}
