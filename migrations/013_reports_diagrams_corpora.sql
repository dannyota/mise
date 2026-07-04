-- +goose Up

-- M6 registered the reports and diagrams corpora in pkg/corpus (SchemaName
-- "reports"/"diagrams") but no migration ever created their schemas — every
-- store.Search fan-out that reached them failed with "relation
-- reports.section does not exist". This migration brings both schemas to the
-- same cumulative shape migrations 001–008 built for the original five, and
-- adds the section.image_ref column the M6 multimodal work introduced on the
-- Go side (store.Section/Hit.ImageRef) to EVERY corpus schema — the shared
-- read/write/search SQL uses one column list across all schemas.

CREATE SCHEMA IF NOT EXISTS reports;
CREATE SCHEMA IF NOT EXISTS diagrams;

-- Tables: the 002 shape with the later per-schema additions baked in —
-- body_tsv/position (006), amendment_event.kind (008) — since a fresh schema
-- can start at the final shape directly.
-- +goose StatementBegin
DO $$
DECLARE
  s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['reports','diagrams'])
  LOOP
    EXECUTE format('
      CREATE TABLE IF NOT EXISTS %I.document (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        corpus_id text NOT NULL,
        title text NOT NULL,
        doc_number text,
        citation_scheme text,
        citation_path text,
        language text NOT NULL DEFAULT ''en'',
        validity_status text NOT NULL DEFAULT ''in_force'',
        issued_date timestamptz,
        effective_date timestamptz,
        expiry_date timestamptz,
        issuing_authority text,
        signer_name text,
        signer_role text,
        owner_department text,
        owner_role text,
        version text,
        supersedes_id uuid,
        superseded_by_id uuid,
        source_url text,
        source_system text,
        content_type text,
        ingest_run_id uuid,
        observed_at timestamptz,
        access_tier text NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now()
      )', s);

    EXECUTE format('
      CREATE TABLE IF NOT EXISTS %I.section (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        document_id uuid NOT NULL REFERENCES %I.document(id),
        corpus_id text NOT NULL,
        citation_path text,
        heading_path text,
        position integer NOT NULL DEFAULT 0,
        body text NOT NULL,
        body_tsv tsvector GENERATED ALWAYS AS (to_tsvector(''simple'', body)) STORED,
        embedding vector(1536),
        validity_status text NOT NULL DEFAULT ''in_force'',
        effective_date timestamptz,
        access_tier text NOT NULL,
        image_ref text,
        created_at timestamptz NOT NULL DEFAULT now()
      )', s, s);

    EXECUTE format('
      CREATE TABLE IF NOT EXISTS %I.amendment_event (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        target_doc_id uuid NOT NULL REFERENCES %I.document(id),
        amending_doc_id uuid,
        clause text,
        event_date timestamptz NOT NULL,
        kind text NOT NULL DEFAULT ''amended'',
        created_at timestamptz NOT NULL DEFAULT now()
      )', s, s);

    -- 006's write keys and search indexes.
    EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS document_doc_number_key ON %I.document (doc_number) WHERE doc_number IS NOT NULL', s);
    EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS document_source_url_key ON %I.document (source_url) WHERE source_url IS NOT NULL', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS section_body_tsv_idx ON %I.section USING gin (body_tsv)', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS section_document_id_idx ON %I.section (document_id)', s);
    BEGIN
      EXECUTE format('CREATE INDEX IF NOT EXISTS section_embedding_ann_idx ON %I.section USING scann (embedding cosine)', s);
    EXCEPTION WHEN OTHERS THEN
      EXECUTE format('CREATE INDEX IF NOT EXISTS section_embedding_ann_idx ON %I.section USING hnsw (embedding vector_cosine_ops)', s);
    END;

    -- 007's amendment_event dedup key.
    EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS amendment_event_dedup_key ON %I.amendment_event (target_doc_id, COALESCE(amending_doc_id,''00000000-0000-0000-0000-000000000000''::uuid), COALESCE(clause,''''), event_date)', s);

    -- 004's grants + RLS. Both corpora are local-confidential
    -- (pkg/corpus: TierLocalConfidential), so they follow the
    -- local_policy/local_sop pattern exactly: table SELECT for every role
    -- (USAGE is the schema gate), USAGE for mise_local only.
    EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA %I TO mise_public, mise_group, mise_local', s);
    EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT SELECT ON TABLES TO mise_public, mise_group, mise_local', s);
    EXECUTE format('GRANT USAGE ON SCHEMA %I TO mise_local', s);
    EXECUTE format('ALTER TABLE %I.document ENABLE ROW LEVEL SECURITY', s);
    EXECUTE format('ALTER TABLE %I.section ENABLE ROW LEVEL SECURITY', s);
    EXECUTE format('ALTER TABLE %I.amendment_event ENABLE ROW LEVEL SECURITY', s);
    EXECUTE format('CREATE POLICY local_read ON %I.document FOR SELECT TO mise_local USING (true)', s);
    EXECUTE format('CREATE POLICY local_read ON %I.section FOR SELECT TO mise_local USING (true)', s);
    EXECUTE format('CREATE POLICY local_read ON %I.amendment_event FOR SELECT TO mise_local USING (true)', s);
  END LOOP;
END $$;
-- +goose StatementEnd

-- image_ref for the original five schemas, so the shared column list in
-- store's read/write/search SQL is valid everywhere. NULL for corpora that
-- never carry figures — only the diagrams ingest path sets it.
-- +goose StatementBegin
DO $$
DECLARE
  s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
  LOOP
    EXECUTE format('ALTER TABLE %I.section ADD COLUMN IF NOT EXISTS image_ref text', s);
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DO $$
DECLARE
  s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
  LOOP
    EXECUTE format('ALTER TABLE %I.section DROP COLUMN IF EXISTS image_ref', s);
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['reports','diagrams'])
  LOOP
    EXECUTE format('DROP POLICY IF EXISTS local_read ON %I.amendment_event', s);
    EXECUTE format('DROP POLICY IF EXISTS local_read ON %I.section', s);
    EXECUTE format('DROP POLICY IF EXISTS local_read ON %I.document', s);
    EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA %I FROM mise_public, mise_group, mise_local', s);
    EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I REVOKE SELECT ON TABLES FROM mise_public, mise_group, mise_local', s);
    EXECUTE format('REVOKE USAGE ON SCHEMA %I FROM mise_local', s);
  END LOOP;
END $$;
-- +goose StatementEnd

DROP SCHEMA IF EXISTS diagrams CASCADE;
DROP SCHEMA IF EXISTS reports CASCADE;
