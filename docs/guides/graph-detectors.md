<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Graph & detectors

Once public law corpora are ingested, add internal control documents to build the compliance
graph. The detectors run automatically and propose mappings for human review.

## Add internal corpora

Register internal corpora (Group standards, Local policies, SOPs) with their access tiers:

```bash
# Group standards (visible to the whole group)
curl -X POST http://localhost:8080/api/v1/registry \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "group-std",
    "kind": "standard",
    "source_plugin": "sharepoint_crawl",
    "access_tier": "group-confidential",
    "tier": "group",
    "jurisdiction": "my",
    "graph_role": { "can_source": true, "can_target": true, "default_edges": ["satisfies","implements"] }
  }'
```

Ingest runs the same medallion pipeline. Internal docs additionally extract **doc-control
headers** (signer, version, effective date) and resolve ownership to a durable `org_role`.

## How the graph builds

Edges are created by two mechanisms:

| Method                   | Edge types              | Source                        | Confidence                 |
| ------------------------ | ----------------------- | ----------------------------- | -------------------------- |
| **Method A — extracted** | `implements`, `derives` | doc-control header (explicit) | 1.0 (human-authored)       |
| **Method B — proposed**  | `satisfies`, `covers`   | AI judge + grounding check    | variable (threshold-gated) |

Method A edges appear immediately on ingest. Method B runs as a Temporal workflow:

```
chunk → embed (FACT_VERIFICATION) → ScaNN ANN → Ranking API rerank
  → Gemini judge (edge_type, confidence, spans) → Check Grounding
  → write relation_edge (promoted=false) → review queue
```

## The 4 detectors

| Detector           | Raises                      | Severity | Trigger                          |
| ------------------ | --------------------------- | -------- | -------------------------------- |
| **RelationDetect** | candidate `satisfies` edges | —        | new/changed control chunk        |
| **ConflictDetect** | `conflict` findings         | critical | Group-std vs law contradiction   |
| **GapScan**        | `gap` findings              | medium   | obligation with 0 coverage       |
| **StaleScan**      | `staleness` findings        | high     | law amendment outdating controls |

All findings enter the **review queue** with `promoted=false`. Nothing auto-promotes.

## Review & promote

Use the Review Workbench (Web UI) or the REST API:

```bash
# List pending candidates
curl http://localhost:8080/api/v1/reviews?status=pending

# Promote a candidate edge (human attestation)
curl -X POST http://localhost:8080/api/v1/reviews/<edge-id>/promote

# Reject a candidate
curl -X POST http://localhost:8080/api/v1/reviews/<edge-id>/reject

# Relink (correct the target)
curl -X POST http://localhost:8080/api/v1/reviews/<edge-id>/relink \
  -d '{"new_target": {"corpus_id": "my-reg", "document_id": "...", "section_id": "..."}}'
```

## Resolve findings

Findings have four dispositions:

- **Map** — coverage exists, just unlinked; promote/relink an edge to close.
- **Document** — the paperwork is incomplete; update the policy/SOP and re-ingest.
- **Accept** — risk-accept the gap with a rationale.
- **Escalate** — a real control gap; track owner + due date.

Closure is **evidence-verified** — the finding closes when re-detection confirms it's gone
(except `accept`, which closes on rationale).

## What's next

- [Audit Q&A](audit-qa.md) — ask questions over the evidence.
- [DATA-MODEL §4–§7](/design/DATA-MODEL.md) — graph schema reference.
