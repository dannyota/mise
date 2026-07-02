-- +goose Up
CREATE SCHEMA IF NOT EXISTS ingest;

CREATE TABLE ingest.run (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  corpus_id text NOT NULL,
  started_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  status text NOT NULL DEFAULT 'running',
  stats jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE ingest.cursor (
  corpus_id text NOT NULL,
  source_id text NOT NULL,
  keyword text NOT NULL DEFAULT '',
  watermark timestamptz NOT NULL DEFAULT 'epoch',
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (corpus_id, source_id, keyword)
);

CREATE TABLE ingest.doc_ledger (
  corpus_id text NOT NULL,
  source_id text NOT NULL,
  external_id text NOT NULL,
  content_hash text NOT NULL DEFAULT '',
  document_id uuid,
  state text NOT NULL DEFAULT 'discovered',
  last_error text,
  observed_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (corpus_id, source_id, external_id)
);

-- +goose Down
DROP SCHEMA IF EXISTS ingest CASCADE;
