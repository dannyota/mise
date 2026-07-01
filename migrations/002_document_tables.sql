-- +goose Up

-- pgvector backs the `section.embedding` column below; a hard requirement —
-- fail the migration if it cannot be installed.
CREATE EXTENSION IF NOT EXISTS vector;

-- AlloyDB Omni's ScaNN index type is optional: available on AlloyDB Omni, not
-- on a plain Postgres+pgvector dev box. Swallow the error (via the implicit
-- savepoint an EXCEPTION block gives a DO statement) so the rest of this
-- migration still applies outside AlloyDB Omni.
-- +goose StatementBegin
DO $$
BEGIN
  CREATE EXTENSION IF NOT EXISTS alloydb_scann;
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'alloydb_scann extension not available, skipping (expected outside AlloyDB Omni): %', SQLERRM;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
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
        body text NOT NULL,
        embedding vector(1536),
        validity_status text NOT NULL DEFAULT ''in_force'',
        effective_date timestamptz,
        access_tier text NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now()
      )', s, s);

    EXECUTE format('
      CREATE TABLE IF NOT EXISTS %I.amendment_event (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        target_doc_id uuid NOT NULL REFERENCES %I.document(id),
        amending_doc_id uuid,
        clause text,
        event_date timestamptz NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now()
      )', s, s);
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
    EXECUTE format('DROP TABLE IF EXISTS %I.amendment_event CASCADE', s);
    EXECUTE format('DROP TABLE IF EXISTS %I.section CASCADE', s);
    EXECUTE format('DROP TABLE IF EXISTS %I.document CASCADE', s);
  END LOOP;
END $$;
-- +goose StatementEnd

-- Extensions (vector, alloydb_scann) are shared cluster resources installed
-- once for the database; they are intentionally not dropped here.
