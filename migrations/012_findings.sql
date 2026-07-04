-- +goose Up

-- M3 findings tables: cross-corpus detector output (gap/conflict/staleness),
-- resolution workflow (finding_resolution + action_plan). finding.access_tier
-- is trigger-computed from node_refs[] — the stricter-of-all-referenced-corpora
-- tier — not a GENERATED column, because iterating a jsonb array in an
-- immutable expression isn't possible. RLS mirrors the 3-policy shape from
-- migrations/010_graph_rls.sql.

CREATE TABLE graph.finding (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kind text NOT NULL CHECK (kind IN ('gap','conflict','staleness')),
  severity text NOT NULL DEFAULT 'medium' CHECK (severity IN ('critical','high','medium','low','info')),
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','acknowledged','resolved','dismissed')),
  node_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
  evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
  access_tier text NOT NULL DEFAULT 'local-confidential',
  detected_at timestamptz NOT NULL DEFAULT now(),
  dedup_key text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE graph.finding_resolution (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  finding_id uuid NOT NULL REFERENCES graph.finding(id) ON DELETE CASCADE,
  disposition text NOT NULL CHECK (disposition IN ('map','document','accept','escalate')),
  owner_department text NOT NULL DEFAULT '',
  owner_role text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','in_progress','in_review','closed','dismissed')),
  rationale text NOT NULL DEFAULT '',
  due_date timestamptz,
  action_plan_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE graph.action_plan (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  scope text NOT NULL DEFAULT '',
  target_date timestamptz,
  owner text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

-- BEFORE INSERT trigger: compute finding.access_tier from node_refs[].
-- Parses the jsonb array, extracts each element's corpus_id, applies
-- graph.stricter_tier() across all of them. Empty node_refs → fail-closed
-- to 'local-confidential' (the column default).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION graph.set_finding_access_tier() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  elem jsonb;
  cid text;
  tier text := NULL;
BEGIN
  IF jsonb_array_length(NEW.node_refs) = 0 THEN
    NEW.access_tier := 'local-confidential';
    RETURN NEW;
  END IF;

  FOR elem IN SELECT jsonb_array_elements(NEW.node_refs) LOOP
    cid := elem ->> 'corpus_id';
    IF cid IS NULL THEN
      NEW.access_tier := 'local-confidential';
      RETURN NEW;
    END IF;
    IF tier IS NULL THEN
      tier := graph.corpus_tier(cid);
    ELSE
      tier := graph.stricter_tier(tier, graph.corpus_tier(cid));
    END IF;
  END LOOP;

  NEW.access_tier := tier;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER finding_set_access_tier BEFORE INSERT ON graph.finding
  FOR EACH ROW EXECUTE FUNCTION graph.set_finding_access_tier();

-- Grants: SELECT to all three RLS roles (owner writes bypass RLS).
GRANT SELECT ON graph.finding, graph.finding_resolution, graph.action_plan
  TO mise_public, mise_group, mise_local;

-- RLS on finding: keyed on access_tier, same 3-policy shape as relation_edge.
ALTER TABLE graph.finding ENABLE ROW LEVEL SECURITY;

CREATE POLICY public_read ON graph.finding FOR SELECT TO mise_public, mise_group, mise_local
  USING (access_tier = 'public');
CREATE POLICY group_read ON graph.finding FOR SELECT TO mise_group, mise_local
  USING (access_tier IN ('public', 'group-confidential'));
CREATE POLICY local_read ON graph.finding FOR SELECT TO mise_local
  USING (true);

-- RLS on finding_resolution: inherits parent finding's access_tier via EXISTS.
ALTER TABLE graph.finding_resolution ENABLE ROW LEVEL SECURITY;

CREATE POLICY public_read ON graph.finding_resolution FOR SELECT TO mise_public, mise_group, mise_local
  USING (EXISTS (
    SELECT 1 FROM graph.finding f
    WHERE f.id = finding_resolution.finding_id AND f.access_tier = 'public'
  ));
CREATE POLICY group_read ON graph.finding_resolution FOR SELECT TO mise_group, mise_local
  USING (EXISTS (
    SELECT 1 FROM graph.finding f
    WHERE f.id = finding_resolution.finding_id AND f.access_tier IN ('public', 'group-confidential')
  ));
CREATE POLICY local_read ON graph.finding_resolution FOR SELECT TO mise_local
  USING (true);

-- RLS on action_plan: no tier column of its own — open to all roles.
-- Action plans are organizational artifacts, not finding-scoped secrets.
ALTER TABLE graph.action_plan ENABLE ROW LEVEL SECURITY;

CREATE POLICY public_read ON graph.action_plan FOR SELECT TO mise_public, mise_group, mise_local
  USING (true);

-- +goose Down

DROP POLICY IF EXISTS public_read ON graph.action_plan;
ALTER TABLE graph.action_plan DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS public_read ON graph.finding_resolution;
DROP POLICY IF EXISTS group_read ON graph.finding_resolution;
DROP POLICY IF EXISTS local_read ON graph.finding_resolution;
ALTER TABLE graph.finding_resolution DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS public_read ON graph.finding;
DROP POLICY IF EXISTS group_read ON graph.finding;
DROP POLICY IF EXISTS local_read ON graph.finding;
ALTER TABLE graph.finding DISABLE ROW LEVEL SECURITY;

REVOKE SELECT ON graph.finding, graph.finding_resolution, graph.action_plan
  FROM mise_public, mise_group, mise_local;

DROP TRIGGER IF EXISTS finding_set_access_tier ON graph.finding;
DROP TABLE IF EXISTS graph.action_plan;
DROP TABLE IF EXISTS graph.finding_resolution;
DROP TABLE IF EXISTS graph.finding;
DROP FUNCTION IF EXISTS graph.set_finding_access_tier();
