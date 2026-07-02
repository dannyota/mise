-- +goose Up

-- org holds the durable ownership resolver (DATA-MODEL §2). A document's
-- (owner_department, owner_role) is a stable organizational anchor that
-- never changes; the person holding that role does (leaver/mover). org_role
-- resolves the anchor to today's holder, org_role_history to the holder as
-- of any past date — so a leaver/mover is one update to these two tables
-- instead of re-stamping every document/attestation that references the
-- anchor.
CREATE SCHEMA IF NOT EXISTS org;

CREATE TABLE org.org_role (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  department text NOT NULL,
  role text NOT NULL,
  current_holder text NOT NULL DEFAULT '',
  holder_email text NOT NULL DEFAULT '',
  holder_since timestamptz,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','vacant')),
  CONSTRAINT org_role_uq UNIQUE (department, role)
);

-- Append-only — "who held (department, role) on date D?" A leaver/mover
-- closes the open row (to_date = the transition instant) and inserts a new
-- one rather than updating in place, so ResolveAsOf can still answer for any
-- date before the transition.
CREATE TABLE org.org_role_history (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  department text NOT NULL,
  role text NOT NULL,
  holder text NOT NULL,
  from_date timestamptz NOT NULL,
  to_date timestamptz
);
CREATE INDEX org_role_history_lookup_idx ON org.org_role_history (department, role, from_date);

GRANT USAGE ON SCHEMA org TO mise_public, mise_group, mise_local;

-- +goose Down
DROP SCHEMA IF EXISTS org CASCADE;
