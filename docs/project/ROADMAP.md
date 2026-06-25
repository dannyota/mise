# Mise — Roadmap

The milestone view of the build: the seven phases as **milestones M0–M6**, the **critical path**
that links them, the **exit gates** each milestone must satisfy, and the **review-load** shape
that paces a solo part-time build. It owns _sequence + gating_; the per-milestone task detail is
[plan/](./plan/README.md), the goals + delta are under [plan/](./plan/README.md), execution risk
is [RISKS](./RISKS.md).

See also:

- [PLAN](./PLAN.md) (build-plan index)
- [GOALS](./plan/GOALS.md)
- [DELTA](./plan/DELTA.md)
- [RISKS](./RISKS.md)
- [DECISIONS](./DECISIONS.md)
- [COST](./COST.md)
- [M0](./plan/M0-skeleton/README.md)
- [M1](./plan/M1-ingest/README.md)
- [M2](./plan/M2-graph/README.md)
- [M3](./plan/M3-detectors/README.md)
- [M4](./plan/M4-audit-qa/README.md)
- [M5](./plan/M5-web-ui/README.md)
- [M6](./plan/M6-scale/README.md)

---

## 1. Milestones

Each phase reaches one milestone; the milestone _is_ the phase's demonstrable outcome.

| Milestone | Plan                                          | Delivers                                                                                    | First demoable value                                  |
| --------- | --------------------------------------------- | ------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| **M0**    | [M0-skeleton](./plan/M0-skeleton/README.md)   | the stack stands up locally — module, corpus registry, AlloyDB + Temporal, Vertex fake seam | `podman compose up` is green                          |
| **M1**    | [M1-ingest](./plan/M1-ingest/README.md)       | 5 corpora ingested, embedded @1536-d, searchable per-corpus, tier-tagged                    | per-corpus evidence search (MCP)                      |
| **M2**    | [M2-graph](./plan/M2-graph/README.md)         | the `graph` schema + explicit internal edges + graph API                                    | the SOP→Policy→Group chain query                      |
| **M3**    | [M3-detectors](./plan/M3-detectors/README.md) | the 4 detectors + findings + the review queue                                               | a grounded `satisfies` candidate + a conflict finding |
| **M4**    | [M4-audit-qa](./plan/M4-audit-qa/README.md)   | cited, grounded answers over REST + MCP + SSE                                               | ask a question → cited answer / abstain               |
| **M5**    | [M5-web-ui](./plan/M5-web-ui/README.md)       | all screens live in the Vue SPA                                                             | the full product in a browser                         |
| **M6**    | [M6-scale](./plan/M6-scale/README.md)         | corpus registry GA + multimodal + new scope by descriptor only                              | add a corpus with zero core edits                     |

---

## 2. Critical path

**Solo + part-time ⇒ strictly sequential** (PLAN): one phase at a time, each gating the next.
There is no parallel track to exploit; the lever is keeping each phase's scope tight.

```mermaid
flowchart LR
    M0["M0 — stack up"] --> M1["M1 — ingest 5"]
    M1 --> M2["M2 — graph spine"]
    M2 --> M3["M3 — detectors"]
    M3 --> M4["M4 — Audit Q&A"]
    M4 --> M5["M5 — Web UI"]
    M5 --> M6["M6 — scale"]
    classDef gated fill:#6d28d9,stroke:#a78bfa,color:#fff,stroke-width:2px
    class M0,M1,M3,M4,M5 gated
```

- **Violet = has non-code exit gates** (§3) — M0/M3/M4/M5 have open implementation-review
  decisions; M1 has real-adopter connector integration input.
- **Fan-in, not a parallel path:** M4 reads M1+M2+M3 artifacts and M5 reads M2+M3+M4 — drawn
  linearly because one builder runs them in order, but each later phase depends on _all_ prior.
- **Earliest value lands at M1** (evidence search) and again at **M4** (Q&A); M5 makes it a
  product. If priorities force a cut, M1→M4 is the spine; M5/M6 are surface + scale.

---

## 3. Exit Gates By Milestone

Open implementation-review decisions plus adopter inputs that create milestone exit criteria.

| Resolve by | Decision                                           | Gates                                                                                |
| ---------- | -------------------------------------------------- | ------------------------------------------------------------------------------------ |
| **M0/M1**  | **14** embedding call site                         | in-app Go embedder vs AlloyDB `google_ml.embedding()` implementation review          |
| **M1**     | **13** adopter-provided source access config       | real-intranet validation; build/CI run on fixtures or a non-bank test site meanwhile |
| **M3**     | **18** eval golden-set bootstrap                   | the mapping precision/recall eval emits its first baseline                           |
| **M3/M4**  | **11** judge + serve threshold / escalation tuning | judge threshold calibration (M3) + serve-model abstain/escalation calibration (M4)   |
| **M5**     | **19** webhook egress policy                       | SSRF-safe endpoint allowlist / URL validation before webhook delivery ships          |

- **DECISIONS 1** (embedding @1536-d) is **enforced** as a fail-closed invariant from M0 and
  re-checked at M6 (the registry validator).
- **DECISIONS 10/17** (bank-owned Vertex for internal control text + region posture) and
  **12/15/16** (runtime sizing/scale/cost) are reference deployment and COST/DEPLOYMENT knobs.
- A milestone can **start** on fixture/offline scope before an input or review is complete; it does
  not **exit** until its exit gates pass or the documented policy variant is selected (AI-GOV §7).

---

## 4. Build load (pacing the solo part-time reviewer)

**Every milestone is detailed into PR-sized tasks** — the counts below are the actual breakdown, not
estimates. The constraint is **human review**, so the table shows where **Heavy-review** work
concentrates — the real pacing signal (RISKS R4).

| Milestone | Tasks | Review-load skew                                        | Notes                                                    |
| --------- | ----: | ------------------------------------------------------- | -------------------------------------------------------- |
| M0        |    19 | mostly **Light** (Heavy: the schema/RLS migration)      | 4 workstreams; scaffolding + seams                       |
| M1        |    24 | mixed, **Heavy** on RLS + the crawler                   | tier isolation + real-source integration land here       |
| M2        |    15 | **Heavy** (graph schema, tier, RLS)                     | small but dense — every edge task touches tier isolation |
| M3        |    18 | **mostly Heavy** — grounding, AI-gov, audit             | the crown-jewel milestone; highest review density        |
| M4        |    23 | **mostly Heavy** — tier propagation, abstain, injection | governance-critical; the new TS service                  |
| M5        |    20 | mixed — Heavy on tier-gated screens                     | review-load eases on static screens                      |
| M6        |    13 | Heavy on reports/multimodal (gated)                     | proves extensibility; low core-change risk               |

- **Heavy-review density peaks at M3–M4.** Budget the most direction/review time there; don't
  bunch other Heavy work alongside.
- **Sizing is relative, not a calendar.** Each task is one reviewable PR (Size XS–L · Review
  Light/Medium/Heavy · Risk Low/Med/High); there are no dates — sequence by dependency, pace by
  review capacity. Re-pace at each milestone boundary against [RISKS](./RISKS.md) §4.
