<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — TypeScript Code Style (backend / reasoning endpoint)

For the **reasoning endpoint** — Node 24 LTS + Hono + the Claude Agent SDK. Base: the
**Google TypeScript Style Guide**, enforced by **`gts`** (Google's TS style toolchain —
ESLint + Prettier + a Google `tsconfig`, with `gts lint` / `gts fix` to auto-detect and
auto-format). On top of gts: **TS strict**, **ESM** (`"type": "module"`,
`moduleResolution: "nodenext"`), **typescript-eslint v8** project rules below, **pnpm**,
**Vitest**. Pinned versions + workspace rules: [TOOLCHAIN.md](./TOOLCHAIN.md) §1/§5.
(Frontend Vue/TS: [CODE_STYLE_VUE.md](./CODE_STYLE_VUE.md).)

---

## 🧩 Size & filenames (lint-enforced)

- **Filenames:** lower **kebab-case** (`model-routing.ts`, `evidence-tools.ts`) — enforced by
  ESLint `unicorn/filename-case`. One concern per file.
- **File length** ≤ 300 lines (ESLint `max-lines`); **function length** ≤ 80
  (`max-lines-per-function`); **line length** = **80** (Prettier `printWidth`, Google
  default). Split before a file sprawls.
- **Auto-format:** `gts fix` (or `prettier --write` + `eslint --fix`) — same checks run in
  pre-commit + CI (TOOLCHAIN §6, CI-CD §2).

## 🗄️ Types & validation

- `strict: true`; **no `any`** — use `unknown` and narrow. Prefer `type` aliases; mark
  `readonly` where possible.
- **Zod at every boundary** — HTTP request bodies, Agent SDK tool inputs (`betaZodTool`),
  and **env** (parse `process.env` once into a typed config; fail fast on missing vars).

## ⚠️ Async & errors

- `async/await` only; **no floating promises** (lint `@typescript-eslint/no-floating-promises`).
- Typed error classes — never throw strings. Handle Agent SDK **refusals** and **iteration
  caps** explicitly. Use `AbortController` for timeouts / cancellation.

## 🧱 Structure (`apps/reasoning/src/`)

- `agent/` — Agent SDK setup + model routing (Haiku default / Sonnet for hard cases).
- `tools/` — MCP wiring to mise's evidence tools (**read-only**).
- `http/` — Hono routes + `streamSSE` for the chat.
- `auth/` — `jose` OIDC verification → caller tier.
- `audit/` — pino + the Agent SDK hooks.
- `config/` — Zod-parsed env. `index.ts` only wires and starts.

## 📊 Logging & audit

- **pino** structured JSON; no `console.log` in prod. Every tool call + model turn is logged
  via Agent SDK **PreToolUse / PostToolUse** hooks (AI-GOVERNANCE §9).

## 🔒 Security

- **Read-only tools only**; deny fs/bash via the SDK permission config. **Never put secrets
  or override-style instructions in prompts**; treat retrieved evidence as _data_, not commands.

## 🧪 Testing

- **Vitest**; mock the model + MCP at the seam, assert the tool calls and the cite/abstain path.
