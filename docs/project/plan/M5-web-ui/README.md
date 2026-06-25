# Mise — Milestone M5: Web UI

A **detailed, PR-sized milestone.** Bring the whole product into the browser: every UI-DESIGN §2
screen lives in the **Vue 3.5 SPA** (Vite, behind OIDC) over the existing serving REST + reasoning
SSE surfaces — Dashboards, Graph Explorer, Review Workbench, Findings, Resolution board, Audit Q&A
chat, Change Timeline, Notifications, Reports, cross-lingual side-by-side evidence, and Corpus Admin
— consuming the **generated contract** types, with the browser **never calling a model**. It adds
**no new backend subsystem**, and ships **no** new corpus kind or scale-out (→ M6).

The breakdown is split across the four workstream files below; tasks are PR-sized and
dependency-ordered (`M5-1 … M5-20`). Each task: **Size** XS–L · **Review** Light/Medium/Heavy ·
**Risk** Low/Med/High.

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [RISKS](../../RISKS.md)
- [UI-DESIGN](../../../design/UI-DESIGN.md) (all §)
- [API-CONTRACT](../../../design/API-CONTRACT.md) §3/§4/§5
- [DATA-MODEL](../../../design/DATA-MODEL.md) §7/§10
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §5/§7
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§4
- [CODE_STYLE_VUE](../../../engineering/CODE_STYLE_VUE.md)
- [DECISIONS](../../DECISIONS.md) 5/10/17/19
- Previous: [Milestone M4](../M4-audit-qa/README.md)
- Next: [Milestone M6](../M6-scale/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** all UI-DESIGN §2 screens are live in the Vue SPA, consuming the generated contract
  over serving REST + reasoning SSE, behind OIDC, with the browser **never calling a model** —
  reaching **M5 — the full product in a browser**.
- **Implements (design):** [UI-DESIGN](../../../design/UI-DESIGN.md) (all) ·
  [API-CONTRACT](../../../design/API-CONTRACT.md) §3/§4/§5 ·
  [DATA-MODEL](../../../design/DATA-MODEL.md) §7/§10 ·
  [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §5/§7 ·
  [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§4 · [DECISIONS](../../DECISIONS.md)
  5/10/17/19.
- **Exit criteria (DoD):**
  - **Generated contract only** — the SPA builds against `packages/contract`; a non-regenerated
    serving schema change **fails the web build** (M5-2/M5-19; API-CONTRACT §5,
    [RISKS](../../RISKS.md) R8).
  - **No model client in `web`** — a static check blocks any model SDK import; the browser calls only
    the reasoning SSE endpoint (M5-19; DECISIONS 5).
  - **Tier-gated screens hide what RLS would deny** — a **lower-tier session test** across
    dashboard / graph / review / findings / Q&A evidence / export proves the UI never renders a
    backend-denied row and does not infer denied metadata via counts/errors (M5-18;
    DATA-GOVERNANCE §2). The UI filter is **convenience; RLS at the DB is the boundary**.
  - **Server-resolved tier + capability flags** — the UI reads tier + `translate_allowed` from a
    server bootstrap call and **never asserts its own tier** (M5-3).
  - **Audit Q&A chat** renders the reasoning **SSE** stream with inline citations, the control chain,
    the **evidence-checked** badge, abstain, and stop — cancellable on client close (M5-13;
    API-CONTRACT §4).
  - **Promote/reject/relink + resolution board** work with idempotency and **evidence-verified
    closure** (M5-10/M5-12).
  - **Per-pane translate toggle is capability-gated** — public corpora translate on demand; a
    confidential tier with `translate_allowed=false` renders the refusal and makes **no call** (M5-5;
    AI-GOVERNANCE §7).
  - **Notifications + webhook management** — webhook delivery is **disabled or internal-only until
    DECISIONS 19** closes; the UI surfaces the policy state and refuses unsafe config (M5-15).
  - **Reports export** — Coverage report + Findings register `.xlsx`, **permitted rows only**, with
    citations/provenance + filter metadata (M5-16).
  - **Corpus Admin** — per-corpus ingest health, live Temporal status, and safe retry for authorized
    operators (M5-17).
  - **Accessibility + responsive pass** — critical screens pass component/a11y checks; no
    clipping/overlap on desktop or mobile (M5-20).
- **Out of scope:** new corpus kinds / registry GA / multimodal → M6; load/SLO tuning → M6; the
  reasoning agent + SSE **semantics** (M4 owns them — M5 only renders the stream); new evidence/edge
  generation (M1–M3 — M5 only reads + attests).

---

## 2. Dependencies & gates

- **Upstream:**
  - **M4** — the reasoning **SSE** endpoint + the finalized serving MCP evidence tools and the
    generated `packages/contract` the SPA renders.
  - **M3** — the review-queue + findings REST/MCP surface the Review Workbench and Findings/Resolution
    screens drive.
  - **M2** — the `graph` API (`/graph/*`) the Graph Explorer + control-chain drawer read.
  - **M1** — tier-tagged evidence + the per-tier **RLS** the tier-hide tests rely on.
  - **M0** — the `packages/contract` generation seam and the `apps/web` stub.
- **Decision/configuration gates:**
  - **DECISIONS 5 (LOCKED — separate TS reasoning service, no model in the browser).** Frames the
    whole milestone: the browser calls only the reasoning SSE endpoint; a static check asserts no
    model client in `web` (M5-19).
  - **DECISIONS 10/17 (LOCKED — bank-owned managed services).** Per-pane translate hits the **Google
    Cloud Translation API** on the **bank's own** GCP (a managed service, **not** the reasoning LLM);
    allowed in the reference deployment and gated for confidential tiers behind a **server-resolved
    capability flag** (M5-3/M5-5; AI-GOVERNANCE §7).
  - **DECISIONS 19 (OPEN — webhook egress policy).** Webhook delivery stays **disabled/internal-only**
    until the SSRF-safe allowlist / URL validation is specified; gates **M5-15**
    ([RISKS](../../RISKS.md) R15).
- **External prerequisites:** an **OIDC issuer/JWKS** for the SPA login; the M4 reasoning endpoint +
  serving REST reachable; the **Mode B** fakes for offline component tests (no GCP call).

---

## 3. Workstreams

The M5 breakdown, four reviewable workstreams, dependency-ordered (IDs `M5-1 … M5-20`). The shared
shell + evidence pane land first (every later screen reuses them); the large analyst screens are
split into child components/composables to stay under the `.vue` cap. Each task: **Size** XS–L ·
**Review** Light/Medium/Heavy · **Risk** Low/Med/High.

| #   | Workstream                                                          | Tasks         | Theme                                                                                             |
| --- | ------------------------------------------------------------------- | ------------- | ------------------------------------------------------------------------------------------------- |
| 1   | [Shell & shared](./01-shell-and-shared.md)                          | M5-1 … M5-6   | app shell + OIDC · typed client · capability flags · evidence pane + gated translate · dashboards |
| 2   | [Graph & review](./02-graph-and-review.md)                          | M5-7 … M5-10  | Graph Explorer · control-chain drawer · Review Workbench · promote/reject/relink                  |
| 3   | [Findings · Q&A · notifications](./03-findings-qa-notifications.md) | M5-11 … M5-15 | findings register · resolution board · SSE chat · change timeline · notifications/webhook         |
| 4   | [Reports · admin · verify](./04-reports-admin-verify.md)            | M5-16 … M5-20 | `.xlsx` exports · Corpus Admin · tier-hide + no-model-client tests · a11y                         |

---

## 4. Test & eval gates (this milestone)

- **Build/lint:** gts/ESLint/Prettier/markdownlint clean; every `.vue` stays under the **400-line
  cap** (CODE_STYLE_VUE; TESTING §1).
- **Component (Vitest + Vue Test Utils, offline):** screens render against the generated contract
  types with faked API/SSE — assert cite rendering, the abstain state, and stream cancel; no GCP call
  (TESTING §3).
- **Contract (anti-drift gate):** `web` (TS) builds against the **generated** `packages/contract`; an
  un-regenerated serving schema change **fails the web build**, run repo-wide in CI (API-CONTRACT §5,
  TESTING §4; [RISKS](../../RISKS.md) R8).
- **Tier-hide (the headline UI gate):** a lower-tier session test proves the UI never renders a
  backend-denied row and does not infer denied metadata through counts/errors — across dashboard,
  graph, review, findings, Q&A evidence, and export (DATA-GOVERNANCE §2; [RISKS](../../RISKS.md) R2).
- **No-model-client (DECISIONS 5):** a static check blocks any model SDK import in `apps/web`; the
  generated client is the only API surface.
- **SSE:** the chat renders the **7 event types** in order and **cancels** on client close
  (`AbortController`) without orphaning the agent loop (API-CONTRACT §4).
- **Webhook egress ([RISKS](../../RISKS.md) R15):** the webhook config UI refuses unsafe endpoints;
  delivery is disabled/internal-only until DECISIONS 19 closes, with negative tests for
  private-address and DNS-rebinding cases.
- **Not yet:** the load/SLO test (TESTING §6) — it lands at scale (M6).

---

## 5. Risks (this milestone)

- **UI filter mistaken for the RLS boundary** (M5-3/M5-7/M5-18) — the UI tier-filter is convenience;
  the boundary is **RLS at the DB**. Both invariants — the browser never calls a model, and the
  filter is never the boundary — are asserted by tests ([RISKS](../../RISKS.md) R2, UI-DESIGN §4).
- **Managed-AI policy drift on translate** (M5-5) — per-pane translate ships evidence text to the
  **Google Cloud Translation API** on the **bank's own** GCP (DECISIONS 10/17). Built behind a
  **server-resolved capability flag** (M5-3) so a stricter adopter policy disables or self-hosts
  translation without a rebuild ([RISKS](../../RISKS.md) R1, AI-GOVERNANCE §7).
- **Webhook SSRF** (M5-15) — arbitrary outbound URLs are an SSRF / control-plane risk; webhook
  delivery stays disabled/internal-only until **DECISIONS 19**'s allowlist + URL validation lands,
  proven by negative tests for private-address and DNS-rebinding ([RISKS](../../RISKS.md) R15).
- **Contract drift across three consumers** (M5-2/M5-19) — `web` diverges if types are hand-mirrored;
  import the **one generated contract**, never mirror — a drift fails the web build
  ([RISKS](../../RISKS.md) R8).
- **Screen sprawl vs the line cap** (M5-7/M5-13) — Graph Explorer, Review Workbench, and the chat are
  large; split layout, drawer, table, and stream into child components + composables to stay under
  the 400-line `.vue` cap (CODE_STYLE_VUE).
