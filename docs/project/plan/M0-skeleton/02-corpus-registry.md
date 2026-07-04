<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M0 · Workstream 2: Corpus registry

Generalize the engine's single-jurisdiction assumption into a **corpus registry** — a typed
descriptor per corpus — and seed the five descriptors, asserting the one locked invariant: every
corpus shares the same embedding space. The net-new heart of M0. Part of
[Milestone M0](./README.md).

See also:

- [M0 overview](./README.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §9
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §1/§8
- [DECISIONS](../../DECISIONS.md) 1
- Previous: [Foundation](./01-foundation.md)
- Next: [Datastore & orchestration](./03-datastore-orchestration.md)

---

## 1. Tasks

Each row = one reviewable PR. The single-jurisdiction → registry generalization is isolated here
so it can't leak into retrieval/store ([RISKS](../../RISKS.md) R10).

| ID   | Task                                                                                                                                                                                                                                                                            | Deliverable / done-when                                                                                                             | Design ref                 | Depends on | Size | Review | Risk |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | -------------------------- | ---------- | ---- | ------ | ---- |
| M0-5 | **Descriptor type + loader (new):** define the `corpus` descriptor (`id, kind, source_plugin, citation_scheme, embed{model,dims,task_type}, access_tier, tier, jurisdiction, graph_role, metadata_config`) + typed loader; generalize the engine's `jurisdiction` field into it | descriptor type + config loader; unit test resolves every field; replaces the hard-coded single-jurisdiction assumption             | DATA-MODEL §9 · ARCH §8    | M0-1       | M    | Medium | Med  |
| M0-6 | **Seed the 5 descriptors (new):** register `vn-reg`, `my-reg` (public), `group-std` (group, `my`), `local-policy` / `local-sop` (local, `vn`) with correct tiers + jurisdictions                                                                                                | all 5 descriptors load from config; `kind`/`tier`/`jurisdiction`/`graph_role` correct per corpus                                    | DATA-MODEL §1/§9 · ARCH §1 | M0-5       | S    | Medium | Med  |
| M0-7 | **One-embed-space + jurisdiction assertion (new):** a test pins the locked invariant and the mapping rule                                                                                                                                                                       | test asserts embed `model`/`dims`/`task_type` identical across all 5, and `satisfies` maps by jurisdiction (Local→host, Group→home) | DATA-MODEL §1/§9 · DEC 1   | M0-6       | XS   | Medium | Low  |

---

## 2. Notes

- The descriptor is the **only** place the engine should learn "which corpus / which
  jurisdiction" — if that assumption is wired deeper (retrieval, store keys), the refactor sprawls
  past M0. Catch it in M0-5 review (the headline M0 risk, [RISKS](../../RISKS.md) R10).
- M0-7 is tiny but load-bearing: the **one shared embed space** (DECISIONS 1, LOCKED) is what
  makes cross-corpus mapping possible at M3 — assert it now so a later corpus can't quietly break
  it (re-checked at M6's registration validator).
- Adding a new operating entity later is just a new `local-*` descriptor with a different
  `jurisdiction` — no rename, no new tier (the "Group/Regional" trap; DATA-MODEL §1). M0 only
  proves the seam; M6 exercises it.
