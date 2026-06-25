# Mise — M4 · Workstream 4: Contract & verify

Tie serving and reasoning together with generated contracts, E2E tests, eval gates, and operator
docs for the API-only Audit Q&A milestone. Part of [Milestone M4](./README.md).

See also:

- [M4 overview](./README.md)
- [API-CONTRACT](../../../design/API-CONTRACT.md) §4/§5
- [TESTING](../../../engineering/TESTING.md) §4/§5
- [OBSERVABILITY](../../../engineering/OBSERVABILITY.md)
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §9
- Previous workstream: [Agent behavior](./03-agent-behavior.md)

---

## 1. Tasks

Each row = one reviewable PR. This workstream proves the API behavior before any browser UI ships.

| ID    | Task                                                                                                                      | Deliverable / done-when                                                                                      | Design ref                   | Depends on   | Size | Review | Risk |
| ----- | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ---------------------------- | ------------ | ---- | ------ | ---- |
| M4-19 | **Generated `packages/contract` for reasoning (new):** export REST/SSE/MCP types consumed by `apps/reasoning`             | reasoning imports generated types; hand-mirrored schemas removed; un-regenerated provider change fails build | API-CONTRACT §5              | M4-17        | S    | Medium | Med  |
| M4-20 | **Consumer contract tests (new):** validate reasoning request/response/SSE handling against generated schemas             | Vitest contract tests fail on schema drift; RFC 9457 errors and SSE events validated                         | TESTING §4 · API-CONTRACT §5 | M4-19        | S    | Medium | Med  |
| M4-21 | **API E2E harness (new):** run question→MCP→model fake→SSE response without browser                                       | E2E covers cited answer, abstain, denied evidence, cancellation, and error cases                             | TESTING §3/§4                | M4-20        | M    | Heavy  | High |
| M4-22 | **Q&A eval gate (new):** run abstention accuracy, citation correctness, and current-law precision on the golden set       | abstention ≥ 0.95, citation correctness ≥ 0.95, current-law precision = 1.0                                  | TESTING §5                   | M4-21, M4-16 | M    | Heavy  | High |
| M4-23 | **Runbook + observability hooks (new):** document config, logs/traces/metrics, model-turn audit lookup, and failure modes | operator can run local fake and real Vertex modes; traces link request id to MCP/model/audit rows            | OBSERVABILITY · AI-GOV §9    | M4-22        | S    | Medium | Med  |

---

## 2. Notes

- **M4-21 is the milestone proof.** It tests the full API/SSE behavior without waiting for the web
  UI, keeping M4 independently shippable.
- **Contract tests are bidirectional pressure.** Serving owns the provider schema; reasoning and
  web consume generated types. A schema change without regeneration should fail loudly.
- **Eval gates are inherited from the evidence engine.** The agent must not degrade citation
  correctness, abstention accuracy, or current-law precision.
- **Runbooks belong before UI.** Operators need to debug model/tool calls and RLS propagation while
  the browser is still out of scope.
