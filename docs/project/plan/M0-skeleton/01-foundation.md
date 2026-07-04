<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M0 · Workstream 1: Foundation

Dev-environment foundation: bring the banhmi/laksa engine up under one `danny.vn/mise` Go module,
lay the monorepo skeleton, and stand up the three-language lint/format gates. Fast to review —
mostly scaffolding. Part of [Milestone M0](./README.md).

See also:

- [M0 overview](./README.md)
- [FOLDER_STRUCTURE](../../../engineering/FOLDER_STRUCTURE.md)
- [TOOLCHAIN](../../../engineering/TOOLCHAIN.md) §1/§2/§4/§5/§6
- Next workstream: [Corpus registry](./02-corpus-registry.md)

---

## 1. Tasks

Each row = one reviewable PR. Reuse = adapt existing engine code; New = net-new to mise.

| ID   | Task                                                                                                                                                | Deliverable / done-when                                                                        | Design ref                         | Depends on | Size | Review | Risk |
| ---- | --------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | ---------------------------------- | ---------- | ---- | ------ | ---- |
| M0-1 | **Module bring-up (reuse):** import the engine under one Go module `danny.vn/mise` at root; Go 1.26 pin + `toolchain`                               | `go build ./...` green; `go.mod` pins `go 1.26` + `tool` directives (golangci-lint, sqlc)      | FOLDER_STRUCTURE · TOOLCHAIN §1/§2 | —          | M    | Light  | Low  |
| M0-2 | **Monorepo skeleton (new):** `cmd/{serving,worker,migrate}`, `pkg/*`, `apps/{reasoning,web}` stubs, `packages/contract`, `deploy/`, workspace files | the FOLDER_STRUCTURE tree exists; `go build ./...` + `pnpm install` succeed on empty stubs     | FOLDER_STRUCTURE                   | M0-1       | S    | Light  | Low  |
| M0-3 | **Go lint gate (new):** `.golangci.yml` v2 (+ `modernize`), `go fix` drift-check, the Go pre-commit hook                                            | `golangci-lint run` clean on the skeleton; drift-check wired; lefthook runs it on staged `.go` | TOOLCHAIN §4                       | M0-2       | S    | Light  | Low  |
| M0-4 | **JS/TS + docs lint gates (new):** gts/ESLint flat config, Prettier, markdownlint; lefthook wires all three languages                               | `pnpm lint` + doc lint clean; pre-commit runs Go · TS/Vue · md formatters/linters together     | TOOLCHAIN §5/§5a/§6                | M0-2       | S    | Light  | Low  |

---

## 2. Notes

- M0-1 is the load-bearing reuse step; keep the import faithful to the engine layout so
  `go build ./...` / `go test ./...` stay trivial (FOLDER_STRUCTURE).
- Splitting the lint gate into Go (M0-3) and JS/TS+docs (M0-4) keeps each PR to one toolchain —
  the three runtimes still converge in **one** lefthook pre-commit + CI gate (TOOLCHAIN §6).
- Toolchain drift across Go 1.26 / Node 24 / the JS stack is the standing tax here — pin hard,
  fail CI on drift from day one ([RISKS](../../RISKS.md) R4).
