//go:build integration

// Package testdb is the integration-test database harness shared by every
// `_test.go` file built with the "integration" tag. New starts a
// Postgres-compatible container, runs the goose migrations from
// migrations/ at the repo root, and caches the resulting *pgxpool.Pool as a
// package-level singleton so the whole test binary reuses one container.
//
// The container image defaults to google/alloydbomni:latest; override it
// with TESTDB_IMAGE (CI passes pgvector/pgvector:pg17 — smaller, no ScaNN).
// The container's user/password/database are always "mise".
//
// Under Podman, point testcontainers at the rootless API socket and disable
// its Ryuk reaper (Ryuk generally can't run rootless):
//
//	systemctl --user start podman.socket
//	export DOCKER_HOST=unix:///run/user/1000/podman/podman.sock
//	export TESTCONTAINERS_RYUK_DISABLED=true
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver used to run migrations
	"github.com/moby/moby/api/types/network"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	defaultImage = "google/alloydbomni:latest"
	dbUser       = "mise"
	dbPassword   = "mise"
	dbName       = "mise"
	pgPort       = "5432/tcp"
)

var (
	once     sync.Once
	pool     *pgxpool.Pool
	setupErr error
)

// New returns the shared, goose-migrated *pgxpool.Pool for the current test
// binary. The backing container starts on the first call and is reused by
// every subsequent test (sync.Once), so parallel tests never race on setup.
// It fails the test via t.Fatalf if the container or migrations can't start.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	once.Do(func() {
		pool, setupErr = setup(context.Background())
	})
	if setupErr != nil {
		t.Fatalf("testdb: setup failed: %v", setupErr)
	}
	return pool
}

// setup starts the container, waits for it to accept SQL connections, runs
// the goose migrations, and opens the pool tests will share.
func setup(ctx context.Context) (*pgxpool.Pool, error) {
	image := os.Getenv("TESTDB_IMAGE")
	if image == "" {
		image = defaultImage
	}

	container, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts(pgPort),
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_USER":     dbUser,
			"POSTGRES_PASSWORD": dbPassword,
			"POSTGRES_DB":       dbName,
		}),
		testcontainers.WithWaitStrategy(
			wait.ForSQL(pgPort, "pgx", dsn).WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("starting testdb container %s: %w", image, err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving testdb container host: %w", err)
	}
	port, err := container.MappedPort(ctx, pgPort)
	if err != nil {
		return nil, fmt.Errorf("resolving testdb container port: %w", err)
	}
	url := dsn(host, port)

	if err := migrate(ctx, url); err != nil {
		return nil, err
	}

	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("opening testdb pool: %w", err)
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, fmt.Errorf("pinging testdb pool: %w", err)
	}
	return p, nil
}

// dsn builds the connection string for the container's fixed mise/mise/mise
// credentials. It doubles as the wait.ForSQL URL builder (host/port are only
// known once the container has started and its port is mapped).
func dsn(host string, port network.Port) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPassword, host, port.Port(), dbName)
}

// migrate runs every migration under <repoRoot>/migrations against url.
func migrate(ctx context.Context, url string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("opening migration connection: %w", err)
	}
	defer func() { _ = db.Close() }()

	dir, err := migrationsDir()
	if err != nil {
		return err
	}

	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("running migrations from %s: %w", dir, err)
	}
	return nil
}

// migrationsDir resolves <repoRoot>/migrations from this source file's own
// path via runtime.Caller, so it works regardless of which package's tests
// invoke New.
func migrationsDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolving testdb.go source path via runtime.Caller")
	}
	// file is <repoRoot>/internal/testdb/testdb.go.
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "migrations"), nil
}
