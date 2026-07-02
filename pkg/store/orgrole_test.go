//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/store"
)

// insertOrgRole inserts an org.org_role row directly. store.OrgRole has no
// write method — Resolve/ResolveAsOf are the canonical, read-only API — so
// tests set up fixtures with raw SQL, the same pattern corpus_test.go's
// setSectionCreatedAt uses for state the store type doesn't expose.
func insertOrgRole(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, dept, role, holder, email, status string, since time.Time,
) {
	t.Helper()
	const q = `
INSERT INTO org.org_role (department, role, current_holder, holder_email, holder_since, status)
VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := pool.Exec(ctx, q, dept, role, holder, email, since, status); err != nil {
		t.Fatalf("inserting org_role %s/%s: %v", dept, role, err)
	}
}

// insertOrgRoleHistory inserts an open (to_date NULL) org_role_history row —
// the initial "who has held this role since from" fact a fresh role starts
// with.
func insertOrgRoleHistory(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, dept, role, holder string, from time.Time,
) {
	t.Helper()
	const q = `
INSERT INTO org.org_role_history (department, role, holder, from_date, to_date)
VALUES ($1, $2, $3, $4, NULL)`
	if _, err := pool.Exec(ctx, q, dept, role, holder, from); err != nil {
		t.Fatalf("inserting org_role_history %s/%s: %v", dept, role, err)
	}
}

// applyLeaver performs the DATA-MODEL §2 leaver/mover update — "one update":
// close the currently-open org_role_history row and start a new one for
// newHolder, then point org_role.current_holder at them. at is the
// transition instant: the closed row's to_date and the new row's from_date.
func applyLeaver(t *testing.T, ctx context.Context, pool *pgxpool.Pool, dept, role, newHolder string, at time.Time) {
	t.Helper()
	const closeQ = `
UPDATE org.org_role_history SET to_date = $3
WHERE department = $1 AND role = $2 AND to_date IS NULL`
	if _, err := pool.Exec(ctx, closeQ, dept, role, at); err != nil {
		t.Fatalf("closing org_role_history %s/%s: %v", dept, role, err)
	}
	insertOrgRoleHistory(t, ctx, pool, dept, role, newHolder, at)

	const updateQ = `
UPDATE org.org_role SET current_holder = $3, holder_since = $4
WHERE department = $1 AND role = $2`
	if _, err := pool.Exec(ctx, updateQ, dept, role, newHolder, at); err != nil {
		t.Fatalf("updating org_role current_holder %s/%s: %v", dept, role, err)
	}
}

func TestOrgRoleResolveReturnsCurrentHolder(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	orgRole := store.NewOrgRole(pool)

	dept := "legal-" + uuid.NewString()
	role := "head"
	since := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	insertOrgRole(t, ctx, pool, dept, role, "Alice", "alice@example.com", "active", since)
	insertOrgRoleHistory(t, ctx, pool, dept, role, "Alice", since)

	holder, found, err := orgRole.Resolve(ctx, dept, role)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !found {
		t.Fatal("Resolve() found = false, want true")
	}
	if holder != "Alice" {
		t.Errorf("Resolve() holder = %q, want %q", holder, "Alice")
	}
}

func TestOrgRoleResolveUnknownRoleNotFound(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	orgRole := store.NewOrgRole(pool)

	holder, found, err := orgRole.Resolve(ctx, "dept-"+uuid.NewString(), "nobody")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if found {
		t.Errorf("Resolve() found = true, want false for an unknown role")
	}
	if holder != "" {
		t.Errorf("Resolve() holder = %q, want empty for an unknown role", holder)
	}
}

// TestOrgRoleResolveVacantRoleNotFound pins Resolve's vacant-seat semantics:
// a row with status='vacant' still exists, but must not resolve to a
// holder — only an active assignment does.
func TestOrgRoleResolveVacantRoleNotFound(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	orgRole := store.NewOrgRole(pool)

	dept := "vacant-" + uuid.NewString()
	role := "head"
	insertOrgRole(t, ctx, pool, dept, role, "", "", "vacant", time.Now().UTC())

	holder, found, err := orgRole.Resolve(ctx, dept, role)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if found {
		t.Errorf("Resolve() found = true, want false for a vacant role (holder=%q)", holder)
	}
}

// TestOrgRoleLeaverUpdatesCurrentHolderPreservesAsOfHistory is the key case
// (DATA-MODEL §2): a leaver/mover is one update — close the open
// org_role_history row and set the new current_holder — and it must not
// disturb what ResolveAsOf reports for a date before the transition.
func TestOrgRoleLeaverUpdatesCurrentHolderPreservesAsOfHistory(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	orgRole := store.NewOrgRole(pool)

	dept := "legal-" + uuid.NewString()
	role := "head"
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	before := t0.AddDate(1, 0, 0) // still within [t0, t1)
	after := t1.AddDate(0, 1, 0)  // within [t1, ∞)

	insertOrgRole(t, ctx, pool, dept, role, "Alice", "alice@example.com", "active", t0)
	insertOrgRoleHistory(t, ctx, pool, dept, role, "Alice", t0)

	assertResolve(t, ctx, orgRole, dept, role, "Alice")
	assertResolveAsOf(t, ctx, orgRole, dept, role, before, "Alice")

	applyLeaver(t, ctx, pool, dept, role, "Bob", t1)

	assertResolve(t, ctx, orgRole, dept, role, "Bob")
	// The as-of-before-transition query is untouched by the leaver update.
	assertResolveAsOf(t, ctx, orgRole, dept, role, before, "Alice")
	assertResolveAsOf(t, ctx, orgRole, dept, role, after, "Bob")
}

// TestOrgRoleResolveAsOfBeforeFirstHistoryRowNotFound checks the edge before
// any history exists for the role: found is false, not an error or a
// zero-value holder mistaken for a real one.
func TestOrgRoleResolveAsOfBeforeFirstHistoryRowNotFound(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	orgRole := store.NewOrgRole(pool)

	dept := "legal-" + uuid.NewString()
	role := "head"
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	insertOrgRoleHistory(t, ctx, pool, dept, role, "Alice", from)

	holder, found, err := orgRole.ResolveAsOf(ctx, dept, role, from.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("ResolveAsOf() error = %v", err)
	}
	if found {
		t.Errorf("ResolveAsOf() before the first history row found = true (holder=%q), want false", holder)
	}
}

// assertResolve is TestOrgRoleLeaverUpdatesCurrentHolderPreservesAsOfHistory's
// Resolve() assertion, factored out to keep that test under the function-
// length/statement-count lint budget.
func assertResolve(t *testing.T, ctx context.Context, orgRole *store.OrgRole, dept, role, want string) {
	t.Helper()
	holder, found, err := orgRole.Resolve(ctx, dept, role)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !found || holder != want {
		t.Errorf("Resolve() = (%q, %v), want (%q, true)", holder, found, want)
	}
}

// assertResolveAsOf is TestOrgRoleLeaverUpdatesCurrentHolderPreservesAsOfHistory's
// ResolveAsOf() assertion — see assertResolve.
func assertResolveAsOf(
	t *testing.T, ctx context.Context, orgRole *store.OrgRole, dept, role string, at time.Time, want string,
) {
	t.Helper()
	holder, found, err := orgRole.ResolveAsOf(ctx, dept, role, at)
	if err != nil {
		t.Fatalf("ResolveAsOf(%s) error = %v", at, err)
	}
	if !found || holder != want {
		t.Errorf("ResolveAsOf(%s) = (%q, %v), want (%q, true)", at, holder, found, want)
	}
}
