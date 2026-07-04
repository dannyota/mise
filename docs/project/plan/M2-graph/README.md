<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Milestone M2: Graph spine

Build the **cross-corpus compliance graph** — the only place the five evidence-only corpora
join. Stand up the `graph` schema (`relation_edge` + `relation_evidence`, nodes as corpus refs,
out-of-corpus targets via the `doc_ref` stub), have **Normalize** emit explicit internal
`implements` / `derives` edges from the doc-control header **deterministically, with no model
call** (Method A), enforce **tier inheritance — the stricter of the two endpoints — at the DB**,
and ship the read-side graph API (the MCP `graph` tool + `GET /graph/nodes/{ref}` +
`/graph/chain/{ref}`) returning tier-filtered nodes, edges, and the control **chain**
(SOP→Policy→Group). It does **not** run the law-facing `satisfies` judge loop, build detectors or
findings, or promote any edge — those are M3.

A **detailed, PR-sized milestone**, split across the three workstream files below; every milestone
(M0–M8) is planned to this depth — see the [build-plan workspace](../README.md).

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [RISKS](../../RISKS.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §4/§5/§2
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §4
- [API-CONTRACT](../../../design/API-CONTRACT.md) §2/§3/§5
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2
- [TESTING](../../../engineering/TESTING.md) §2/§4
- [DECISIONS](../../DECISIONS.md) 6
- Previous: [Milestone M1](../M1-ingest/README.md)
- Next: [Milestone M3](../M3-detectors/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** the compliance graph exists and is queryable. The `graph` schema is migrated;
  Normalize extracts explicit internal edges deterministically (Method A, **no model call**);
  every edge carries `access_tier = stricter-of-two-endpoints`, DB-enforced; the graph API
  returns tier-filtered nodes/edges/chain — reaching **M2 — the graph spine**.
- **Implements (design):** [DATA-MODEL](../../../design/DATA-MODEL.md) §4 (nodes · edges ·
  evidence) / §5 (Method A — extracted, no judge) / §2 (doc-control header · `org_role`) ·
  [ARCHITECTURE](../../../design/ARCHITECTURE.md) §4 ·
  [API-CONTRACT](../../../design/API-CONTRACT.md) §2/§3/§5 ·
  [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2 · [DECISIONS](../../DECISIONS.md) 6.
- **Exit criteria (DoD):**
  - `migrate` applies the `graph` schema — `relation_edge` + `relation_evidence` — with the
    node-ref model (`corpus_id, document_id, section_id`) and the `doc_ref` stub for
    out-of-corpus targets; idempotent + re-runnable (testcontainers; DATA-MODEL §4).
  - **Normalize emits `implements` / `derives` edges** from the parsed doc-control header
    (`local-sop → local-policy`, `local-policy → group-std`), `evidence_kind = extracted`,
    `promoted = false`, **no model call** — Method A; re-running Normalize on unchanged input
    produces no duplicate edges (idempotent; DATA-MODEL §5).
  - An **absent / unparsable** doc-control header yields **no edge** (never a guess) — asserted
    by a unit test ([RISKS](../../RISKS.md) R6, DATA-MODEL §2).
  - **Every edge's `access_tier` is the stricter of its two endpoints, set by the DB** (trigger
    / generated, not app-supplied); a cross-tier-deny **RLS test on real AlloyDB Omni is green**
    — a low-tier caller cannot read a confidential edge through the join (DATA-GOVERNANCE §2,
    TESTING §2).
  - Extracted-edge `relation_evidence` records `evidence_kind = extracted` with the
    `quoted_from_span` / `quoted_to_span` from the header and the durable attestation owner
    resolved via `org_role` (DATA-MODEL §2/§4).
  - The MCP `graph` tool + `GET /graph/nodes/{ref}` + `GET /graph/chain/{ref}` return
    tier-filtered nodes, edges, and the control **chain** (SOP→Policy→Group, verbatim text +
    citation per hop), under the caller's RLS role; the **contract test is green** against the
    Go-generated OpenAPI + JSON Schema (API-CONTRACT §5, TESTING §4).
  - The control-chain walk is **bounded** (max depth + cycle guard); a malformed/cyclic graph
    terminates without runaway.
- **Out of scope:** the law-facing `satisfies` judge loop (Method B — propose→verify→promote) →
  M3; detectors (conflict · gap · staleness) + findings → M3; edge **promotion** / the review
  workbench write path → M3/M4; the reasoning endpoint's chain composition → M4; the Graph
  Explorer UI → M5; registry GA → M6.

---

## 2. Dependencies & gates

- **Upstream — M1** provides: normalized internal docs + the **metadata envelope**, the
  **parsed doc-control header** Method A extracts edges from (DATA-MODEL §2), the `org_role`
  resolver + history (attestation ownership), per-corpus schemas, and the **per-tier RLS roles**
  the graph RLS policies extend. M0 provides the empty `graph` schema (M0-10) this milestone
  fills and the migration harness.
- **Decision/configuration gates — low exposure.** Extraction is **deterministic and calls no
  model**, so managed-AI policy does **not** gate this milestone's edge build — no confidential
  text reaches a Vertex model on the Method A path. The tier-inheritance and graph-RLS tasks
  (M2-3, M2-4, M2-15) are **Heavy-review**: they are the confidentiality boundary on the
  cross-corpus join surface ([RISKS](../../RISKS.md) R2). RLS correctness is a build concern,
  enforced and tested here.
- **Rationale for sequencing — DECISIONS 6:** seed the **internal extractable edges first**
  (SOP→Policy→Group, Method A), then the inferred law-facing `satisfies` (Method B, M3). The
  spine is built before the detectors so M3's `satisfies` and the transitive SOP→law `covers`
  edge (DECISIONS 7) have a graph to attach to.
- **External prerequisites:** none new — M2 runs entirely on the M0/M1 local stack (AlloyDB Omni
  on podman compose, `VERTEX=fake`); no Vertex/GCP call is needed to build or test the graph.

---

## 3. Workstreams

The M2 breakdown, three reviewable workstreams, dependency-ordered (IDs `M2-1 … M2-15`). The
tier-inheritance + RLS work (the confidentiality boundary) is split into its own focused PRs, and
the Method A extraction is split header-parse vs edge-write so each PR reviews in one sitting.
Each task: **Size** XS–L · **Review** Light/Medium/Heavy · **Risk** Low/Med/High.

| #   | Workstream                                           | Tasks         | Theme                                                        |
| --- | ---------------------------------------------------- | ------------- | ------------------------------------------------------------ |
| 1   | [Graph schema](./01-graph-schema.md)                 | M2-1 … M2-4   | `relation_edge`/`relation_evidence` · `doc_ref` · tier + RLS |
| 2   | [Extraction (Method A)](./02-extraction-method-a.md) | M2-5 … M2-9   | header parse → edge write · extracted evidence · `org_role`  |
| 3   | [Graph API](./03-graph-api.md)                       | M2-10 … M2-15 | read repo · control chain · MCP `graph` · REST `/graph/*`    |

---

## 4. Test & eval gates (this milestone)

- **Build/lint:** `go build`/`go test` green at Go 1.26; `golangci-lint run` (+ `modernize`)
  clean; the generated graph types regenerate clean and the contract test passes (TOOLCHAIN §4,
  TESTING §4).
- **Unit (offline = Mode B):** the **doc-control-header parser** (table-driven, including the
  **absent-header → no-edge** case — [RISKS](../../RISKS.md) R6); the **header → edge** mapping
  (corpus-pair → `edge_type`); **idempotent edge write** (re-run yields no duplicates); the
  **chain-walk** depth + cycle bound (TESTING §2). No Vertex call — Method A is deterministic.
- **Integration (testcontainers, AlloyDB Omni):** the `graph` migration is idempotent;
  tier-inheritance is **DB-set** (insert an edge across two tiers → `access_tier` = stricter);
  **mandatory cross-tier-deny RLS** — a low-tier caller cannot read a confidential edge or
  traverse it through a graph join (DATA-GOVERNANCE §2, TESTING §2; the milestone's headline
  gate, [RISKS](../../RISKS.md) R2).
- **Contract (the anti-drift gate):** `serving` responses for the MCP `graph` tool + `GET
/graph/nodes/{ref}` + `/graph/chain/{ref}` validate against the **Go-generated** JSON Schema /
  OpenAPI; a provider change that isn't regenerated fails the build (API-CONTRACT §5, TESTING §4,
  [RISKS](../../RISKS.md) R8).
- **Not yet:** the mapping-quality eval (no `satisfies` edges until M3) and the load/SLO test
  (TESTING §5/§6) — nothing cross-corpus to score until the judge loop lands.

---

## 5. Risks (this milestone)

- **RLS correctness on the join surface** (M2-3/M2-4/M2-15) — the graph is the **one place
  corpora join**, so one tier-inheritance or policy bug collapses isolation; enforce the
  stricter-of-two rule **at the DB** and gate with mandatory cross-tier-deny tests on real
  AlloyDB ([RISKS](../../RISKS.md) R2 — the headline; DATA-GOVERNANCE §2).
- **Doc-control header inconsistency** (M2-5/M2-6) — ragged internal templates yield missing or
  wrong edges; drive parsing off the per-source `metadata_config`, and treat an **absent header
  as "no edge", never a guess** ([RISKS](../../RISKS.md) R6, DATA-MODEL §2).
- **Contract drift** (M2-13/M2-14) — serving / reasoning / web diverge if the graph types are
  hand-mirrored; generate the one contract from Go and fail the build on un-regenerated drift
  ([RISKS](../../RISKS.md) R8, API-CONTRACT §5).
- **Chain-walk depth / cycle** (M2-11) — an unbounded or cyclic SOP→Policy→Group walk runs away;
  bound depth and guard cycles, asserted by a unit test.
- **Solo-reviewer bottleneck** (M2-3/M2-4/M2-15) — the Heavy-review tier/RLS tasks concentrate on
  one part-time human; they are split into one-PR-each slices so they don't bunch
  ([RISKS](../../RISKS.md) R4).
