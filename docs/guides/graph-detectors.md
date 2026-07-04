<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Graph & detectors

Once public law corpora are ingested, internal control documents build the compliance graph.
The detectors propose mappings for human review.

## Internal corpora — the library connector

The three internal corpora ingest from a **document-library drop folder** (the `library`
source, M9). Set `LIBRARY_ROOT` on the worker and lay documents out one file per document:

```
$LIBRARY_ROOT/
  group-std/      Group standards          (tier: group-confidential)
  local-policy/   Local policies           (tier: local-confidential)
  local-sop/      SOPs and runbooks        (tier: local-confidential)
```

- **Formats:** `.pdf` / `.docx` / `.html` / `.md`. The directory a file sits in **defines its
  corpus and tier** — discovery logs every file with both, so misfiling is visible.
- **Metadata sidecar (optional):** `<file>.meta.json` next to the document carries
  doc-control metadata — `number`, `title`, `doc_type`, `language`, `signer_name`,
  `signer_role`, `owner_department`, `owner_role`, `version`, `issued_date`,
  `effective_date` (dates `YYYY-MM-DD`), and `relations` (`[{"type": "implements",
"target_number": "…"}]`). Unknown keys are **rejected** so typos surface at ingest.
  Without a sidecar, the filename becomes the title.
- **Change detection:** editing a file in place re-indexes it (content hash), and re-running
  ingest is idempotent.
- Trigger per corpus, same as public law:

```bash
temporal workflow start --task-queue mise-ingest --type IngestCorpusWorkflow \
  --input '{"Corpus":"group-std"}'
```

Locally, mount the folder into the worker container as a Podman volume; in the reference
deployment, sync from a bucket or mount a share. Alternatively, the **SharePoint web-crawl**
connector crawls a document library live — see
[Configure sources](sources.md#sharepoint-web-crawl-alternative-to-drop-folders). Both
sources can feed the same corpus.

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
