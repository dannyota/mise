# Mise — M5 · Workstream 4: Reports · corpus admin · verify

Finish the web milestone with exportable reports, Corpus Admin, and the browser test gates that
prove generated contracts, tier hiding, and no browser model client. Part of
[Milestone M5](./README.md).

See also:

- [M5 overview](./README.md)
- [UI-DESIGN](../../../design/UI-DESIGN.md) §2/§5
- [API-CONTRACT](../../../design/API-CONTRACT.md) §3/§5
- [DATA-MODEL](../../../design/DATA-MODEL.md) §9/§10
- [TESTING](../../../engineering/TESTING.md) §3/§4
- [CODE_STYLE_VUE](../../../engineering/CODE_STYLE_VUE.md)
- Previous workstream: [Findings · Q&A · notifications](./03-findings-qa-notifications.md)

---

## 1. Tasks

Each row = one reviewable PR. This workstream closes the web milestone's verification gates.

| ID    | Task                                                                                                                           | Deliverable / done-when                                                                                        | Design ref                    | Depends on  | Size | Review | Risk |
| ----- | ------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- | ----------------------------- | ----------- | ---- | ------ | ---- |
| M5-16 | **Reports export (new):** Coverage report and Findings register `.xlsx` export                                                 | exports include permitted rows only, citations/provenance, generated timestamp, and filter metadata            | UI-DESIGN §2 · DATA-MODEL §7  | M5-11       | M    | Medium | Med  |
| M5-17 | **Corpus Admin (new):** ingest status, Temporal run links, source config summary, and retry controls for authorized users      | admin screen shows per-corpus ingest health and lets authorized operator retry safe workflows                  | DATA-MODEL §9 · ARCH §3       | M5-6, M5-15 | L    | Heavy  | High |
| M5-18 | **Tier-hide browser tests (new):** lower-tier session tests for dashboard, graph, review, findings, Q&A evidence, and export   | tests prove UI never renders backend-denied rows and does not infer denied metadata through counts/errors      | DATA-GOV §2 · TESTING §3      | M5-17       | M    | Heavy  | High |
| M5-19 | **No-model-client + contract drift tests (new):** assert browser imports no model SDK and builds only against generated types  | static check blocks model SDK imports; schema drift breaks web build; generated client is the only API surface | DEC 5 · API-CONTRACT §5       | M5-18       | S    | Medium | Med  |
| M5-20 | **Web polish + accessibility pass (new):** responsive layout, keyboard navigation, loading/empty/error states, and a11y checks | critical screens pass component tests/a11y checks; no clipping/overlap on desktop and mobile                   | UI-DESIGN §5 · CODE_STYLE_VUE | M5-19       | M    | Medium | Med  |

---

## 2. Notes

- **Exports are governed views.** They must apply the same server-side tier filter as the screen,
  not re-query broader data client-side.
- **Corpus Admin is not registry GA.** It shows and operates the five M1 corpora; adding arbitrary
  new corpus kinds is M6.
- **Verification is the closeout.** M5 only exits when browser tests prove tier hiding, generated
  contract use, SSE rendering, and no model provider client in `apps/web`.
- **Accessibility and responsive behavior are part of done.** The UI is an operator workbench, so
  dense screens still need keyboard reachability and stable layout.
