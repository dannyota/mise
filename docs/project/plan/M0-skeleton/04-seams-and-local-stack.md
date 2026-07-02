# Mise — M0 · Workstream 4: Seams & local stack

Put every Vertex touchpoint behind a Go interface (so Mode B runs fully offline), stand up the
serving skeleton, and wire the one `podman compose` stack + CI offline path that proves M0. Part
of [Milestone M0](./README.md).

See also:

- [M0 overview](./README.md)
- [LOCAL-DEV](../../../engineering/LOCAL-DEV.md) §1/§2/§4
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §3
- [DECISIONS](../../DECISIONS.md) 1/14
- Previous: [Datastore & orchestration](./03-datastore-orchestration.md)

---

## 1. Tasks

Each row = one reviewable PR. The old single "embedder seam" task is split into the real adapter
(M0-14) and the fake + dim-assertion (M0-15); the compose task into the stack (M0-18) and the CI
offline wiring (M0-19).

| ID    | Task                                                                                                                       | Deliverable / done-when                                                                                    | Design ref                 | Depends on   | Size | Review | Risk |
| ----- | -------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | -------------------------- | ------------ | ---- | ------ | ---- |
| M0-14 | **Embedder interface + `gemini-embedding-001` adapter @1536-d (reuse→change):** keep `pkg/rag/embed`; add the real adapter | seam returns a **1536-d** vector from `gemini-embedding-001`; `VERTEX=real` selects it (DEC 1/14)          | ARCH §3 · LOCAL-DEV §4     | M0-1         | M    | Medium | Med  |
| M0-15 | **Fake embedder + dim assertion (new):** deterministic fake of the same dim; a build-time guard on the dimension           | `VERTEX=fake` returns a deterministic 1536-d fake; a non-1536-d vector **fails the build** (DEC 1)         | ARCH §3 · LOCAL-DEV §4     | M0-14        | S    | Light  | Low  |
| M0-16 | **Fake parse/judge/ground seams (new):** extend the seam pattern beyond embed (canned shapes, no logic)                    | every Vertex touchpoint sits behind a Go interface with a fake; the offline run needs no GCP (Mode B)      | ARCH §3 · LOCAL-DEV §4     | M0-15        | S    | Light  | Low  |
| M0-17 | **Serving skeleton (reuse→adapt):** `cmd/serving` boots under an RLS role, exposes `/healthz` + an empty MCP scaffold      | serving boots under compose; health green; MCP scaffold registers zero tools cleanly                       | ARCH §2 · FOLDER_STRUCTURE | M0-11        | S    | Light  | Low  |
| M0-18 | **`podman compose` stack (new):** one compose file brings up AlloyDB + Temporal + serving + worker; documented up/down     | `podman compose up` stands the whole local stack; `VERTEX=fake`; `.env.example` complete (LOCAL-DEV §1/§2) | LOCAL-DEV §1/§2            | M0-13, M0-17 | M    | Medium | Med  |
| M0-19 | **CI Mode-B plumbing path (new):** CI runs the offline (fake-seam) plumbing path green                                     | CI green on Mode B — no Vertex/GCP call needed to pass (LOCAL-DEV §4, TESTING §2)                          | LOCAL-DEV §4 · TESTING §2  | M0-18        | S    | Light  | Low  |

---

## 2. Notes

- **The seam is the policy-variant mechanism for the whole project.** Every Vertex call — embed,
  parse, judge, ground (and later the serve model) — sits behind a Go interface so a stricter
  adopter policy can swap to self-hosted AI without a rewrite (AI-GOVERNANCE §7;
  [RISKS](../../RISKS.md) R1). M0 establishes the pattern; later milestones reuse it.
- **DEC 14 is locked behind M0-14's interface: in-app Go embedder**, not AlloyDB's DB-side
  `google_ml.embedding()` — the offline fake this call site enables is what gates CI
  (LOCAL-DEV §4).
- Mode B (offline, fake vectors) validates **plumbing only** — cross-corpus mapping is not
  semantically meaningful with fake vectors; real-quality work uses Mode A (real Vertex from the
  dev project). CI runs Mode B; feature/quality work runs Mode A.
