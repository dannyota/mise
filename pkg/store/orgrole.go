package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrgRole is the durable ownership resolver (DATA-MODEL §2): a document's
// (owner_department, owner_role) is a stable organizational anchor that
// never changes, but the person holding that role does (leaver/mover).
// OrgRole resolves the anchor to a holder — Resolve for today, ResolveAsOf
// for a past date — by reading org.org_role / org.org_role_history
// (migrations/011_org_role.sql). A leaver/mover is a single write against
// those two tables (close the open org_role_history row, update
// org_role.current_holder); every document and attestation that references
// the (department, role) anchor is untouched.
type OrgRole struct {
	pool *pgxpool.Pool
}

// NewOrgRole returns an OrgRole backed by pool.
func NewOrgRole(pool *pgxpool.Pool) *OrgRole {
	return &OrgRole{pool: pool}
}

// Resolve returns the person currently holding (dept, role) — org_role's
// current_holder — and whether an active assignment exists. A (dept, role)
// with no org_role row, or one marked status='vacant' (the seat exists but
// is unfilled), reports found=false: only an active row resolves to a
// holder.
func (o *OrgRole) Resolve(ctx context.Context, dept, role string) (holder string, found bool, err error) {
	const q = `
SELECT current_holder FROM org.org_role
WHERE department = $1 AND role = $2 AND status = 'active'`

	err = o.pool.QueryRow(ctx, q, dept, role).Scan(&holder)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("resolving org role %s/%s: %w", dept, role, err)
	}
	return holder, true, nil
}

// ResolveAsOf returns whoever held (dept, role) at time at, read from
// org_role_history: the row whose [from_date, to_date) interval covers at —
// an open row (to_date IS NULL) extends to the present. It answers as-of-
// date questions a leaver/mover must never disturb, e.g. "who owned POL-001
// when this conflict was raised?" (DATA-MODEL §2). found is false when no
// history row covers at — before the role's earliest recorded holder, or a
// gap the data doesn't account for.
func (o *OrgRole) ResolveAsOf(
	ctx context.Context, dept, role string, at time.Time,
) (holder string, found bool, err error) {
	const q = `
SELECT holder FROM org.org_role_history
WHERE department = $1 AND role = $2 AND from_date <= $3 AND (to_date IS NULL OR to_date > $3)
ORDER BY from_date DESC
LIMIT 1`

	err = o.pool.QueryRow(ctx, q, dept, role, at).Scan(&holder)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("resolving org role %s/%s as of %s: %w", dept, role, at, err)
	}
	return holder, true, nil
}
