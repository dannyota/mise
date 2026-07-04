<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Milestone M9: Internal sources (library connector)

Make the three internal corpora (`group-std`, `local-policy`, `local-sop`) **ingestable** —
they have been fully provisioned (schemas, RLS, tiers, graph roles) since M1/M2 but have had
**no source connector** (the M1b SharePoint web-crawl was deferred for lack of a test site,
[M1 plan](../M1-ingest/README.md)). M9 ships the **document-library connector**: a
filesystem-directory `ingest.Source` (`pkg/ingest/library`) that reads a per-corpus drop
folder — one file per document (PDF/DOCX/HTML/Markdown) plus an optional `.meta.json`
sidecar carrying doc-control metadata. Local dev mounts it as a Podman volume; the reference
deployment syncs a bucket to the volume with standard tooling. The SharePoint web-crawl
remains deferred; when it lands it plugs the same `Source` seam.

Tasks are PR-sized and dependency-ordered (`M9-1 … M9-4`). Each task: **Size** XS–L ·
**Review** Light/Medium/Heavy · **Risk** Low/Med/High.

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [M1 plan](../M1-ingest/README.md) (the deferred WS2 this replaces-for-now)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §5
- [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§2
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2
- Previous: [Milestone M8](../M8-release/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** an operator drops files under `LIBRARY_ROOT/<corpus>/`, triggers
  `IngestCorpusWorkflow` for that corpus, and the documents land tier-stamped, RLS-gated,
  embedded, and searchable — proven by an integration test and documented in the
  [graph & detectors guide](../../../guides/graph-detectors.md).
- **In scope:** `pkg/ingest/library` source; sidecar metadata contract; `config.NewSources`
  wiring behind `LIBRARY_ROOT`; unit + integration tests; guide update.
- **Out of scope:** SharePoint/Graph API crawl (unchanged deferral); OCR of scanned files
  beyond the existing Doc AI seam; any new REST surface.

## 2. Work breakdown

| Task | Title                                                                                                                  | Size | Review | Risk |
| ---- | ---------------------------------------------------------------------------------------------------------------------- | ---- | ------ | ---- |
| M9-1 | `pkg/ingest/library`: Discover (dir walk, mtime watermark, sha change-detect) · FetchDetail (sidecar merge) · Download | M    | Medium | Low  |
| M9-2 | `config.NewSources`: wire `group-std`/`local-policy`/`local-sop` behind `LIBRARY_ROOT`                                 | XS   | Light  | Low  |
| M9-3 | Tests: unit (walk/watermark/sidecar/sha) + integration (drop file → search hit as `mise_local`)                        | S    | Medium | Low  |
| M9-4 | Guide + LOCAL-DEV: document the drop-folder layout and sidecar schema                                                  | XS   | Light  | Low  |

## 3. Sidecar contract (`<file>.meta.json`)

All fields optional; the filename (sans extension) is the fallback title:

```
{
  "number":         "GRP-STD-014",
  "title":          "Group Information Security Standard",
  "doc_type":       "standard",
  "language":       "en",
  "signer_name":    "…",
  "signer_role":    "…",
  "owner_department": "…",
  "owner_role":     "…",
  "version":        "3.2",
  "issued_date":    "2026-01-15",
  "effective_date": "2026-02-01",
  "relations":      [{ "type": "implements", "target_number": "…" }]
}
```

## 4. Gates & risks

- **Gates:** `golangci-lint` clean · unit + `-tags integration` green · guide lint-clean.
- **Risk — silent misfiling:** a file dropped under the wrong corpus dir inherits that
  corpus's tier. Mitigation: the guide states the layout is tier-defining; discovery logs
  every file with its resolved corpus and tier.
- **Risk — sidecar drift:** unknown sidecar keys are rejected (strict decode) so typos
  surface at ingest, not as silently-missing metadata.
