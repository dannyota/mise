# M4 Audit Q&A — Design Spec

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the TypeScript reasoning endpoint that answers audit questions with cited,
grounded evidence over SSE — or cleanly abstains — reading only tier-scoped evidence as the
user.

**Architecture:** A separate Hono + Claude Agent SDK service (`apps/reasoning`) calls mise's
existing MCP evidence tools (`search`, `document`, `graph`) as a read-only, permission-gated
MCP client. Caller identity flows through OIDC → MCP → RLS so evidence never crosses tier
boundaries. Answers stream over SSE with 7 event types; the agent loop is bounded and audited.

**Tech stack:** Node 24 LTS, Hono, Claude Agent SDK (`@anthropic-ai/agent-sdk`), Zod, jose
(OIDC), pino, Vitest 4, gts, pnpm 11, TS 6.0 strict ESM.

---

## Decisions (autonomous — all locked for this spec)

1. **WS1 scope (MCP finalization):** The 3 existing MCP tools work but need provenance fields
   (source_url, citation_path) consistently populated on every response. Finalize = add
   provenance, publish JSON Schemas from Go types, add cross-tier-deny provider tests. No new
   tools.

2. **OIDC library:** `jose` (already spec'd in UI-DESIGN and CODE_STYLE_TS). Lightweight, no
   runtime deps, handles JWKS rotation. No passport/express middleware — Hono middleware only.

3. **MCP client approach:** The Agent SDK's built-in MCP client connects to mise's serving MCP
   over stdio (in-process for local dev) or SSE (production). Tier propagation = custom header
   on every tool call carrying the verified caller identity; serving's MCP tools read the header
   and SET LOCAL ROLE before querying. This matches DATA-GOVERNANCE §4.

4. **Model routing:** Haiku 4.5 default, Sonnet 4.6 escalation. Config seam via env
   (`MODEL_DEFAULT`, `MODEL_ESCALATION`). Escalation triggered by thin-support signal (< N
   evidence hits or low confidence on first pass). DEC 11 thresholds calibrated against golden
   set in M4-16.

5. **SSE event design:** 7 event types per API-CONTRACT §4: `token` (streamed answer text),
   `citation` (inline evidence reference), `chain` (control-chain hop), `evidence_checked`
   (tool call completed), `abstain` (clean refusal), `done` (terminal), `error` (terminal
   failure). Hono's built-in SSE support (`streamSSE` from `hono/streaming`).

6. **Permission model:** SDK permission config allows only `mcp__mise__search`,
   `mcp__mise__document`, `mcp__mise__graph`. Deny everything else (filesystem, bash, write,
   network). Negative tests assert each denied class fails closed.

7. **Audit logging:** Agent SDK PreToolUse/PostToolUse hooks log every tool call + model turn
   to structured JSON (pino). Fields: model, prompt_hash, caller, correlation_id, tool_name,
   timestamps. Audit rows never include denied evidence content.

8. **Contract generation:** Go types → OpenAPI → `openapi-typescript` → generated TS types in
   `packages/contract`. MCP JSON Schemas generated separately from Go struct tags. Reasoning
   imports generated types; hand-mirrored schemas removed. Un-regenerated drift fails the build.

9. **Testing strategy:**
   - Mode B (offline/CI): fake model + fake MCP at the seam. No GCP calls.
   - Vitest unit tests: agent loop paths (cite, abstain, refusal, iteration-cap, cancel).
   - Permission negative tests: each denied tool class.
   - Tier propagation test: RLS-denied row never reaches model.
   - Contract tests: Zod schemas validate against generated types.
   - E2E harness: full question → MCP → model-fake → SSE response (no browser).
   - Eval gates: abstention ≥ 0.95, citation correctness ≥ 0.95, current-law = 1.0.

10. **Injection defense:** Adversarial instructions embedded in evidence cannot change tool
    permissions, tier, or write behavior. Tested with fixture containing malicious instructions.

11. **Wave structure for subagent execution:** 6 waves matching dependency order:
    - W1: M4-1,2,3 (MCP finalization — Go, parallel)
    - W2: M4-4,5 (schema publish + provider tests)
    - W3: M4-6,7,8 (reasoning scaffold + model seam + OIDC)
    - W4: M4-9,10,11 (MCP client + permissions + envelope)
    - W5: M4-12,13,14,15,16,17,18 (agent loop — sequential, heavy)
    - W6: M4-19,20,21,22,23 (contract + E2E + eval)

---

## Scope boundaries

- **In scope:** Everything in the M4 plan (23 tasks, 4 workstreams).
- **Out of scope:** Vue UI (M5), load/SLO testing (M6), REST `/translate` (M5), new evidence
  generation (M1-M3 only), M1b internal connectors.
- **Deferred:** Haiku→Sonnet escalation threshold tuning is provisional until golden-set
  calibration (M4-16). DEC 11 closes at M4 exit.

---

## Risk mitigations

| Risk                         | Mitigation                                                      |
| ---------------------------- | --------------------------------------------------------------- |
| Confused deputy (R9)         | Tier propagation test: denied row never reaches model           |
| Prompt injection (R11)       | Read-only agent + permission allowlist + injection fixture test |
| Contract drift (R8)          | Generated types + contract test in CI                           |
| Model policy drift (R1)      | Config seam — model endpoint swap, not rebuild                  |
| Over-abstain / under-abstain | Golden-set calibration before exit (M4-16)                      |

---

## File structure (new/modified)

```
apps/reasoning/src/
  agent/          — Agent SDK setup, model routing, loop bounds
  tools/          — MCP client wiring (read-only evidence tools)
  http/           — Hono routes, SSE streaming, OIDC middleware
  audit/          — PreToolUse/PostToolUse hooks, structured logging
  config.ts       — Zod-validated env config (existing, extended)
  index.ts        — Hono app entry (existing, extended)

packages/contract/src/
  mcp-schemas/    — Generated MCP JSON Schemas
  rest/           — Generated REST/SSE types from OpenAPI
  index.ts        — Re-exports

pkg/mcp/          — (Go) Provenance additions to existing tools
```
