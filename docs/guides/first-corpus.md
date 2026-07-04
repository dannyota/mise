<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# First corpus

Start with **public law** — no internal documents, no SharePoint credentials, no security
sign-offs needed. This guide ingests the Vietnamese banking regulation corpus (`vn-reg`) and
verifies search works.

## Prerequisites

- mise running locally (`podman compose up -d` + `go run ./cmd/serving`)
- Or a deployed instance with `VERTEX=real` for production-quality embeddings

## 1. Register the corpus

The corpus registry accepts a descriptor — no code change needed:

```bash
curl -X POST http://localhost:8080/api/v1/registry \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "vn-reg",
    "kind": "law",
    "source_plugin": "vbpl_crawler",
    "citation_scheme": "dieu_khoan_diem",
    "embed": { "model": "gemini-embedding-001", "dims": 1536, "task_type": "RETRIEVAL_DOCUMENT" },
    "access_tier": "public",
    "jurisdiction": "vn",
    "graph_role": { "can_source": false, "can_target": true, "default_edges": ["satisfies"] }
  }'
```

## 2. Trigger ingest

```bash
# Start the ingest workflow for vn-reg
curl -X POST http://localhost:8080/api/v1/ingest \
  -H 'Content-Type: application/json' \
  -d '{"corpus": "vn-reg"}'
```

The Temporal workflow runs: **Discover → Fetch → Parse → Normalize → Embed → Index**.

Monitor progress:

```bash
# Check workflow status
temporal workflow list --query 'WorkflowType="IngestWorkflow" AND ExecutionStatus="Running"'
```

## 3. Verify search

Once ingest completes, search the corpus:

```bash
# Search for IT system safety requirements
curl 'http://localhost:8080/api/v1/search?q=IT+system+safety+requirements&corpus=vn-reg'
```

Expected: ranked results with verbatim Vietnamese law text, citation references
(Điều/Khoản/Điểm), and relevance scores.

## 4. Run the eval harness

```bash
go run ./cmd/eval \
  -corpus vn-reg \
  -golden deploy/eval/golden-vn.json \
  -min-recall 0 \
  -min-mrr 0
```

The first run sets the baseline — metrics are logged but not gated (thresholds start at 0).

## Adding Malaysia regulation

Same pattern — register `my-reg` with `jurisdiction: "my"` and `source_plugin: "agc_crawler"`:

```bash
curl -X POST http://localhost:8080/api/v1/registry \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "my-reg",
    "kind": "law",
    "source_plugin": "agc_crawler",
    "citation_scheme": "part_section",
    "embed": { "model": "gemini-embedding-001", "dims": 1536, "task_type": "RETRIEVAL_DOCUMENT" },
    "access_tier": "public",
    "jurisdiction": "my",
    "graph_role": { "can_source": false, "can_target": true, "default_edges": ["satisfies"] }
  }'
```

## What's next

- [Graph & detectors](graph-detectors.md) — add internal docs and watch the compliance graph build.
- [DATA-MODEL §1](/design/DATA-MODEL.md) — full corpus schema reference.
