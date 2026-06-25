# Mise — M5 · Workstream 1: Shell & shared

The SPA foundation: app shell + OIDC, the **typed client generated from `packages/contract`**, the
shared **verbatim evidence pane** with the gated per-pane translate toggle, and Dashboards. Every
later screen reuses the client + evidence pane. Part of [Milestone M5](./README.md).

See also:

- [M5 overview](./README.md)
- [UI-DESIGN](../../../design/UI-DESIGN.md) §1/§4/§5
- [API-CONTRACT](../../../design/API-CONTRACT.md) §3/§5
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §7
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§4
- [CODE_STYLE_VUE](../../../engineering/CODE_STYLE_VUE.md)
- [DECISIONS](../../DECISIONS.md) 5/10/17/19
- Next workstream: [Graph & review](./02-graph-and-review.md)

---

## 1. Tasks

Each row = one reviewable PR. Reuse = adapt the proven Vue stack (DEC 5); New = net-new to mise.

| ID   | Task                                                                                                                                        | Deliverable / done-when                                                                                                | Design ref                      | Depends on | Size | Review | Risk |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------- | ---------- | ---- | ------ | ---- |
| M5-1 | **App shell + OIDC (reuse→adapt):** `apps/web` Vite + Vue 3.5 + router + Tailwind 4/reka-ui shell; OIDC login → bearer on every API call    | shell boots; OIDC redirect → token; unauthenticated route redirects to login; no model client imported (asserted)      | UI-DESIGN §1 · API-CONTRACT §1  | —          | M    | Light  | Low  |
| M5-2 | **Typed client from `packages/contract` (new):** a `fetch` wrapper that imports the **generated** REST types; RFC 9457 + `Idempotency-Key`  | client compiles against `packages/contract`; a non-regenerated schema change **fails the web build** (contract test)   | API-CONTRACT §5/§6 · TESTING §4 | M5-1       | S    | Medium | Med  |
| M5-3 | **Server-resolved capability flags (new):** read tier + per-tier `translate_allowed` from a server bootstrap call; no client-asserted tier  | shell exposes the server-resolved tier + capability flags; the UI never asserts its own tier (RLS is the boundary)     | DATA-GOV §2/§7 · AI-GOV §7      | M5-2       | S    | Heavy  | Med  |
| M5-4 | **Shared verbatim evidence pane (new):** side-by-side verbatim + citation_path + `source_url`; reused by graph/review/findings/chat         | pane renders two verbatim panes with citation + validity badge; verbatim source stays authoritative (UI-DESIGN §4)     | UI-DESIGN §4 · DATA-MODEL §2    | M5-2       | M    | Medium | Low  |
| M5-5 | **Per-pane translate toggle, gated (new):** `POST /translate` per pane; the toggle is **disabled + renders a refusal** when the flag is off | public corpora translate on demand; a confidential tier with `translate_allowed=false` shows the refusal, no call made | AI-GOV §7 · API-CONTRACT §3/§6  | M5-3, M5-4 | M    | Heavy  | High |
| M5-6 | **Dashboards (new):** `GET /dashboards/summary` → coverage % · open conflicts · staleness · queue depth · per-corpus ingest status          | dashboard renders the summary tiles; lower-tier session sees only permitted corpora (a tier-hide test)                 | UI-DESIGN §2 · API-CONTRACT §3  | M5-2       | M    | Medium | Low  |

---

## 2. Notes

- **The translate toggle (M5-5) is the only gated read action and the High-risk task here.** It
  ships evidence text to the **Google Cloud Translation API** — a managed service on the **bank's
  own** GCP, **not** the reasoning LLM. DECISIONS 10/17 allow it in the reference deployment.
  Built behind a **server-resolved capability flag** (M5-3): a stricter adopter policy is a config
  state and the UI renders the refusal — never a rebuild. If required, the call routes to the
  self-hosted translator behind the same `/translate` seam (AI-GOVERNANCE §7).
- **Capability flags come from the server, never the client.** The UI tier-filter is convenience;
  the boundary is RLS at the DB (UI-DESIGN §4, RISKS R2). M5-3 makes the tier + `translate_allowed`
  flag server-resolved so no screen can assert clearance it doesn't have.
- **The typed client (M5-2) is the contract anchor** for `web`: it imports `packages/contract`,
  never hand-mirrors types, so a serving change that isn't regenerated breaks the build (RISKS R8,
  TESTING §4).
- The shared evidence pane (M5-4) is reused by Graph Explorer, Review Workbench, Findings, and the
  chat — building it once keeps each downstream screen under the 400-line `.vue` cap (CODE_STYLE_VUE).
