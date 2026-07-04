<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M3 · Workstream 1: Findings & candidate pipeline

The data foundation for M3: the findings/resolution schema + migration that the detectors write
into, and the Method-B candidate pipeline (embed→ScaNN ANN→Ranking rerank) that feeds the judge —
**no model judging yet**, just the retrieval shape that produces candidate pairs. Part of
[Milestone M3](./README.md).

See also:

- [M3 overview](./README.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §5/§6/§7
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§5
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §5
- [DECISIONS](../../DECISIONS.md) 1
- Next workstream: [Judge & ground](./02-judge-and-ground.md)

---

## 1. Tasks

Each row = one reviewable PR. The findings migration is split from RLS, and the candidate pipeline
from the rerank, so each PR reviews on its own. Reuse = adapt existing engine code; New = net-new.

| ID   | Task                                                                                                                                          | Deliverable / done-when                                                                                                            | Design ref                  | Depends on | Size | Review | Risk |
| ---- | --------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | --------------------------- | ---------- | ---- | ------ | ---- |
| M3-1 | **Findings schema migration (new):** `migrate` adds `finding` / `finding_resolution` / `action_plan` to the `graph` schema; idempotent        | tables exist with `kind`/`status`/`severity`/`node_refs[]`/`evidence`/`disposition`; re-run is a no-op (testcontainers)            | DATA-MODEL §6/§7            | M2         | M    | Heavy  | Med  |
| M3-2 | **Finding RLS + tier inheritance (new):** a finding inherits the **stricter** tier of its `node_refs[]`; RLS policy on the findings tables    | low-tier caller can't see a `group-std`-tier conflict finding; cross-tier-deny test green (real AlloyDB)                           | DATA-GOV §2 · DATA-MODEL §6 | M3-1       | S    | Heavy  | Med  |
| M3-3 | **Candidate-pair model + repo (new):** the in-flight candidate type (`from_node · to_node · target_corpus · pair_texts`) + a write/read repo  | candidate type defined; repo round-trips against AlloyDB; unit test green                                                          | DATA-MODEL §5 · ARCH §5     | M2         | S    | Medium | Low  |
| M3-4 | **Method-B retrieval: embed→ScaNN ANN (reuse→adapt):** embed a changed control chunk (`FACT_VERIFICATION`) + ANN vs the **target law corpus** | given a control chunk, returns top-N nearest law sections from the right jurisdiction corpus (Local→host, Group→home); Mode-B fake | DATA-MODEL §5 · DEC 1/27    | M3-3       | M    | Medium | Med  |
| M3-5 | **Ranking API rerank → top-k pairs (reuse→adapt):** rerank the ANN hits with the Ranking API; emit the top-k candidate pairs                  | top-k pairs emitted with rerank scores; offline path uses the fake rank seam; unit test on the merge/cutoff                        | DATA-MODEL §5 · ARCH §5     | M3-4       | S    | Medium | Med  |

---

## 2. Notes

- **Why split M3-1/M3-2:** the findings migration is one Heavy PR; the RLS/tier-inheritance on the
  new findings surface deserves its own focused review — a finding that spans a confidential node
  and surfaces to a low-tier caller is the start of a leak ([RISKS](../../RISKS.md) R2). The
  finding inherits the **stricter** tier of its `node_refs[]`, the same rule M2 set for edges
  (DATA-GOVERNANCE §2).
- **The candidate pipeline (M3-4/M3-5) calls no judge** — it is retrieval only (embed → ScaNN →
  rerank), so it runs fully on the Mode-B fake seams and carries **no confidentiality-gate
  exposure** by itself; the gate lands when the judge reads the pair texts (workstream 2). Keeping
  retrieval separate from judging keeps each PR's review surface clean.
- **Jurisdiction routing is load-bearing:** Method B embeds vs the **target** law corpus by the
  mapping rule (`local-* → host vn-reg`, `group-std → home my-reg`; DATA-MODEL §1/§5). A wrong
  target corpus silently produces garbage candidates — assert the routing in M3-4.
- **One shared embed space (DEC 1, LOCKED)** is what makes the cross-corpus ANN meaningful — the
  control chunk and the law sections live in the same 1536-d space (asserted back at M0-7). With
  the Mode-B fake vectors the pipeline is **plumbing-only**; real mapping quality needs Mode A.
