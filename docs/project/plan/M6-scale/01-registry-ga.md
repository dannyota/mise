# Mise — M6 · Workstream 1: Registry GA

Make the corpus registry **GA**: formalize the descriptor as the single registration surface,
enforce the **single embedding space fail-closed**, drive edge eligibility off `graph_role`, finalize
`metadata_config`, and prove "**add a corpus, touch no core file**" with a CI guard and a
descriptor-only new scope. The load-bearing workstream — every new kind rides it. Part of
[Milestone M6](./README.md).

See also:

- [M6 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §8
- [DATA-MODEL](../../../design/DATA-MODEL.md) §2/§4/§9
- [API-CONTRACT](../../../design/API-CONTRACT.md) §2/§5
- [DECISIONS](../../DECISIONS.md) 1/8
- Next workstream: [Reports corpus](./02-reports-corpus.md)

---

## 1. Tasks

Each row = one reviewable PR. Reuse = generalize the M0 corpus registry; New = net-new to mise. The
fail-closed validator (M6-2) and the no-core-diff guard (M6-6) are the GA gates.

| ID   | Task                                                                                                                                                                                                                                                                            | Deliverable / done-when                                                                                                                       | Design ref              | Depends on | Size | Review | Risk |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- | ---------- | ---- | ------ | ---- |
| M6-1 | **Registry descriptor model + loader (reuse→adapt):** formalize the `corpus` descriptor (`id, kind, source_plugin, citation_scheme, embed{}, access_tier, graph_role{}, tier, jurisdiction, metadata_config`) as the one registration surface; loader resolves a descriptor set | the 5 M1 corpora load as descriptors with no behavior change; the descriptor is the only place a corpus is declared (ARCH §8)                 | ARCH §8 · DATA-MODEL §9 | —          | M    | Medium | Med  |
| M6-2 | **One-embedding-space validator — fail-closed (new):** registration refuses any descriptor whose `embed{model,dims,task_type}` ≠ the locked space; native image vectors barred                                                                                                  | a mismatched `embed{}` is rejected **before any ingest**; a unit test asserts fail-closed; `gemini-embedding-2`-style vectors refused (DEC 1) | DATA-MODEL §9 · DEC 1   | M6-1       | S    | Heavy  | High |
| M6-3 | **`graph_role` enforcement + default edges (new):** the descriptor's `graph_role{can_source,can_target,default_edges}` governs which edges a corpus may source/target and seeds default edges, applied at edge write                                                            | an edge whose endpoints violate `graph_role` is refused; default edges seed on registration; integration test green                           | DATA-MODEL §4 · ARCH §8 | M6-1       | M    | Heavy  | Med  |
| M6-4 | **`metadata_config` GA hardening (new):** finalize the per-source config shape (defaults + parse locations) deferred from M1; validate on registration                                                                                                                          | the `metadata_config` schema is fixed and validated at register time; an invalid config fails closed (DATA-MODEL §2/§9)                       | DATA-MODEL §2/§9        | M6-1       | M    | Medium | Med  |
| M6-5 | **Registry admin + MCP/REST surface + contract (new):** list/add/inspect corpora through the **generated contract**; extends the M5 Corpus Admin screen                                                                                                                         | the registry surface validates against the published schemas; the contract test is green; a drift fails the build (RISKS R8)                  | API-CONTRACT §2/§5      | M6-1       | M    | Medium | Med  |
| M6-6 | **No-core-diff GA guard (new):** a CI check that adding a corpus by descriptor touches **no** detector/graph/serve core file — the load-bearing GA acceptance                                                                                                                   | the guard adds a fixture corpus and asserts zero core diff; a core edit during corpus-add **fails CI** (registry-GA leak)                     | ARCH §8 · TESTING §1    | M6-2, M6-3 | S    | Heavy  | High |
| M6-7 | **New operating-entity scope, descriptor-only (new):** add a second `local-*` scope with a different `jurisdiction`; route `satisfies` correctly with no new tier and no core edit                                                                                              | the new scope ingests + maps by descriptor only; `satisfies` routes by jurisdiction; the no-core-diff guard stays green (DEC 8)               | DATA-MODEL §9 · DEC 8   | M6-3, M6-6 | M    | Medium | Med  |

---

## 2. Notes

- **GA is an acceptance test, not a claim.** The load-bearing definition is "add a corpus, touch no
  core file" — so M6-6's no-core-diff guard is the gate the whole milestone is measured by. If a
  code edit is still needed to add a kind, GA is not met.
- **The embed-space validator (M6-2) fails closed.** A mismatched `embed{}` is the one thing that
  silently fragments the shared vector space (DECISIONS 1 LOCKED, [RISKS](../../RISKS.md) R12); it is
  refused at registration, before any ingest, and native image vectors are barred for the same
  reason — the caption path (Workstream 3) embeds caption **text**, never an image vector.
- **`graph_role` (M6-3) is how a new kind joins the graph without a code change** — reports and
  diagrams declare `can_source`/`can_target`/`default_edges` in the descriptor, and the edge-write
  path enforces them, so Workstream 2's reports→graph mapping rides the descriptor, not a new branch.
- **`metadata_config` (M6-4) firms up the M1 placeholder** — DATA-MODEL §9 left its shape to the
  implementation phase because it depends on template consistency; M6 fixes and validates it now that
  real corpora have exercised it ([RISKS](../../RISKS.md) R6).
