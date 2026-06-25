# Mise — M5 · Workstream 3: Findings · Q&A · notifications

Build the operational screens around findings and Audit Q&A: Findings register, Resolution board,
SSE chat rendering, timeline, and notifications/webhook management. Part of
[Milestone M5](./README.md).

See also:

- [M5 overview](./README.md)
- [UI-DESIGN](../../../design/UI-DESIGN.md) §2/§4
- [API-CONTRACT](../../../design/API-CONTRACT.md) §3/§4
- [DATA-MODEL](../../../design/DATA-MODEL.md) §7/§10
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §5/§9
- [DECISIONS](../../DECISIONS.md) 19
- Previous workstream: [Graph & review](./02-graph-and-review.md)
- Next workstream: [Reports · corpus admin · verify](./04-reports-admin-verify.md)

---

## 1. Tasks

Each row = one reviewable PR. Webhook delivery must remain disabled or internal-only until DEC 19's
egress policy is implemented.

| ID    | Task                                                                                                                              | Deliverable / done-when                                                                                                | Design ref                      | Depends on   | Size | Review | Risk |
| ----- | --------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------- | ------------ | ---- | ------ | ---- |
| M5-11 | **Findings register (new):** list conflict/gap/staleness findings with evidence, severity, status, and filters                    | findings table renders permitted rows and evidence previews; lower-tier session test hides denied findings             | UI-DESIGN §2 · DATA-MODEL §6    | M5-2, M5-4   | L    | Medium | Med  |
| M5-12 | **Resolution board (new):** create/update resolutions and action plans, with owners and due/status fields                         | resolution workflow persists through API; closed finding requires evidence-verified closure                            | DATA-MODEL §7 · API-CONTRACT §4 | M5-11        | L    | Heavy  | High |
| M5-13 | **Audit Q&A chat (new):** render reasoning SSE events, inline citations, control chain, evidence-checked badge, abstain, and stop | chat streams/cancels correctly; browser calls only reasoning endpoint, never a model provider                          | AI-GOV §5 · API-CONTRACT §4     | M4-23, M5-4  | L    | Heavy  | High |
| M5-14 | **Change timeline (new):** show amendment events, detector runs, review actions, and model/audit events                           | timeline is filterable by corpus/node/finding; entries link back to evidence and audit context                         | DATA-MODEL §3/§7 · AI-GOV §9    | M5-11, M5-13 | M    | Medium | Med  |
| M5-15 | **Notifications + webhook management (new):** in-app/email notifications and webhook config UI gated by DEC 19 egress policy      | webhooks disabled/internal-only until allowlist/URL validation exists; UI shows policy state and refuses unsafe config | DATA-MODEL §10 · DEC 19         | M5-12        | M    | Heavy  | High |

---

## 2. Notes

- **Q&A chat is rendering, not reasoning.** M4 owns the agent and SSE semantics; M5 owns a correct,
  cancellable, cited rendering of that stream.
- **Webhook UI cannot get ahead of policy.** DEC 19 exists because arbitrary outbound URLs are an
  SSRF/control-plane risk; the UI should surface disabled/refused states until the server policy is
  closed.
- **Findings and resolutions are operational records.** They need filtering and workflows, but the
  evidence remains verbatim/cited, not re-summarized into authority.
- **Timeline is an audit navigation tool.** It should link model/tool/review events without
  duplicating sensitive evidence outside the governed stores.
