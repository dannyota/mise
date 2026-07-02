-- +goose Up

-- A changed document re-indexed by the ingest pipeline re-derives its
-- relation events from the source's current Relations and re-inserts them
-- (pkg/pipeline's applyRelations has no read-before-write check). Without a
-- natural-key constraint, that duplicates amendment_event rows on every
-- genuine re-index of a document whose relations haven't actually changed.
-- This unique index gives store.InsertAmendmentEvents' ON CONFLICT DO
-- NOTHING a target: an event is the same fact if it names the same target
-- and amending document, the same clause, and the same date.
-- amending_doc_id/clause are nullable, so a plain UNIQUE index would let
-- multiple NULLs coexist (SQL NULL <> NULL) — coalesce them to fixed
-- sentinels so two unattributed/clause-less events on the same target/date
-- also dedup.
-- +goose StatementBegin
DO $$
DECLARE s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
  LOOP
    EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS amendment_event_dedup_key ON %I.amendment_event (target_doc_id, COALESCE(amending_doc_id,''00000000-0000-0000-0000-000000000000''::uuid), COALESCE(clause,''''), event_date)', s);
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
    EXECUTE format('DROP INDEX IF EXISTS %I.amendment_event_dedup_key', s);
  END LOOP;
END $$;
-- +goose StatementEnd
