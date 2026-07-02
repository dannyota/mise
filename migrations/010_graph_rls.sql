-- +goose Up

-- Confidentiality gate for the graph join surface (RISKS R2): the graph
-- schema has USAGE granted to all three roles (migrations/004), but the three
-- graph tables (migrations/009) had no SELECT grant — so reads failed closed
-- (42501) until now. This migration grants SELECT and, in the same step,
-- adds RLS so a role reads only edges/evidence/doc_refs at or below its tier.
-- Mirrors 004's exact three-policy shape (public/group/local), keyed on
-- relation_edge's GENERATED access_tier column instead of a stored one.

GRANT SELECT ON graph.relation_edge, graph.relation_evidence, graph.doc_ref
  TO mise_public, mise_group, mise_local;

ALTER TABLE graph.relation_edge ENABLE ROW LEVEL SECURITY;
ALTER TABLE graph.relation_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE graph.doc_ref ENABLE ROW LEVEL SECURITY;

-- relation_edge: access_tier is the GENERATED stricter-of-two-corpora tier
-- (migrations/009_graph_tables.sql) — the same three-policy shape as 004's
-- document tables, keyed on that column instead of a stored one.
CREATE POLICY public_read ON graph.relation_edge FOR SELECT TO mise_public, mise_group, mise_local
  USING (access_tier = 'public');
CREATE POLICY group_read ON graph.relation_edge FOR SELECT TO mise_group, mise_local
  USING (access_tier IN ('public', 'group-confidential'));
CREATE POLICY local_read ON graph.relation_edge FOR SELECT TO mise_local
  USING (true);

-- relation_evidence has no tier column of its own — it inherits its parent
-- edge's, via the same EXISTS-join-to-parent shape 004 uses for
-- amendment_event -> document. Evidence is visible iff its edge is.
CREATE POLICY public_read ON graph.relation_evidence FOR SELECT TO mise_public, mise_group, mise_local
  USING (EXISTS (
    SELECT 1 FROM graph.relation_edge e
    WHERE e.id = relation_evidence.edge_id AND e.access_tier = 'public'
  ));
CREATE POLICY group_read ON graph.relation_evidence FOR SELECT TO mise_group, mise_local
  USING (EXISTS (
    SELECT 1 FROM graph.relation_edge e
    WHERE e.id = relation_evidence.edge_id AND e.access_tier IN ('public', 'group-confidential')
  ));
CREATE POLICY local_read ON graph.relation_evidence FOR SELECT TO mise_local
  USING (true);

-- doc_ref's own corpus_id is its tier anchor (graph.corpus_tier, migrations/
-- 009_graph_tables.sql) — a doc_ref is readable iff its own corpus's tier is
-- readable by the role, independent of which edges (if any) reference it.
CREATE POLICY public_read ON graph.doc_ref FOR SELECT TO mise_public, mise_group, mise_local
  USING (graph.corpus_tier(corpus_id) = 'public');
CREATE POLICY group_read ON graph.doc_ref FOR SELECT TO mise_group, mise_local
  USING (graph.tier_rank(graph.corpus_tier(corpus_id)) <= 1);
CREATE POLICY local_read ON graph.doc_ref FOR SELECT TO mise_local
  USING (true);

-- +goose Down

DROP POLICY IF EXISTS public_read ON graph.relation_edge;
DROP POLICY IF EXISTS group_read ON graph.relation_edge;
DROP POLICY IF EXISTS local_read ON graph.relation_edge;
DROP POLICY IF EXISTS public_read ON graph.relation_evidence;
DROP POLICY IF EXISTS group_read ON graph.relation_evidence;
DROP POLICY IF EXISTS local_read ON graph.relation_evidence;
DROP POLICY IF EXISTS public_read ON graph.doc_ref;
DROP POLICY IF EXISTS group_read ON graph.doc_ref;
DROP POLICY IF EXISTS local_read ON graph.doc_ref;

ALTER TABLE graph.relation_edge DISABLE ROW LEVEL SECURITY;
ALTER TABLE graph.relation_evidence DISABLE ROW LEVEL SECURITY;
ALTER TABLE graph.doc_ref DISABLE ROW LEVEL SECURITY;

REVOKE SELECT ON graph.relation_edge, graph.relation_evidence, graph.doc_ref
  FROM mise_public, mise_group, mise_local;
