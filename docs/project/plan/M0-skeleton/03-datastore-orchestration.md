# Mise — M0 · Workstream 3: Datastore & orchestration

Provision **AlloyDB Omni** (one DB, schema-per-corpus + an empty `graph` schema, RLS roles per
tier) and **self-hosted Temporal** with a working worker. The Heavy-review heart of M0 — the
schema/RLS migration is split into three so no single PR is oversized. Part of
[Milestone M0](./README.md).

See also:

- [M0 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §2/§9
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2
- [LOCAL-DEV](../../../engineering/LOCAL-DEV.md) §2
- [DECISIONS](../../DECISIONS.md) 3/4
- Previous: [Corpus registry](./02-corpus-registry.md)
- Next: [Seams & local stack](./04-seams-and-local-stack.md)

---

## 1. Tasks

Each row = one reviewable PR. The old single "schema + graph + RLS migration" (Heavy) is split
into **M0-9 / M0-10 / M0-11** so each is reviewable on its own.

| ID    | Task                                                                                                                   | Deliverable / done-when                                                                                              | Design ref              | Depends on | Size | Review | Risk |
| ----- | ---------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- | ----------------------- | ---------- | ---- | ------ | ---- |
| M0-8  | **AlloyDB Omni container (reuse→adapt):** add `google/alloydbomni` to podman compose; point the store at it            | DB container healthy under `podman compose`; store connects; ScaNN + pgvector extensions load (DEC 4)                | LOCAL-DEV §2 · ARCH §9  | M0-2       | M    | Medium | Low  |
| M0-9  | **Per-corpus schema migration (new):** `migrate` creates one DB + the 5 corpus schemas; idempotent + re-runnable       | migration creates `vn_reg`/`my_reg`/`group_std`/`local_policy`/`local_sop`; re-run is a no-op (testcontainers)       | ARCH §2 · DATA-MODEL §1 | M0-8       | M    | Heavy  | Med  |
| M0-10 | **Empty `graph` schema (new):** add the `graph` schema alongside the corpus schemas (no tables yet — M2 fills it)      | `graph` schema exists empty; migration still idempotent                                                              | ARCH §2/§4              | M0-9       | XS   | Medium | Low  |
| M0-11 | **RLS roles per tier (new):** per-access-tier DB roles + the policy skeleton on each schema; re-runnable               | roles for public / group-confidential / local-confidential exist; policies attach per schema; role-exists test green | DATA-GOV §2             | M0-9       | S    | Heavy  | Med  |
| M0-12 | **Temporal self-hosted in compose (reuse→adapt):** add Temporal; persistence reuses the AlloyDB instance (separate DB) | `temporal` up under compose; persistence on the shared instance (DEC 3); server health green                         | ARCH §9 · DEC 3         | M0-8       | S    | Medium | Med  |
| M0-13 | **Worker registration + trivial workflow (reuse→adapt):** register the worker; a no-op workflow runs end-to-end        | worker registers; no-op workflow completes; `testing/synctest` test green (no flakes)                                | ARCH §9 · TOOLCHAIN §3  | M0-12      | S    | Medium | Low  |

---

## 2. Notes

- **Why the split:** the original single migration (schema-per-corpus + graph + RLS) was one
  Heavy PR. M0-9 (schemas) / M0-10 (empty graph) / M0-11 (RLS roles) each review independently;
  RLS especially deserves its own focused review — a mis-set role is the start of a tier leak
  ([RISKS](../../RISKS.md) R2).
- **RLS at M0 is structural only** — roles + policies exist and apply; the full cross-tier-deny
  **row-visibility** tests need data and land in M1 (TESTING §2). Don't claim isolation here, only
  that the machinery is in place.
- **Temporal-on-AlloyDB** (M0-12) reuses the shared instance (separate database) — watch for
  schema contention; mis-wiring surfaces as flaky workflow completion ([RISKS](../../RISKS.md) §4
  watchlist).
