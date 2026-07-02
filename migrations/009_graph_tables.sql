-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION graph.corpus_tier(cid text) RETURNS text IMMUTABLE LANGUAGE sql AS $$
  SELECT CASE cid
    WHEN 'vn-reg' THEN 'public' WHEN 'my-reg' THEN 'public'
    WHEN 'group-std' THEN 'group-confidential'
    WHEN 'local-policy' THEN 'local-confidential' WHEN 'local-sop' THEN 'local-confidential'
    ELSE 'local-confidential' END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION graph.tier_rank(t text) RETURNS int IMMUTABLE LANGUAGE sql AS $$
  SELECT CASE t WHEN 'public' THEN 0 WHEN 'group-confidential' THEN 1 ELSE 2 END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION graph.stricter_tier(a text, b text) RETURNS text IMMUTABLE LANGUAGE sql AS $$
  SELECT CASE WHEN graph.tier_rank(a) >= graph.tier_rank(b) THEN a ELSE b END $$;
-- +goose StatementEnd

CREATE TABLE graph.doc_ref (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  corpus_id text NOT NULL,
  ref_key text NOT NULL UNIQUE,
  document_id uuid,
  section_id uuid,
  label text NOT NULL DEFAULT '',
  src_ref jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX doc_ref_unresolved_idx ON graph.doc_ref (ref_key) WHERE document_id IS NULL;

CREATE TABLE graph.relation_edge (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  from_corpus_id text NOT NULL,
  from_document_id uuid NOT NULL,
  from_section_id uuid,
  to_ref_id uuid NOT NULL REFERENCES graph.doc_ref(id) ON DELETE CASCADE,
  to_corpus_id text NOT NULL,
  edge_type text NOT NULL CHECK (edge_type IN ('satisfies','implements','derives','covers')),
  direction text NOT NULL DEFAULT 'up',
  promoted boolean NOT NULL DEFAULT false,
  access_tier text GENERATED ALWAYS AS (graph.stricter_tier(graph.corpus_tier(from_corpus_id), graph.corpus_tier(to_corpus_id))) STORED,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT relation_edge_uq UNIQUE (from_corpus_id, from_document_id, to_ref_id, edge_type)
);
CREATE INDEX relation_edge_from_idx ON graph.relation_edge (from_corpus_id, from_document_id);
CREATE INDEX relation_edge_to_idx ON graph.relation_edge (to_ref_id);

CREATE TABLE graph.relation_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  edge_id uuid NOT NULL REFERENCES graph.relation_edge(id) ON DELETE CASCADE,
  evidence_kind text NOT NULL CHECK (evidence_kind IN ('extracted','model_classification','human_attested')),
  confidence double precision NOT NULL DEFAULT 0,
  grounding_score double precision NOT NULL DEFAULT 0,
  rationale text NOT NULL DEFAULT '',
  quoted_from_span text NOT NULL DEFAULT '',
  quoted_to_span text NOT NULL DEFAULT '',
  run_id uuid,
  model text NOT NULL DEFAULT '',
  prompt_hash text NOT NULL DEFAULT '',
  created_by text NOT NULL DEFAULT '',
  promoted_by text NOT NULL DEFAULT '',
  promoted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT relation_evidence_uq UNIQUE (edge_id, evidence_kind)
);
CREATE INDEX relation_evidence_edge_idx ON graph.relation_evidence (edge_id);

-- +goose Down
DROP TABLE IF EXISTS graph.relation_evidence;
DROP TABLE IF EXISTS graph.relation_edge;
DROP TABLE IF EXISTS graph.doc_ref;
DROP FUNCTION IF EXISTS graph.stricter_tier(text,text);
DROP FUNCTION IF EXISTS graph.tier_rank(text);
DROP FUNCTION IF EXISTS graph.corpus_tier(text);
