# Mise — Milestone M4: Audit Q&A

Reaches **M4 — cited, grounded Audit Q&A over REST + MCP + SSE.** The **separate TypeScript
reasoning endpoint** (Node 24 LTS + Hono + the Claude Agent SDK, running Claude Haiku 4.5 /
Sonnet 4.6 on the **bank's own** Vertex via ADC) becomes a **read-only, permission-gated MCP
client** that calls mise's evidence tools **as the user** (tier propagation, no confused deputy),
composes a **cited** answer or **abstains**, and **streams over SSE**. To get there, the serving
MCP evidence tools are finalized and their JSON Schemas published, and the generated
`packages/contract` + contract test keep provider and consumers aligned. It does **not** ship the
Web UI (M5) — verification is an API/E2E harness, no browser — and adds **no scale-out** (M6).

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [RISKS](../../RISKS.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §6
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §5/§8
- [API-CONTRACT](../../../design/API-CONTRACT.md) §1/§2/§4/§5
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §4
- [TESTING](../../../engineering/TESTING.md) §3/§4/§5
- [DELIVERY-MODEL](../../../engineering/DELIVERY-MODEL.md)
- [DECISIONS](../../DECISIONS.md) 5/10/11/17
- Previous: [Milestone M3](../M3-detectors/README.md)
- Next: [Milestone M5](../M5-web-ui/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** the reasoning endpoint answers an audit question with a grounded, **cited** answer
  (or a clean **abstain**), streamed over SSE, reading **only tier-scoped evidence as the user** —
  reaching **M4 — Audit Q&A**.
- **Implements (design):** [ARCHITECTURE](../../../design/ARCHITECTURE.md) §6 ·
  [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §5/§8 ·
  [API-CONTRACT](../../../design/API-CONTRACT.md) §1/§2/§4/§5 ·
  [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §4 ·
  [TESTING](../../../engineering/TESTING.md) §3/§4/§5.
- **Exit criteria (DoD):**
  - The serving **MCP evidence tools** `search` · `document` · `graph` are finalized — read-only,
    tier-scoped, each returning **verbatim text + citation + provenance**; their **JSON Schemas
    are published** as the MCP-native contract (API-CONTRACT §2/§5).
  - `apps/reasoning` boots as its own container (Hono + Agent SDK), Haiku/Sonnet on the **bank's
    own** Vertex via ADC behind a **model-id config seam**; `/healthz` green (ARCH §6).
  - **OIDC verify (`jose`)** resolves the caller; the endpoint calls MCP **as the user** — a
    **tier-propagation test** proves an RLS-denied row never reaches the model (no confused
    deputy; API-CONTRACT §1, RISKS R9).
  - **Permission gate proven:** allow `mcp__mise__*`; **deny** filesystem / bash / write /
    arbitrary network — a **negative test** asserts each denied class fails closed (AI-GOV §5).
  - The guarded **agent loop** (retrieve → opt rerank → compose, building the control chain)
    returns a **cited** answer grounded by **Claude citations**, or **abstains** on thin support;
    bounds enforced (iteration cap · `max_tokens` · refusal handling) (AI-GOV §5).
  - The **SSE** stream emits the **7 event types** (`token` · `citation` · `chain` ·
    `evidence_checked` · `abstain` · `done` · `error`) and is **cancellable** (client close →
    `AbortController`) (API-CONTRACT §4).
  - **Per-turn audit:** Agent SDK **PreToolUse / PostToolUse** hooks log every tool call + model
    turn (model · inputs · cite-spans · caller · ts) (AI-GOV §9).
  - The generated **`packages/contract`** + the **contract test** are green repo-wide — a serving
    schema change that isn't regenerated **fails the consumer build** (API-CONTRACT §5, TESTING §4).
  - **Eval gates** pass on the Q&A golden set: **abstention accuracy ≥ 0.95** and **citation
    correctness ≥ 0.95** (TESTING §5).
- **Out of scope:** the Vue chat UI + SSE rendering → M5; the REST `/translate` confidential gate
  is an M5 surface; load/SLO test and registry GA → M6; new evidence/edge generation (owned by
  M1–M3 — M4 only **reads** what they produced).

---

## 2. Dependencies & gates

- **Upstream:**
  - **M3** — promoted edges + findings, so the agent can build the **control chain**
    (SOP → Policy → Group → law) the `graph` tool returns.
  - **M2** — the `graph` MCP tool + the `graph` schema it reads.
  - **M1** — ingested, tier-classified evidence the `search` / `document` tools return, and the
    per-tier **RLS** the propagation test relies on.
  - **M0** — the `packages/contract` generation seam, the serving MCP scaffold, and the
    `apps/reasoning` stub.
- **Decision/calibration gates:**
  - **DECISIONS 10/17 (LOCKED — bank-owned Vertex for internal control text).** The serve agent
    reads confidential **evidence** to Claude on the **bank's own** GCP Vertex, inside the bank's
    perimeter. It adds **no exposure category beyond the write path** — the agent sees only text
    already parsed + embedded at write time (AI-GOV §7). If an adopter later bars managed AI by
    policy, the serve model **swaps to the self-hosted variant behind the same read-only MCP loop**
    — config, not rearchitecture.
  - **DECISIONS 11 (LOCKED provisional) / DEC 30 (serve-path thresholds).** Model id is a
    **config seam**, so this is **tuning, not rework**; gates the abstain/grounding **threshold**
    calibration (**M4-16**), confirmed against the golden set before exit.
  - **DECISIONS 5 (separate TS reasoning service — LOCKED).** Frames the whole milestone: no
    reasoning in Go, none in the browser; the polyglot cost is isolated behind the MCP/SSE
    boundary.
- **External prerequisites:** ADC for **Claude on the bank's Vertex** (`CLAUDE_CODE_USE_VERTEX=1`)
  for the fidelity (Mode A) path; an **OIDC issuer/JWKS** to verify caller tokens; the **Mode B**
  offline path (faked model + MCP at the seam) gates CI with no GCP call (TESTING §3, LOCAL-DEV §4).

---

## 3. Workstreams

The M4 breakdown, split into four reviewable workstreams. Tasks are PR-sized and dependency-
ordered (IDs `M4-1 … M4-23`); the Heavy tier-propagation, permission-gate, grounding/abstain, and
SSE work is split so no single PR is oversized. Each task: **Size** XS–L · **Review**
Light/Medium/Heavy · **Risk** Low/Med/High.

| #   | Workstream                                         | Tasks         | Theme                                                          |
| --- | -------------------------------------------------- | ------------- | -------------------------------------------------------------- |
| 1   | [Serving MCP surface](./01-serving-mcp-surface.md) | M4-1 … M4-5   | finalize search/document/graph · publish MCP JSON Schemas      |
| 2   | [Endpoint core](./02-endpoint-core.md)             | M4-6 … M4-11  | `apps/reasoning` · OIDC + tier propagation · MCP + permissions |
| 3   | [Agent behavior](./03-agent-behavior.md)           | M4-12 … M4-18 | guarded loop · grounding + abstain + bounds · SSE · hooks      |
| 4   | [Contract & verify](./04-contract-and-verify.md)   | M4-19 … M4-23 | generated `packages/contract` · contract test · E2E + eval     |

---

## 4. Test & eval gates (this milestone)

- **Serving (Go) — provider side (TESTING §2/§4):** the three MCP tools return verbatim text +
  citation + provenance under an RLS role; responses **validate against the published JSON
  Schemas**; a cross-tier-deny test confirms a denied row is absent from every tool's output.
- **Reasoning (TS) — unit, offline = Mode B (TESTING §3):** **Vitest** mocks the **model + MCP at
  the seam**; assert the **cite path**, the **abstain** path, the tool-call sequence, **refusal**
  handling, and the **iteration-cap** stop — no Vertex/GCP call needed to pass.
- **Permission gate (negative test, AI-GOV §5):** allow `mcp__mise__*`; assert filesystem · bash ·
  write · arbitrary-network calls are **denied** by the SDK permission config and fail closed.
- **Tier propagation (RLS-propagation test, RISKS R9):** the endpoint forwards the caller's
  identity; an RLS-denied row **never** reaches the model — the confused-deputy proof.
- **SSE:** the **7 event types** stream in order and the stream **cancels** on client close
  (`AbortController`) without orphaning the agent loop (API-CONTRACT §4, CODE_STYLE_TS).
- **Contract test (anti-drift gate, TESTING §4):** `reasoning` (Zod) builds against the
  **generated** `packages/contract`; an un-regenerated serving schema change **fails the build**,
  run repo-wide in CI (RISKS R8).
- **Eval harness (TESTING §5):** on the Q&A golden set — **abstention accuracy ≥ 0.95** and
  **citation correctness ≥ 0.95** (gates, inherited from the banhmi engine); **current-law
  precision = 1.0** preserved through the agent (it must not surface superseded text as current).
- **Injection test (AI-GOV §8, RISKS R11):** adversarial instructions embedded in evidence cannot
  make the read-only agent act or exceed the caller's tier.
- **Not yet:** the Vue/Vitest UI tests + the load/SLO test (TESTING §6) — they land with the UI
  (M5) and at scale (M6).

---

## 5. Risks (this milestone)

- **Confused-deputy / tier propagation** (M4-9/M4-13) — the endpoint reads under the **service's**
  tier instead of the **user's**; the boundary is the **DB, not the agent**, so forward the
  verified identity on every MCP call and prove it with the RLS-propagation test
  ([RISKS](../../RISKS.md) R9, Heavy review).
- **Managed-AI policy drift** (M4-13/M4-14/M4-16) — confidential **evidence** reaches Claude on
  the **bank's own** Vertex by default (DECISIONS 10/17). The config seam keeps a stricter adopter
  policy a model-endpoint swap behind the same MCP loop, not a rebuild ([RISKS](../../RISKS.md)
  R1).
- **Prompt injection via evidence** (M4-15/M4-18) — ingested text carries adversarial instructions;
  the agent is **read-only, permission-gated, tier-scoped**, so the blast radius is capped to the
  caller's tier — assert it in the injection test ([RISKS](../../RISKS.md) R11).
- **Contract drift** (M4-19/M4-20) — serving / reasoning diverge if types are hand-mirrored; the
  **one generated contract** + the contract test fail the build on un-regenerated drift
  ([RISKS](../../RISKS.md) R8).
- **Grounding / abstain + threshold calibration** (M4-14/M4-16) — wrong thresholds make the agent
  guess or over-abstain; DECISIONS 11 defines the tuning discipline — thresholds **logged +
  versioned** and calibrated against the golden set before exit ([RISKS](../../RISKS.md) R7;
  grounding correctness).
