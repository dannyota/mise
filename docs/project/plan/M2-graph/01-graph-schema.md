<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M2 · Workstream 1: Graph schema

Fill the empty `graph` schema (M0-10) with the edge model — `relation_edge` + `relation_evidence`,
the node-ref type and the `doc_ref` stub for out-of-corpus targets — and enforce the
confidentiality-critical **tier-inheritance rule (stricter of the two endpoints) at the database**,
with RLS on the join surface. The Heavy-review heart of M2; the tier + RLS work is split into its
own focused PRs. Part of [Milestone M2](./README.md).

See also:

- [M2 overview](./README.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §4
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §4
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2
- [TESTING](../../../engineering/TESTING.md) §2
- [DECISIONS](../../DECISIONS.md) 6
- Next workstream: [Extraction (Method A)](./02-extraction-method-a.md)

---

## 1. Tasks

Each row = one reviewable PR. New = net-new to mise; Reuse = adapt the banhmi engine's intra-corpus
relation model. The tier-inheritance (M2-3) and graph-RLS (M2-4) rules are split out so the
confidentiality boundary gets its own focused review ([RISKS](../../RISKS.md) R2).

| ID   | Task                                                                                                                                                                                                                                                           | Deliverable / done-when                                                                                                                | Design ref                           | Depends on | Size | Review | Risk |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ---------- | ---- | ------ | ---- |
| M2-1 | **`relation_edge` + node-ref + `doc_ref` stub (new/reuse):** migrate `relation_edge` (`from_node, to_node, edge_type, direction, promoted, access_tier`); node = `(corpus_id, document_id, section_id)`; out-of-corpus targets via the engine's `doc_ref` stub | `graph` schema gains `relation_edge`; node-ref + `doc_ref` types resolve; migration idempotent + re-runnable (testcontainers)          | DATA-MODEL §4 · ARCH §4 · DEC 21/22  | M0-10      | M    | Heavy  | Med  |
| M2-2 | **`relation_evidence` table (new):** migrate `relation_evidence` (`edge_id, evidence_kind, confidence, grounding_score, rationale, quoted_*_span, run_id, model, prompt_hash, created_by, promoted_by, promoted_at`); FK to `relation_edge`                    | `relation_evidence` exists with the full audit-trail columns; FK + indexes; migration still idempotent                                 | DATA-MODEL §4                        | M2-1       | S    | Medium | Low  |
| M2-3 | **Tier-inheritance rule — stricter-of-two, DB-enforced (new):** `relation_edge.access_tier` is a GENERATED column derived from the two endpoints (DEC 20), not app-supplied; defines the tier ordering public < group-confidential < local-confidential        | inserting an edge across two tiers sets `access_tier` = the stricter; app-supplied value is ignored/overridden; integration test green | DATA-GOV §2 · DATA-MODEL §4 · DEC 20 | M2-1       | M    | Heavy  | High |
| M2-4 | **Graph RLS policies (new):** RLS on `relation_edge` + `relation_evidence` keyed off `access_tier`, extending the M0/M1 per-tier roles; a join cannot surface an edge above the caller's clearance                                                             | RLS policies attach to both graph tables; a cross-tier-deny test (low-tier caller, confidential edge) is green on real AlloyDB Omni    | DATA-GOV §2 · TESTING §2             | M2-3, M2-2 | M    | Heavy  | High |

---

## 2. Notes

- **The graph is the only place corpora join (ARCHITECTURE §4), so tier inheritance is the
  confidentiality-critical rule** — enforce **stricter-of-two at the DB** (M2-3), not in the app,
  so no code path can forge a looser tier. A `local-policy → my-reg` edge reads as
  local-confidential (DATA-GOVERNANCE §2). This is the milestone's headline risk
  ([RISKS](../../RISKS.md) R2) — High risk on M2-3, the rule itself.
- **Why M2-3 / M2-4 are split:** the tier-derivation rule and the RLS policies that depend on it
  review independently; the policy PR (M2-4) gets a focused review and the **mandatory
  cross-tier-deny test** on real AlloyDB — a mis-set policy is the start of a tier leak. Don't
  claim isolation until that test is green.
- **`relation_evidence` carries the full audit trail now (M2-2)** even though Method A only writes
  the `extracted` kind this milestone — the `model`/`prompt_hash`/`grounding_score`/`promoted_*`
  columns exist so M3's judge loop and human promotion attach to the same row without a migration
  (DATA-MODEL §4).
- **`doc_ref` stub** (M2-1) lets an edge target a node not yet in any corpus (e.g. a cited law
  section not yet ingested) without a dangling FK — reuse the engine's existing mechanism rather
  than inventing one.
