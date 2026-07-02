//go:build integration

package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
)

// TestNewRouterHealthzOnlyWithoutAlloyDBHost pins wireEvidence's
// zero-dependency path end to end through newRouter: without ALLOYDB_HOST,
// serving stays healthz-only — no pool, no /readyz route.
func TestNewRouterHealthzOnlyWithoutAlloyDBHost(t *testing.T) {
	t.Setenv("ALLOYDB_HOST", "")

	r, pool, err := newRouter(context.Background(), slog.Default())
	if err != nil {
		t.Fatalf("newRouter() error = %v, want nil", err)
	}
	if pool != nil {
		t.Error("newRouter() pool != nil, want nil when ALLOYDB_HOST is unset")
	}

	srv := httptest.NewServer(r)
	defer srv.Close()

	assertStatus(t, srv.URL+"/healthz", http.StatusOK)
	assertStatus(t, srv.URL+"/readyz", http.StatusNotFound)
}

// TestNewRouterWiresEvidenceWithAlloyDBHost points ALLOYDB_HOST/PORT/USER/
// PASSWORD/DATABASE at the real testdb container and asserts the other half
// of wireEvidence: a live pool, a working /readyz, plus the MCP evidence and
// graph tools actually registered — reached over the real streamable-HTTP
// mount, not just inferred from the pool being non-nil (T13 Important:
// wireEvidence was untested).
func TestNewRouterWiresEvidenceWithAlloyDBHost(t *testing.T) {
	dbPool := testdb.New(t)
	setTestdbEnv(t, dbPool)

	r, pool, err := newRouter(context.Background(), slog.Default())
	if err != nil {
		t.Fatalf("newRouter() error = %v, want nil", err)
	}
	if pool == nil {
		t.Fatal("newRouter() pool = nil, want a live pool when ALLOYDB_HOST is set")
	}
	defer pool.Close()

	srv := httptest.NewServer(r)
	defer srv.Close()

	assertStatus(t, srv.URL+"/healthz", http.StatusOK)
	assertStatus(t, srv.URL+"/readyz", http.StatusOK)
	assertMCPToolsRegistered(t, srv.URL, []string{"document", "graph", "search"})
}

// setTestdbEnv points the ALLOYDB_* env vars config.DB/wireEvidence read at
// the same container pool already connects to, so wireEvidence's real-pool
// path connects to it too.
func setTestdbEnv(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	cfg := pool.Config().ConnConfig
	t.Setenv("ALLOYDB_HOST", cfg.Host)
	t.Setenv("ALLOYDB_PORT", strconv.Itoa(int(cfg.Port)))
	t.Setenv("ALLOYDB_USER", cfg.User)
	t.Setenv("ALLOYDB_PASSWORD", cfg.Password)
	t.Setenv("ALLOYDB_DATABASE", cfg.Database)
}

func assertStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test helper; url is a local httptest.Server address
	if err != nil {
		t.Fatalf("GET %s error = %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		t.Errorf("GET %s status = %d, want %d", url, resp.StatusCode, want)
	}
}

// assertMCPToolsRegistered connects a real MCP client to the streamable
// HTTP endpoint mounted at baseURL+"/mcp" and asserts it advertises exactly
// want (sorted).
func assertMCPToolsRegistered(t *testing.T, baseURL string, want []string) {
	t.Helper()
	ctx := context.Background()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "wire-test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: baseURL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connecting mcp client: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	got := make([]string, len(res.Tools))
	for i, tool := range res.Tools {
		got[i] = tool.Name
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("registered tools = %v, want %v", got, want)
	}
}
