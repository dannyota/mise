<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M4 · Workstream 3: Agent behavior

Implement the guarded read-only agent loop: retrieve evidence, build the chain, compose a cited
answer or abstain, stream over SSE, and log every model/tool turn. Part of
[Milestone M4](./README.md).

See also:

- [M4 overview](./README.md)
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §5/§8/§9
- [API-CONTRACT](../../../design/API-CONTRACT.md) §4
- [TESTING](../../../engineering/TESTING.md) §3/§5
- [DECISIONS](../../DECISIONS.md) 11
- Previous workstream: [Endpoint core](./02-endpoint-core.md)
- Next workstream: [Contract & verify](./04-contract-and-verify.md)

---

## 1. Tasks

Each row = one reviewable PR. The loop is bounded, read-only, and evidence-grounded.

| ID    | Task                                                                                                                                | Deliverable / done-when                                                                                              | Design ref              | Depends on   | Size | Review | Risk |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- | ----------------------- | ------------ | ---- | ------ | ---- |
| M4-12 | **Guarded retrieve loop (new):** retrieve with `search`/`document`/`graph`, cap iterations/tokens, and preserve tool-call order     | fake-model unit test covers happy path, thin-support path, refusal path, and iteration cap                           | AI-GOV §5 · TESTING §3  | M4-11        | M    | Heavy  | High |
| M4-13 | **Control-chain composer (new):** assemble SOP→Policy→Group→law evidence from graph + documents for the answer context              | answer context includes only permitted chain hops with citations; denied hop causes an abstain/partial-evidence path | ARCH §6 · DATA-GOV §4   | M4-12        | M    | Heavy  | High |
| M4-14 | **Grounded answer + Claude citations (new):** compose concise answer with inline citations to returned evidence                     | cited answer references exact evidence spans; unsupported answer path abstains                                       | AI-GOV §5 · TESTING §5  | M4-13        | M    | Heavy  | High |
| M4-15 | **Prompt-injection defense test (new):** adversarial evidence cannot change tool permissions, tier, or write behavior               | injection fixture proves evidence instructions are treated as data and cannot call denied tools                      | AI-GOV §8 · RISKS R11   | M4-14, M4-10 | S    | Heavy  | High |
| M4-16 | **Abstain/escalation threshold calibration (new):** tune support/abstain thresholds and optional Haiku→Sonnet escalation            | thresholds/model routing logged and versioned; Q&A golden-set calibration closes DEC 11 for serving                  | DEC 11 · TESTING §5     | M4-14        | M    | Heavy  | High |
| M4-17 | **SSE stream + cancellation (new):** emit `token`, `citation`, `chain`, `evidence_checked`, `abstain`, `done`, and `error` events   | event order tested; client disconnect cancels the agent loop via `AbortController`                                   | API-CONTRACT §4         | M4-14        | M    | Heavy  | Med  |
| M4-18 | **Agent hooks + per-turn audit (new):** PreToolUse/PostToolUse and model-turn audit rows with model, prompt hash, caller, and spans | every model/tool turn is logged with correlation id; audit rows omit denied evidence content                         | AI-GOV §9 · DATA-GOV §6 | M4-17        | M    | Heavy  | High |

---

## 2. Notes

- **The answer path must prefer abstain over invention.** Thin support, denied evidence, model
  refusal, or missing citations should produce a clear abstain event.
- **M4-16 is where DEC 11 closes for serving.** Thresholds and escalation are calibrated against
  eval data, not guessed in design docs.
- **SSE is part of product behavior, not UI polish.** M5 only renders the stream; M4 owns event
  semantics, order, and cancellation.
- **Audit logs must not create a side leak.** They should record enough to reconstruct the turn
  without storing evidence above the auditor's allowed tier in an unsafe place.
