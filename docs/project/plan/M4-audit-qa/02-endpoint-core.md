# Mise — M4 · Workstream 2: Endpoint core

Stand up the separate TypeScript reasoning service, verify OIDC, propagate caller identity to MCP,
and deny every non-evidence tool class. Part of [Milestone M4](./README.md).

See also:

- [M4 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §6
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §5
- [API-CONTRACT](../../../design/API-CONTRACT.md) §1/§4
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §4
- [DECISIONS](../../DECISIONS.md) 5
- Previous workstream: [Serving MCP surface](./01-serving-mcp-surface.md)
- Next workstream: [Agent behavior](./03-agent-behavior.md)

---

## 1. Tasks

Each row = one reviewable PR. The endpoint is a client of mise evidence tools; it is not another
data authority.

| ID    | Task                                                                                                                           | Deliverable / done-when                                                                                         | Design ref                    | Depends on  | Size | Review | Risk |
| ----- | ------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------- | ----------------------------- | ----------- | ---- | ------ | ---- |
| M4-6  | **`apps/reasoning` Hono service (new):** Node 24 + Hono skeleton, config loader, `/healthz`, container, and local fake mode    | service boots in compose and CI; health/config checks green; no model call required in Mode B                   | ARCH §6 · LOCAL-DEV §4        | M0-2        | M    | Medium | Med  |
| M4-7  | **Claude Agent SDK + model config seam (new):** configure Haiku/Sonnet on Vertex via ADC and a fake model for tests            | model id/provider comes from config; fake model exercises unit tests; real path uses adopter Vertex credentials | AI-GOV §5 · DEC 10/17         | M4-6        | M    | Heavy  | High |
| M4-8  | **OIDC verification (new):** verify bearer token with `jose`, map caller identity, roles, and correlation ids                  | invalid/missing token rejected; caller context available to every request and audit log                         | API-CONTRACT §1 · DATA-GOV §4 | M4-6        | M    | Heavy  | High |
| M4-9  | **MCP client as user (new):** call mise MCP tools with caller identity/tier propagated, not service-wide authority             | RLS-propagation test proves denied evidence never reaches the model or response path                            | DATA-GOV §4 · RISKS R9        | M4-5, M4-8  | M    | Heavy  | High |
| M4-10 | **Permission allowlist (new):** SDK permissions allow `mcp__mise__*` and deny filesystem, bash, writes, and arbitrary network  | negative tests assert each denied tool class fails closed                                                       | AI-GOV §5                     | M4-7        | S    | Heavy  | High |
| M4-11 | **Request envelope + audit context (new):** normalize question, corpus filters, locale, trace ids, idempotency, and user scope | request schema validates; audit context is attached before any tool/model call                                  | API-CONTRACT §4 · AI-GOV §9   | M4-8, M4-10 | S    | Medium | Med  |

---

## 2. Notes

- **DEC 5 is locked here.** Reasoning is a separate TypeScript service using the Agent SDK, not Go
  code and not browser-side model calls.
- **Caller identity is the boundary.** The endpoint must call MCP as the user; a service-account
  shortcut is a confused-deputy bug.
- **The permission allowlist should be boring and explicit.** Only mise evidence tools are
  available to the agent; all other tool classes fail closed and are tested.
- **Model choice is a config seam.** DEC 11 calibrates thresholds/escalation later; this
  workstream only makes the seam real.
