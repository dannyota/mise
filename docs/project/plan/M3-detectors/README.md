# Mise — Milestone M3: Cross-corpus detectors

The **crown-jewel milestone, planned in detail.** The 4 cross-corpus detectors run as Temporal
workflows on the graph spine from M2: **RelationDetect** runs the full
embed→ScaNN→rerank→**Gemini judge**→**Check Grounding** chain and persists only grounded
`satisfies` candidates (`promoted=false`) with a complete audit row; SOP→law is **transitive
only** (DEC 7); **ConflictDetect** raises the Group-standard vs VN/MY-law contradiction (the crown
jewel); **GapScan** and **StaleScan** raise gap and staleness findings; the **review-queue API**
supports promote/reject/relink (re-trigger + recompute, no auto-promotion); and the **bootstrap
golden set** (DEC 18) seeds the eval baseline. It does **not** ship the Review Workbench UI (→ M5),
the reasoning endpoint / Audit Q&A (→ M4), or load/SLO tuning (→ M6).

This milestone is the **most review-dense** in the plan — most tasks are Heavy-review (grounding,
AI-governance, model-egress controls, the findings migration). The breakdown is split across the
four workstream files below; tasks are PR-sized and dependency-ordered (`M3-1 … M3-18`).

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [RISKS](../../RISKS.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §5/§6/§7/§8
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §1/§4/§9
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §5
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §5
- [API-CONTRACT](../../../design/API-CONTRACT.md) §4
- [TESTING](../../../engineering/TESTING.md) §5
- [DELIVERY-MODEL](../../../engineering/DELIVERY-MODEL.md)
- [DECISIONS](../../DECISIONS.md) 7/10/11/17/18
- Previous: [M2](../M2-graph/README.md)
- Next: [M4](../M4-audit-qa/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** the 4 cross-corpus detectors run as Temporal workflows on the M2 graph, propose
  **grounded** `satisfies` candidates, and raise gap/conflict/staleness **findings** into an
  API-level review queue — reaching **M3 — cross-corpus detectors**. AI proposes, grounding gates,
  humans attest; **nothing auto-promotes** (AI-GOVERNANCE §1).
- **Implements (design):** [DATA-MODEL](../../../design/DATA-MODEL.md) §5/§6/§7/§8 ·
  [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §1/§4/§9 ·
  [ARCHITECTURE](../../../design/ARCHITECTURE.md) §5 ·
  [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §5 ·
  [API-CONTRACT](../../../design/API-CONTRACT.md) §4 · [TESTING](../../../engineering/TESTING.md) §5.
- **Exit criteria (DoD):**
  - **Findings schema applied** — `finding` / `finding_resolution` / `action_plan` migrated into
    the `graph` schema; idempotent + re-runnable; each finding row carries its driving metadata
    and `node_refs[]` (DATA-MODEL §6/§7); RLS test green (edge/finding inherits the stricter tier).
  - **Candidate pipeline (Method B)** — a new/changed control chunk runs
    embed(`FACT_VERIFICATION`)→ScaNN ANN vs the target law corpus→Ranking rerank, yielding top-k
    candidate pairs (DATA-MODEL §5); offline path runs on fake seams (Mode B).
  - **Judge + ground** — the Gemini judge returns `edge_type · confidence · quoted_spans`;
    **Check Grounding** verifies the rationale is entailed by **both** verbatim texts; only
    candidates clearing the confidence + grounding thresholds are written `promoted=false` with the
    full audit row (`run_id · model · prompt_hash · confidence · grounding_score · created_by`);
    **ungrounded candidates are discarded** (AI-GOVERNANCE §4, DATA-MODEL §5).
  - **RelationDetect workflow** runs the chain end-to-end as a Temporal workflow with deterministic
    activities (`testing/synctest` green, no flakes).
  - **SOP→law transitive only** — `covers` edges are **computed** through the promoted
    SOP→Policy→…→law chain, **never a direct judge run** (DEC 7; a test asserts no judge call on the
    `local-sop → law` pair).
  - **ConflictDetect** raises a `conflict` finding on the **Group-standard vs VN/MY-law**
    contradiction (the crown jewel) via a Check Grounding **contradiction** check on the two
    verbatim texts (DATA-MODEL §6).
  - **GapScan (cron)** raises `gap` findings (obligation with 0 `satisfies` · standard with 0
    `implements` · policy with 0 `sop`); **StaleScan (on `amendment_event`)** raises `staleness`
    findings where `amendment_event.date > downstream effective_date`, **cascading to SOPs**
    (DATA-MODEL §3/§6).
  - **Review-queue API** — `GET /reviews`, `POST /reviews/{edge}/promote|reject|relink` and `GET
/findings` / `POST /findings/{id}/resolution` write `human_attested` evidence; a **relink
    re-triggers detection** (re-judge with the correction as constraint) and **recomputes** the
    dependent findings; **no auto-promotion**; contract test green (API-CONTRACT §3).
  - **Bootstrap golden set + eval** — a curated VN/Malay `satisfies` set (DEC 18) seeds the eval
    harness; the mapping precision/recall harness runs and **emits a baseline** (not a gate at
    first run — see §4; TESTING §5).
- **Out of scope:** the Review Workbench **UI** screens (→ M5); the reasoning endpoint / Audit Q&A
  serve path (→ M4); notification fan-out + webhooks (delivery → M4/M5, DATA-MODEL §10); the
  self-hosted judge/ground **implementation** (built behind the seam here, exercised if adopter
  policy selects it — M6/run); load/SLO tuning of the detector throughput (→ M6).

---

## 2. Dependencies & gates

- **Upstream:**
  - **M2 (graph spine)** — the `graph` schema (`relation_edge` + `relation_evidence`), the
    `node_ref` + `doc_ref` stub, the tier-inheritance rule (stricter-of-two), and the explicit
    `implements`/`derives` edges (Method A). M3's `satisfies`/`covers` and the findings join onto
    this spine; the transitive `covers` walks the M2 chain.
  - **M1 (ingest)** — embeddings @1536-d + the **ScaNN index** per corpus, the metadata envelope
    (`effective_date`, `validity_status`, `amendment_event`) that GapScan/StaleScan consume, and
    the per-corpus `search`/`document` MCP surface.
  - **M0 (seams)** — the embed/judge/ground Go interface seams + their fakes (the offline Mode-B
    path that gates CI without a Vertex call).
- **Decision/calibration gates:**
  - **DEC 10/17 (LOCKED — bank-owned Vertex for internal control text).** The Gemini judge, Check
    Grounding, and the candidate-pipeline embed read confidential **policy/standard clauses** on
    the **bank's own GCP Vertex**, inside the bank's perimeter. This is the settled reference path.
    If an adopter later bars managed AI by policy, the judge/ground/embed calls swap to
    the **self-hosted variant behind the same workflow shape** (AI-GOVERNANCE §7) — config, not
    rearchitecture.
  - **DEC 11 (LOCKED provisional — judge model + thresholds).** Gemini 3.5 Flash is the default
    judge; hard cases may route to Haiku/Sonnet when eval + cost justify it. Confidence + grounding
    thresholds gate candidacy; they are logged, versioned, and **tuned against the golden set
    before exit**. Gates the tuning in **M3-9** and **M3-18**.
  - **DEC 7 (SOP→law transitive-only — locked).** Constrains **M3-12**: `covers` is computed
    through the promoted chain, never a direct judge run.
  - **DEC 18 (LOCKED provisional — eval golden-set bootstrap).** Before any human attestation
    exists there is no set to score against; the curated VN/Malay seed gates the eval baseline in
    **M3-17/M3-18**.
  - **DEC 24 (depguard — LOCKED).** `pkg/graph` stays pure/deterministic; import restrictions
    enforced by depguard. Detectors live outside that fence.
  - **DEC 26 (detectors in `pkg/detect` — LOCKED).** All detector workflows and the candidate
    pipeline live in `pkg/detect`, separate from `pkg/graph`.
  - **DEC 29 (transitive covers, DEC 7 enforced — LOCKED).** `covers` edges are computed
    through promoted chains only; a test asserts no judge call on the SOP→law path.
- **External prerequisites:** for the **Mode A** (real Vertex) quality work — the dev GCP project
  with the Gemini judge + Check Grounding + Ranking APIs enabled in the configured region. The
  **Mode B** offline path (fake seams) gates CI with no GCP call (LOCAL-DEV §4).

---

## 3. Workstreams

The M3 breakdown, split into four reviewable workstreams. Tasks are PR-sized and dependency-ordered
(IDs `M3-1 … M3-18`); the judge/ground path and the relink-recompute fan-out are split so no single
PR is oversized. Each task: **Size** XS–L · **Review** Light/Medium/Heavy · **Risk** Low/Med/High.

| #   | Workstream                                                       | Tasks         | Theme                                                                    |
| --- | ---------------------------------------------------------------- | ------------- | ------------------------------------------------------------------------ |
| 1   | [Findings & candidate pipeline](./01-findings-and-candidates.md) | M3-1 … M3-5   | findings/resolution schema + migration · Method-B embed→ANN→rerank       |
| 2   | [Judge & ground](./02-judge-and-ground.md)                       | M3-6 … M3-11  | Gemini judge · Check Grounding gate · candidate+audit write · workflow   |
| 3   | [Detectors](./03-detectors.md)                                   | M3-12 … M3-15 | transitive `covers` · ConflictDetect (crown jewel) · GapScan · StaleScan |
| 4   | [HITL & eval](./04-hitl-and-eval.md)                             | M3-16 … M3-18 | review-queue API (promote/reject/relink) · golden set · eval harness     |

---

## 4. Test & eval gates (this milestone)

- **Build/lint:** `go build`/`go test` green at Go 1.26; `golangci-lint run` (+ `modernize`) +
  gts/markdownlint clean (TOOLCHAIN §4; TESTING §1).
- **Unit (offline = Mode B):** the judge/ground/embed seams are **faked** behind their Go
  interfaces — the offline path **is** the unit-test path (TESTING §2, LOCAL-DEV §4). Asserts the
  candidate pipeline shape, the grounding gate (ungrounded → discard), and the threshold logic.
- **Concurrency:** `testing/synctest` covers the RelationDetect / ConflictDetect / StaleScan
  Temporal workflows + activity retries — virtual time, deterministic, no flakes (TOOLCHAIN §3).
- **Integration (testcontainers, AlloyDB Omni):** the findings migration creates
  `finding`/`finding_resolution`/`action_plan` in `graph`; re-run is a no-op. **RLS tests are
  mandatory** — a low-tier caller must not see a `group-std`-tier candidate edge or conflict
  finding even through the cross-corpus join (DATA-GOVERNANCE §2, TESTING §2, RISKS R2). The
  edge/finding inherits the **stricter** tier.
- **Contract:** the review-queue + findings REST/MCP surface validates against the published
  schemas; a serving change that isn't regenerated **fails the build** (API-CONTRACT §5, TESTING
  §4, RISKS R8).
- **Eval (mapping quality — the crown-jewel gate):** the harness runs against the **bootstrap
  golden set** (DEC 18) and emits **mapping precision/recall** plus the inherited banhmi
  retrieval metrics. The cross-corpus mapping metric is **new to mise / provisional** (no banhmi
  baseline) — the **first run sets the baseline, it is not a pass/fail gate** (TESTING §5, RISKS
  R5). The inherited retrieval floors (recall@k ≥ 0.90, current-law = 1.0) **do** gate, since they
  carry over from M1.
- **Not yet:** the E2E web → reasoning → serving flow (needs the UI + reasoning endpoint, M4/M5);
  load/SLO of the detector throughput (M6).

---

## 5. Risks (this milestone)

- **Managed-AI policy drift** (M3-6 … M3-13) — the judge + Check Grounding + confidential-pair
  embed run on the **bank's own** GCP Vertex by default (DEC 10/17). Every call sits behind the M0
  config seam, so a stricter adopter policy is an endpoint swap to the self-hosted variant
  (AI-GOVERNANCE §7), not a rebuild ([RISKS](../../RISKS.md) R1).
- **Mapping-quality cold start** (M3-17/M3-18) — the cross-corpus `satisfies` metric has **no
  banhmi baseline**; seed the curated VN/Malay golden set (DEC 18) and treat the first number as a
  **baseline, not a gate** ([RISKS](../../RISKS.md) R5).
- **Judge precision / threshold tuning** (M3-9/M3-18) — wrong thresholds flood the review queue or
  suppress real mappings; thresholds **logged + versioned**, tuned against the golden set before
  exit, with confidence **and** grounding both gating candidacy ([RISKS](../../RISKS.md) R7,
  DEC 11).
- **RLS correctness on the new candidate/finding surface** (M3-1/M3-2/M3-16) — a candidate edge or
  conflict finding spans tiers; a policy bug leaks confidential text through the join. Mandatory
  cross-tier-deny tests; the edge/finding inherits the **stricter** tier
  ([RISKS](../../RISKS.md) R2).
- **Relink recompute fan-out + cross-lingual false positives** (M3-13/M3-16) — a relink
  re-triggers detection and recomputes dependent findings; an unbounded cascade or a VN↔EN
  translation artifact in ConflictDetect produces noise. Bound the recompute set to affected
  nodes; ground the contradiction in **both verbatim texts**, never a translation
  ([RISKS](../../RISKS.md) R5/R7).
