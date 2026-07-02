// Package config provides typed, env-based configuration for all mise services.
package config

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
)

// defaultRole is Role's fallback — the least-privileged RLS role
// (migrations/004_rls_roles.sql).
const defaultRole = "mise_public"

// DB returns AlloyDB connection parameters from the ALLOYDB_* environment
// variables (.env.example), defaulting to the reference local-dev stack
// (compose.yaml). An unparseable ALLOYDB_PORT falls back to the default
// port rather than erroring — DB has no error return.
func DB() store.Config {
	return store.Config{
		Host:     envOr("ALLOYDB_HOST", "localhost"),
		Port:     envIntOr("ALLOYDB_PORT", 5432),
		User:     envOr("ALLOYDB_USER", "mise"),
		Password: envOr("ALLOYDB_PASSWORD", "mise-dev"),
		Database: envOr("ALLOYDB_DATABASE", "mise"),
	}
}

// NewEmbedder returns the Embedder VERTEX selects: "fake" (the default) for
// the offline deterministic embedder (LOCAL-DEV §4 Mode B), "real" for
// Vertex AI gemini-embedding-001 via GCP_PROJECT/GCP_REGION. Any other
// VERTEX value is an error.
func NewEmbedder(ctx context.Context) (embed.Embedder, error) {
	switch v := envOr("VERTEX", "fake"); v {
	case "fake":
		return embed.NewFake(), nil
	case "real":
		e, err := embed.NewVertex(ctx, os.Getenv("GCP_PROJECT"), envOr("GCP_REGION", "us-central1"))
		if err != nil {
			return nil, fmt.Errorf("config: creating vertex embedder: %w", err)
		}
		return e, nil
	default:
		return nil, fmt.Errorf("config: unknown VERTEX value %q, want \"fake\" or \"real\"", v)
	}
}

// Role returns the RLS role serving assumes for MCP evidence calls
// (MISE_DB_ROLE), defaulting to mise_public — never derived from request
// input (migrations/004_rls_roles.sql).
func Role() string {
	return envOr("MISE_DB_ROLE", defaultRole)
}

// envOr returns the environment variable named key, or fallback if unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envIntOr returns the environment variable named key parsed as an int, or
// fallback if key is unset, empty, or not a valid integer.
func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
