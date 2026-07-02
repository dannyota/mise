-- +goose Up

-- amendment_event rows record a dated act on a target document but not what
-- kind of act it was (amended/superseded/repealed) — pkg/ingest.RelationEvent
-- already classifies every event into exactly that at normalize time
-- (EventKind), but store.AmendmentEvent had nowhere to keep it. Without it,
-- re-deriving a target document's validity_status from its stored events
-- (the fix for a re-indexed target regressing past an applied amendment) is
-- impossible after the fact. Default 'amended' backfills existing rows: every
-- row inserted so far came from applyRelations, whose RelationEvent.Kind is
-- always a real, non-empty classification, and "amended" is the most common
-- of the three.
-- +goose StatementBegin
DO $$
DECLARE s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
  LOOP
    EXECUTE format('ALTER TABLE %I.amendment_event ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT ''amended''', s);
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
    EXECUTE format('ALTER TABLE %I.amendment_event DROP COLUMN IF EXISTS kind', s);
  END LOOP;
END $$;
-- +goose StatementEnd
