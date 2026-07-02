-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
  LOOP
    EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS document_doc_number_key ON %I.document (doc_number) WHERE doc_number IS NOT NULL', s);
    EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS document_source_url_key ON %I.document (source_url) WHERE source_url IS NOT NULL', s);
    EXECUTE format('ALTER TABLE %I.section ADD COLUMN IF NOT EXISTS body_tsv tsvector GENERATED ALWAYS AS (to_tsvector(''simple'', body)) STORED', s);
    EXECUTE format('ALTER TABLE %I.section ADD COLUMN IF NOT EXISTS position integer NOT NULL DEFAULT 0', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS section_body_tsv_idx ON %I.section USING gin (body_tsv)', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS section_document_id_idx ON %I.section (document_id)', s);
    BEGIN
      EXECUTE format('CREATE INDEX IF NOT EXISTS section_embedding_ann_idx ON %I.section USING scann (embedding cosine)', s);
    EXCEPTION WHEN OTHERS THEN
      EXECUTE format('CREATE INDEX IF NOT EXISTS section_embedding_ann_idx ON %I.section USING hnsw (embedding vector_cosine_ops)', s);
    END;
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.section_embedding_ann_idx', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.section_body_tsv_idx', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.section_document_id_idx', s);
    EXECUTE format('ALTER TABLE %I.section DROP COLUMN IF EXISTS body_tsv', s);
    EXECUTE format('ALTER TABLE %I.section DROP COLUMN IF EXISTS position', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.document_doc_number_key', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.document_source_url_key', s);
  END LOOP;
END $$;
-- +goose StatementEnd
