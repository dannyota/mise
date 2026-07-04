<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# First corpus

Start with **public law** — no internal documents, no credentials, no security sign-offs.
The Vietnam and Malaysia regulation corpora ship **built in**: their source crawlers are
compiled into the worker and need zero configuration.

## Built-in sources

Sources are wired per corpus at build time (`pkg/config` · `NewSources`) — you don't
configure them, you just trigger ingest:

| Corpus   | Source    | What it crawls                                               |
| -------- | --------- | ------------------------------------------------------------ |
| `vn-reg` | `vbpl`    | vbpl.vn — MoJ national legal database (SBV + related bodies) |
| `vn-reg` | `vanban`  | vanban.chinhphu.vn — government document portal              |
| `vn-reg` | `congbao` | congbao.chinhphu.vn — the official gazette                   |
| `vn-reg` | `sbv`     | State Bank of Vietnam portal (Hà Nội)                        |
| `my-reg` | `agclom`  | lom.agc.gov.my — Laws of Malaysia (Acts + P.U. gazette)      |
| `my-reg` | `bnm`     | bnm.gov.my — Bank Negara policy documents & guidelines       |
| `my-reg` | `sc`      | sc.com.my — Securities Commission Malaysia                   |

The corpus registry itself is compile-time (`pkg/corpus`); the REST surface exposes it
**read-only** for introspection:

```bash
curl http://localhost:8080/api/v1/registry          # all corpus descriptors
curl http://localhost:8080/api/v1/registry/vn-reg   # one descriptor
```

## 1. Trigger ingest

Ingest runs as a Temporal workflow — start it with the `temporal` CLI:

```bash
temporal workflow start --task-queue mise-ingest --type IngestCorpusWorkflow \
  --input '{"Corpus":"vn-reg"}'
```

Options in the input JSON:

- `Since` — RFC 3339 timestamp; overrides each source's stored discovery watermark
  (operator backfill). Zero value = incremental from the stored cursor.
- `Keyword` — per-source discovery query term; sources filtered server-side by keyword
  bypass the scope matcher.

The workflow runs **Discover → Fetch → Parse → Normalize → Embed → Index** with bounded
parallelism; one failed document counts in the run's `Failed` tally without failing the run.

> With `VERTEX=fake` (the local default) the crawlers still hit the **live public sources**
> — only the embed/parse seams are faked — so expect real documents with deterministic
> token-bag embeddings.

## 2. Watch it run

```bash
# Temporal UI (local stack)
open http://localhost:8088

# Or query the ingest ledger directly
psql -c "SELECT * FROM ingest.run ORDER BY started_at DESC LIMIT 5"
psql -c "SELECT status, count(*) FROM ingest.doc_ledger GROUP BY status"
```

## 3. Verify search

Search is an **MCP tool** on the serving process (mounted at `/mcp`), used by the reasoning
agent and any MCP client:

```bash
# Any MCP client works; with the MCP inspector:
npx @modelcontextprotocol/inspector http://localhost:8080/mcp
# → call the `search` tool: {"query": "IT system safety requirements", "corpora": ["vn-reg"]}
```

Expected: ranked hits with verbatim Vietnamese law text, citation paths (Điều/Khoản/Điểm),
validity status, and source URLs.

## 4. Run the eval harness

```bash
go run ./cmd/eval \
  -golden eval/golden-vn.json \
  -corpora vn-reg \
  -role mise_public \
  -min-recall 0 -min-mrr 0
```

The first run sets the baseline — gates at `0` log metrics without failing. Defaults gate at
recall@k ≥ 0.90 and MRR@k ≥ 0.85.

## Malaysia

Identical — the sources are already wired:

```bash
temporal workflow start --task-queue mise-ingest --type IngestCorpusWorkflow \
  --input '{"Corpus":"my-reg"}'
```

## What's next

- [Graph & detectors](graph-detectors.md) — internal corpora and the compliance graph.
- [DATA-MODEL §1](/design/DATA-MODEL.md) — corpus schema reference.
- [LOCAL-DEV](/engineering/LOCAL-DEV.md) — the full local walkthrough.
