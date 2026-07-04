# Mise — Testing Strategy

The test pyramid across Go · TS · Vue, the **MCP/REST contract test** that keeps the
services from drifting, the **eval golden-set harness** (mapping quality), and the
**load test** that proves the read-path SLO. This doc owns _how we test_; the per-language
test rules live in the CODE_STYLE docs.

See also:

- [TOOLCHAIN.md](./TOOLCHAIN.md)
- [CI-CD.md](./CI-CD.md) (gates)
- [API-CONTRACT.md](../design/API-CONTRACT.md) (the contract under test)
- [OBSERVABILITY.md](./OBSERVABILITY.md) §4 (SLOs)
- [DATA-MODEL.md](../design/DATA-MODEL.md) §8 (golden set)

---

## 1. The pyramid

| Level           | Scope                                               | Tools                                  | Where                        |
| --------------- | --------------------------------------------------- | -------------------------------------- | ---------------------------- |
| **Unit**        | pure logic, parsers, RRF merge, metadata extraction | Go table-driven · Vitest               | every package/module         |
| **Integration** | DB queries + **RLS**, MCP server, ingest activities | Go + **testcontainers** (AlloyDB Omni) | `serving`, `store`, `worker` |
| **Contract**    | the MCP/REST surface `web`+`reasoning` depend on    | schema/contract tests (§4)             | repo-wide gate               |
| **E2E**         | a real Q&A: web → reasoning → serving → DB          | Playwright + a seeded corpus           | smoke, pre-release           |
| **Eval**        | mapping precision/recall, retrieval recall@k        | golden-set harness (§5)                | nightly + per release        |
| **Load**        | read-path latency SLO                               | k6/vegeta                              | pre-release                  |

---

## 2. Go

- **Table-driven** with `t.Run`; **`-race` in CI** (CODE_STYLE_GO).
- **`testing/synctest`** (Go 1.25, TOOLCHAIN §3) for concurrency/timeout/backoff in the
  Temporal activities and worker pools — virtual time, deterministic, no flakes.
- **Vertex seams faked** via their Go interfaces (embed · parse · judge · ground) — the
  offline path (LOCAL-DEV §4 Mode B) **is** the unit-test path.
- **RLS tests are mandatory**: a low-tier caller must not see `group-std`/`local-policy` rows
  even through a cross-corpus graph join (DATA-GOVERNANCE §2). Test against real AlloyDB
  Omni via testcontainers, not a mock.
- **Graph RLS/cross-tier-deny gate** (M2): the graph's compliance-chain surface has a dedicated
  deny suite (`pkg/store/graph_rls_test.go`, `pkg/httpapi/graph_deny_test.go`) that seeds
  local-confidential edges and asserts tier isolation across **every** read path — raw SQL
  oracle (`SET LOCAL ROLE`), `GraphRepo.GetNode`/`Chain`, REST `GET /graph/nodes` and
  `/graph/chain`, and the MCP `graph` handler — for all three roles
  (`mise_public`/`mise_group`/`mise_local`). An extraction-to-chain E2E test
  (`pkg/store/graph_e2e_test.go`) additionally proves the full Method A pipeline —
  `ExtractEdges` → `WriteExtractedEdge` → `Chain` — including write idempotency. Both run
  under `//go:build integration` against real Postgres.

## 3. TS (reasoning) & Vue (web)

- **Vitest**; mock the **model + MCP at the seam** and assert the **cite/abstain** path and
  tool calls (CODE_STYLE_TS). Assert refusals and iteration-cap handling explicitly.
- Vue: **Vue Test Utils** for components; assert the browser **never calls a model**
  (no model client in `web`) and that tier-gated screens hide what RLS would deny.

---

## 4. Contract test (the anti-drift gate)

`web` and `reasoning` both consume `serving`'s **MCP tools + REST**
([API-CONTRACT.md](../design/API-CONTRACT.md)).
A single source of truth (API-CONTRACT §5) generates types; the contract test asserts
**provider and consumers agree**:

- **Provider:** `serving` responses validate against the published JSON Schemas for every
  MCP tool (`search`/`document`/`graph`) + REST endpoint.
- **Consumers:** `reasoning` (Zod) and `web` (typed client) build against the **generated**
  types — a schema change that isn't regenerated **fails the build** (CI-CD §2).
- Run in CI repo-wide so a serving change can't silently break either consumer.

---

## 5. Eval harness (mapping quality — first-class, not a notebook)

The crown jewel (cross-lingual `satisfies` mapping) needs a runnable quality gate:

- **Golden set** = human-attested edges (DATA-MODEL §8) — promote/reject/relink decisions
  accumulate into it.
- **Metrics, floors & targets.** Retrieval-side metrics and **floors** are **inherited
  from the banhmi engine's proven `cmd/eval` harness** (same engine, measured on its VN
  golden set) — real gates from day one. The cross-corpus **mapping** metric is **new to
  mise** and **unbuilt / unmeasured at design time**, so its numbers below are **provisional
  planning targets, not gates** — set the real bar from the **first eval run** on the
  bootstrap golden set (DECISIONS 18). Tracked **per release** and trended (a drop on a
  gated metric = a model/threshold change shipped badly — model change-control,
  AI-GOVERNANCE §3). **Citation precision** carries over banhmi's ≥ 0.95 floor **value**, but
  not its meaning: banhmi's citation field scored an answer model's citations — a mode mise
  doesn't have — and was permanently unpopulated, whereas mise's `CitationPrecision` is a
  live metric, precision over every top-k hit a search call returns. Treat its floor like the
  mapping targets: provisional until the first real-corpus eval run confirms 0.95 still holds
  under the new definition.

  | Metric                                           | Floor / target     | Basis                                                                      |
  | ------------------------------------------------ | ------------------ | -------------------------------------------------------------------------- |
  | retrieval **recall@k**                           | ≥ 0.90             | banhmi hybrid measured ~0.89–1.0                                           |
  | retrieval **MRR@k**                              | ≥ 0.85             | banhmi ~0.85–0.89                                                          |
  | **current-law precision** (in-force)             | = 1.0              | banhmi 1.0 — validity correctness is non-negotiable                        |
  | **abstention accuracy**                          | ≥ 0.95             | banhmi 1.0                                                                 |
  | **citation correctness**                         | ≥ 0.95 provisional | banhmi's floor value; **new semantics** in mise — recalibrate at first run |
  | **mapping precision** (cross-corpus `satisfies`) | ~0.70 provisional  | **new to mise** — no banhmi basis; calibrate at first eval (DEC 18)        |
  | **mapping recall@k**                             | ~0.60 provisional  | as above                                                                   |

- **Cold-start:** before any human attestation exists, seed a small hand-labelled set so
  the harness produces a baseline (this is the bootstrap gap flagged for discussion).
- **Baseline-not-gate (DEC 18).** The first eval run establishes the baseline — it runs the
  full harness and records precision/recall, but `min-precision=0` and `min-recall=0`
  (provisional) so it **does not gate** CI. Thresholds are raised once the baseline is
  calibrated from real data. The eval command always exits 0 on first run; subsequent runs
  compare against the stored baseline and fail only when thresholds are set above 0.
- Runs nightly on `master` against a fixed corpus snapshot so results are comparable.

---

## 6. Load test (prove the SLO)

- Drive the read path (cache-hit and cache-miss mix) to assert **p95 < 150 ms / < 500 ms**
  (OBSERVABILITY §4) at the default 2-vCPU AlloyDB sizing — this is also how we **validate the
  sizing decision** (DECISIONS 4) rather than assuming it.
- Include a **filtered** query mix (every read is access-tier/RLS filtered) so the test
  reflects ScaNN's filtered-search path, the reason AlloyDB was chosen.

---

## 7. Test Defaults

- **Testcontainers, split by environment:** the CI integration job (CI-CD §2) runs
  `pgvector/pgvector:pg17` — smaller, no ScaNN, so its vector index falls back to HNSW
  (`internal/testdb`) — to keep the gate fast and Docker-only. **AlloyDB Omni** remains the
  local/nightly reference image for the RLS/ScaNN-sensitive suites (row-visibility policies,
  filtered ANN search) that need the real engine, not the fallback. A shared ephemeral DB is a
  local/manual acceleration option, not the reference gate.
- Run Playwright E2E **nightly and pre-release** by default; keep PR gates focused on unit,
  integration, contract, and component tests unless a small smoke path proves stable enough.
- Contract types are generated **from Go outward** into `packages/contract` (API-CONTRACT §5).
