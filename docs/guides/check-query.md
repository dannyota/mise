<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Check & query

After an ingest run: verify what landed, then query it — as the right tier.

## 1. Did the run succeed?

```bash
# Temporal UI (local: http://localhost:8088) — workflow status, per-document activity retries

# Or ask the database directly
psql -c "SELECT corpus_id, status, started_at, discovered, indexed, failed
         FROM ingest.run ORDER BY started_at DESC LIMIT 5"
psql -c "SELECT corpus_id, state, count(*) FROM ingest.doc_ledger
         GROUP BY corpus_id, state ORDER BY corpus_id"
```

A healthy run: `indexed` ≈ `discovered`, `failed` = 0. One failed document never fails the
run — it shows up in `doc_ledger` as `failed` with its error, and the next run retries it.

## 2. What corpora exist?

```bash
curl http://localhost:8080/api/v1/registry | jq '.[].id'
curl http://localhost:8080/api/v1/registry/group-std | jq
```

The descriptor shows each corpus's jurisdiction, tier, and graph role — the
[source pairing](sources.md) reflected back.

## 3. Search the evidence (MCP)

Search and document reads are **MCP tools** on the serving process (`/mcp`) — the same
tools the reasoning agent uses. Quickest interactive client:

```bash
npx @modelcontextprotocol/inspector http://localhost:8080/mcp
```

Call `search`:

```json
{ "query": "patch management obligations", "corpora": ["vn-reg", "local-sop"], "top_k": 5 }
```

Optional: `"as_of_date": "2026-01-01"` (validity as of a date), `"in_force_only": false`
(include repealed/superseded — default is in-force only). Each hit returns verbatim text,
citation path, validity status, and source URL. Fetch a full document with the `document`
tool: `{ "corpus_id": "vn-reg", "document_id": "<uuid>" }`.

## 4. Tiers — who sees what

The serving process reads as one RLS role (`MISE_DB_ROLE`); rows outside that tier are
invisible **at the database**, not filtered in code:

| Role          | Sees                                       |
| ------------- | ------------------------------------------ |
| `mise_public` | `vn-reg`, `my-reg` only                    |
| `mise_group`  | law + `group-std`                          |
| `mise_local`  | everything (law + Group + Country corpora) |

Quick check that tiering works: run the same search as `mise_public` and `mise_local` —
internal hits must appear only in the second.

## 5. Walk the graph (REST)

```bash
# One node and its outgoing edges — ref is corpus_id/document_id[/section_id], / as %2F
curl "http://localhost:8080/api/v1/graph/nodes/local-sop%2F<doc-uuid>"

# The full compliance chain from a node (SOP → policy → standard → law)
curl "http://localhost:8080/api/v1/graph/chain/local-sop%2F<doc-uuid>"
```

Pending AI-proposed edges and findings:

```bash
curl "http://localhost:8080/api/v1/reviews?status=pending"
curl "http://localhost:8080/api/v1/findings?kind=gap"
```

## 6. Ask, don't query — Q&A and the Web UI

- **[Audit Q&A](audit-qa.md)** — natural-language questions with cited answers (SSE); the
  agent abstains when the evidence is insufficient.
- **[Web UI](web-ui.md)** — dashboards, graph explorer, review workbench, coverage report.

## 7. Gate the quality (eval)

```bash
go run ./cmd/eval -golden eval/golden-vn.json -corpora vn-reg -role mise_public
```

Gates recall@k / MRR@k / current-law / abstain / citation against a golden set — run it
after every significant ingest to catch retrieval regressions
([TESTING §4](/engineering/TESTING.md)).

## What's next

- [Graph & detectors](graph-detectors.md) — promote/reject the proposed mappings.
- [Operations](operations.md) — schedules, monitoring, backup.
