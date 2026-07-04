# Mise — Milestone M0: Skeleton

The **first milestone, planned in detail.** Stand mise up by reusing and improving the
banhmi/laksa engine into the `danny.vn/mise` Go module: generalize the engine's
single-jurisdiction assumption into the **corpus registry**, put `gemini-embedding-001` @1536-d
behind the embedder seam, provision **AlloyDB Omni** (one DB, schema-per-corpus + an empty
`graph` schema) and **self-hosted Temporal**, and run the whole stack locally on **podman
compose** with the `VERTEX=fake` seam — reaching **M0: the stack stands up (locally)**. It does
**not** ingest any real corpus, build any graph edge/detector, or ship the UI.

The work is split across the four workstream files below; every milestone (M0–M8) is planned to
this depth — see the [build-plan workspace](../README.md).

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [RISKS](../../RISKS.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §2/§9/§10
- [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§9
- [FOLDER_STRUCTURE](../../../engineering/FOLDER_STRUCTURE.md)
- [LOCAL-DEV](../../../engineering/LOCAL-DEV.md)
- [TOOLCHAIN](../../../engineering/TOOLCHAIN.md)
- [DECISIONS](../../DECISIONS.md)
- Next: [Milestone M1](../M1-ingest/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** the full stack builds and runs locally on podman compose with the Vertex fake
  seam; the engine is generalized from one jurisdiction to a **corpus registry**, and AlloyDB +
  Temporal back it — reaching **M0 — the stack stands up (locally)**.
- **Implements (design):** [ARCHITECTURE](../../../design/ARCHITECTURE.md) §2/§9/§10 ·
  [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§9 ·
  [FOLDER_STRUCTURE](../../../engineering/FOLDER_STRUCTURE.md) ·
  [LOCAL-DEV](../../../engineering/LOCAL-DEV.md) · [TOOLCHAIN](../../../engineering/TOOLCHAIN.md).
- **Exit criteria (DoD):**
  - `go build ./...` + `go test ./...` green on `danny.vn/mise` at the **Go 1.26** pin;
    `golangci-lint run` (incl. `modernize`) clean (TOOLCHAIN §1/§4).
  - `podman compose up` brings up AlloyDB Omni + Temporal + serving + worker, each health-green
    (LOCAL-DEV §2).
  - `migrate` bootstraps one database with **5 corpus schemas + an (empty) `graph` schema** and
    **RLS roles per tier**; idempotent + re-runnable (ARCHITECTURE §2).
  - The **corpus registry** resolves all 5 descriptors and a test asserts the **one shared embed
    space** (`gemini-embedding-001` @1536-d) and the by-jurisdiction `satisfies` mapping
    (DATA-MODEL §9; DECISIONS 1).
  - The embedder seam returns a **1536-d** vector (real) and a deterministic fake of the same dim
    (`VERTEX=fake`); a non-1536-d vector fails the build (DECISIONS 1).
  - A trivial Temporal workflow registers + completes against the self-hosted server (no ingest).
  - Unit/plumbing tests green on the **offline (Mode B)** path — no Vertex/GCP call needed in CI.
- **Out of scope:** real ingest, connectors, Doc AI parse, the metadata envelope → M1; graph
  edges → M2; detectors → M3; reasoning endpoint → M4; Web UI → M5; registry GA → M6.

---

## 2. Dependencies & gates

- **Upstream:** none — the first milestone; it unblocks every later one.
- **Decision gates:** **DEC 4** (AlloyDB Omni from day 1, drives the container choice);
  **DEC 14** (LOCKED — in-app Go embedder, not AlloyDB `google_ml.embedding()`; kept behind the
  interface for portability); **DEC 1** (embedding @1536-d, fixes the dim contract every later
  milestone inherits).
- **External prerequisites:** a **dev GCP project** with Vertex + ADC for the optional Mode A
  fidelity path (not needed for the Mode B path that gates CI); local **Podman** with
  `podman compose` and the `google/alloydbomni` image pullable.

---

## 3. Workstreams

The M0 breakdown, split into four reviewable workstreams. Tasks are PR-sized and dependency-
ordered (IDs `M0-1 … M0-19`); the hard schema/RLS migration and the multi-part seams are split
so no single PR is oversized. Each task: **Size** XS–L · **Review** Light/Medium/Heavy · **Risk**
Low/Med/High.

| #   | Workstream                                                   | Tasks         | Theme                                                    |
| --- | ------------------------------------------------------------ | ------------- | -------------------------------------------------------- |
| 1   | [Foundation](./01-foundation.md)                             | M0-1 … M0-4   | module bring-up · monorepo skeleton · lint gates         |
| 2   | [Corpus registry](./02-corpus-registry.md)                   | M0-5 … M0-7   | descriptor type · seed the 5 · one-embed-space assertion |
| 3   | [Datastore & orchestration](./03-datastore-orchestration.md) | M0-8 … M0-13  | AlloyDB · schema-per-corpus + graph + RLS · Temporal     |
| 4   | [Seams & local stack](./04-seams-and-local-stack.md)         | M0-14 … M0-19 | embedder/Vertex seams · serving skeleton · compose · CI  |

---

## 4. Test & eval gates (this milestone)

- **Build/lint:** `go build`/`go test` green at Go 1.26; `golangci-lint run` (+ `modernize`) +
  `go fix` drift-check clean; gts/ESLint/Prettier/markdownlint clean (TOOLCHAIN §4/§5; TESTING §1).
- **Unit (offline = Mode B):** Vertex seams (embed · parse · judge · ground) faked behind their
  Go interfaces — the offline path **is** the unit-test path (TESTING §2, LOCAL-DEV §4); asserts
  1536-d + fake determinism (DECISIONS 1).
- **Integration (testcontainers, AlloyDB Omni):** migration creates 5 corpus schemas + `graph` +
  the per-tier RLS roles; re-run is a no-op (TESTING §2). Full row-visibility RLS tests land with
  data in M1 — here the gate is that roles/policies exist and apply.
- **Concurrency:** `testing/synctest` covers the Temporal worker / no-op workflow (TOOLCHAIN §3).
- **Registry:** all 5 descriptors resolve with the right tier/jurisdiction and one shared embed
  space (DATA-MODEL §9) — the locked invariant that makes cross-corpus mapping possible.
- **Not yet:** the eval golden-set harness (TESTING §5), the contract test (TESTING §4), and the
  load/SLO test (TESTING §6) — nothing to score until there is data + an evidence API (M1/M4).

---

## 5. Risks (this milestone)

- **Engine generalization leak** (M0-5/6) — the single-jurisdiction assumption wired deeper than
  the descriptor sprawls the refactor; isolate to the descriptor + config seam early
  ([RISKS](../../RISKS.md) R10).
- **DEC 14 locked** (M0-14) — in-app Go embedder, not AlloyDB `google_ml.embedding()`; the
  interface still isolates the call site, so a future IAM/latency/portability need can revisit
  it without a wider refactor.
- **AlloyDB Omni local fidelity** (M0-8/9) — the Omni container must load ScaNN + pgvector and the
  RLS/schema-per-corpus model, else "local == deploy" weakens (LOCAL-DEV §1).
- **Temporal-on-AlloyDB persistence** (M0-12) — Temporal's schema co-existing with corpus schemas
  without contention (DECISIONS 3); mis-wiring shows as flaky workflow completion.
- **Toolchain drift across three runtimes** (M0-3/4) — Go 1.26 + Node 24 + the JS stack must agree
  in one gate from day one ([RISKS](../../RISKS.md) R4 — the reviewer's time is the budget).
