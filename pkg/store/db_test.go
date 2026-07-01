package store_test

import (
	"testing"

	"danny.vn/mise/pkg/store"
)

func TestConfigDSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  store.Config
		want string
	}{
		{
			name: "basic",
			cfg: store.Config{
				Host:     "localhost",
				Port:     5432,
				User:     "mise",
				Password: "secret",
				Database: "mise",
			},
			want: "host=localhost port=5432 user=mise password=secret dbname=mise sslmode=disable",
		},
		{
			name: "custom port and database",
			cfg: store.Config{
				Host:     "alloydb.internal",
				Port:     5433,
				User:     "mise_local",
				Password: "hunter2",
				Database: "mise_reference",
			},
			want: "host=alloydb.internal port=5433 user=mise_local password=hunter2 " +
				"dbname=mise_reference sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.DSN(); got != tt.want {
				t.Errorf("DSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestConnect is an integration test placeholder. Full testcontainers wiring
// (starting an AlloyDB Omni container and running goose migrations) is
// deferred; until then, the migrations are verified manually via podman
// compose (Task 8).
func TestConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	t.Log("integration tests require a running AlloyDB — skipping in unit mode")
	t.Skip("requires testcontainers setup — wired in Task 4 integration PR")
}
