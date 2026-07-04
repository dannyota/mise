<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Configure sources

mise ships five core corpora in two families: **public law** (crawled, zero config) and
**internal controls** (drop folder, one env var). This page maps who is who and how each is
configured.

## The corpus model — Group vs Country

In the reference deployment the banking **group** operates under **Malaysian** regulation
and the **country subsidiary** under **Vietnamese** regulation. That pairing is wired into
the corpus registry — each internal corpus knows which law it must _satisfy_:

| Corpus         | What it holds               | Jurisdiction | Tier               | Maps to (satisfies)      |
| -------------- | --------------------------- | ------------ | ------------------ | ------------------------ |
| `my-reg`       | Malaysian regulation (law)  | MY           | public             | — (target only)          |
| `vn-reg`       | Vietnamese regulation (law) | VN           | public             | — (target only)          |
| `group-std`    | **Group** standards         | MY           | group-confidential | `my-reg`                 |
| `local-policy` | **Country** policies        | VN           | local-confidential | `vn-reg` + `group-std`   |
| `local-sop`    | **Country** SOPs / runbooks | VN           | local-confidential | derives < `local-policy` |

So: **Group source ⇒ `group-std` (MY side)** · **Country sources ⇒ `local-policy` +
`local-sop` (VN side)**. The chain the graph builds is
`local-sop → local-policy → group-std → my-reg` and `local-policy → vn-reg`.

> A different group/country pairing (or more countries) is an adopter deploy setting — the
> registry descriptor per corpus carries jurisdiction and mapping target
> ([DATA-MODEL §1](/design/DATA-MODEL.md)).

## Law sources (VN + MY) — nothing to configure

The crawlers are compiled in; each ingest run visits all of them for its corpus:

| Corpus   | Crawlers                                                            |
| -------- | ------------------------------------------------------------------- |
| `vn-reg` | vbpl.vn · vanban.chinhphu.vn · congbao.chinhphu.vn · SBV portal     |
| `my-reg` | lom.agc.gov.my (Acts + P.U.) · bnm.gov.my (policy docs) · sc.com.my |

Trigger one Temporal workflow per corpus (repeat weekly via a schedule —
[operations](operations.md)):

```bash
temporal workflow start --task-queue mise-ingest --type IngestCorpusWorkflow \
  --input '{"Corpus":"my-reg"}'
temporal workflow start --task-queue mise-ingest --type IngestCorpusWorkflow \
  --input '{"Corpus":"vn-reg"}'
```

Optional input fields: `"Since": "2025-01-01T00:00:00Z"` (backfill past the stored
watermark), `"Keyword": "cyber"` (server-side discovery filter).

## Group source (`group-std`)

1. Set `LIBRARY_ROOT` on the **worker** (e.g. `/data/library`, a mounted volume).
2. Drop Group standards under `$LIBRARY_ROOT/group-std/` — one file per document
   (`.pdf` / `.docx` / `.html` / `.md`).
3. Add a sidecar per document for doc-control metadata (optional but recommended —
   it powers explicit graph edges and ownership):

```
$LIBRARY_ROOT/group-std/GRP-STD-014 Information Security Standard.pdf
$LIBRARY_ROOT/group-std/GRP-STD-014 Information Security Standard.pdf.meta.json
```

```json
{
  "number": "GRP-STD-014",
  "title": "Group Information Security Standard",
  "doc_type": "standard",
  "language": "en",
  "signer_name": "…",
  "signer_role": "…",
  "owner_department": "Group CISO Office",
  "owner_role": "Head of Information Security",
  "version": "3.2",
  "issued_date": "2026-01-15",
  "effective_date": "2026-02-01"
}
```

4. Trigger: `--input '{"Corpus":"group-std"}'` (same workflow as law).

Files land **group-confidential** — visible to the `mise_group` and `mise_local` roles,
never to `mise_public`. Unknown sidecar keys are rejected at ingest (typo protection);
editing a file in place re-indexes it.

## Country sources (`local-policy`, `local-sop`)

Same mechanics, different folders and one extra sidecar field — `relations` declares the
explicit upward edge (Method A, confidence 1.0, no AI involved):

```
$LIBRARY_ROOT/local-policy/POL-IT-007 IT Risk Policy.docx
$LIBRARY_ROOT/local-policy/POL-IT-007 IT Risk Policy.docx.meta.json
$LIBRARY_ROOT/local-sop/SOP-IT-112 Patch Management.md
```

```json
{
  "number": "POL-IT-007",
  "title": "IT Risk Management Policy",
  "doc_type": "policy",
  "owner_department": "IT Risk",
  "relations": [{ "type": "implements", "target_number": "GRP-STD-014" }]
}
```

Trigger each: `--input '{"Corpus":"local-policy"}'` · `--input '{"Corpus":"local-sop"}'`.
Both land **local-confidential** (only `mise_local` sees them). The AI-side mapping —
`satisfies` candidates against `vn-reg` — is proposed by the detectors afterwards and waits
for human review ([graph & detectors](graph-detectors.md)).

## Order matters (once)

Ingest law **before** internal docs — the detectors map controls onto whatever law is
already indexed. The steady state is: law on a weekly schedule, internal folders daily or on
demand.

## What's next

- [Check & query](check-query.md) — verify what landed and search it per tier.
- [Graph & detectors](graph-detectors.md) — review AI-proposed mappings.
