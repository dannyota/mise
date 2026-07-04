<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Folder Structure (scaffolding plan)

Plan the repo layout **before** scaffolding code. mise is **one product, three runtimes**
(Go · TypeScript · Vue), so the shape is a **monorepo**.

See also:

- [ARCHITECTURE.md](../design/ARCHITECTURE.md) (the services)
- [CODE_STYLE_GO.md](./CODE_STYLE_GO.md)
- [CODE_STYLE_TS.md](./CODE_STYLE_TS.md)
- [CODE_STYLE_VUE.md](./CODE_STYLE_VUE.md)

---

## ⚖️ Decisions

- **Monorepo.** One repo — the services are tightly coupled (shared MCP contract, atomic
  cross-service changes), the team is small. Not polyrepo.
- **One Go module at the root** (**reuses + improves** the banhmi/laksa engine — same author;
  least friction, `go build ./...` works): `cmd/` binaries + `pkg/` libraries.
- **pnpm workspace** for the JS/TS side (`apps/*`, `packages/*`).
- **One deployable per service**, each its own container → independent scaling (ARCHITECTURE §9).

## 🧩 Layout

```
mise/
  README.md  CLAUDE.md
  docs/                       # design + convention docs (this folder)
  go.mod  go.sum              # ONE Go module, rooted here
  cmd/
    serving/                  # evidence + graph API + MCP servers           (Go)
    worker/                   # Temporal workers: ingest pipeline + detectors (Go)
    migrate/                  # DB migrations / schema bootstrap              (Go)
  pkg/                        # Go libraries (reused from banhmi/laksa + new)
    rag/                      #   retrieve · embed · lexical (hybrid ScaNN+BM25/RRF)
    ingest/                   #   source connectors (vbpl · congbao · sbvhanoi · bnm · agclom · sc · sharepoint)
    graph/                    #   nodes · edges · detectors (relation/conflict/gap/staleness)
    store/                    #   AlloyDB access (sqlc), RLS, migrations
    mcp/                      #   MCP server implementation (per corpus)
    config/                   #   typed config, Vertex interface seams
  apps/
    reasoning/                # TS reasoning endpoint (Node 24 LTS + Hono + Claude Agent SDK)
      src/ { agent, tools, http, auth, audit, config }  package.json  Containerfile
    web/                      # Vue 3.5 SPA (Vite)
      src/ { views, components, composables, api }  package.json  Containerfile
  packages/
    contract/                 # generated TS types from serving's OpenAPI + MCP JSON Schema (API-CONTRACT §5)
  deploy/                     # k8s/Helm, KEDA scalers, GKE config, per-service Containerfiles
  pnpm-workspace.yaml         # apps/* + packages/*
  package.json                # root (pnpm) — workspace scripts
  Taskfile.yml                # build/test/lint across Go + JS
  .github/workflows/          # CI, one job per service
  .env.example
```

## 🧭 Why this shape

- **Go at root** matches the banhmi/laksa engine layout and keeps `go build ./...` /
  `go test ./...` trivial.
- **`cmd/` thin, `pkg/` logic** — idiomatic Go; binaries just wire and start.
- **`apps/`** holds runnable JS services; **`packages/contract`** holds the **generated**
  evidence/MCP types — `serving` (Go) is the source of truth and types are generated
  outward so `web` and `reasoning` can't drift ([API-CONTRACT.md](../design/API-CONTRACT.md) §5,
  enforced by the contract test, [TESTING.md](./TESTING.md) §4).
- **One container per deployable** (serving · worker · reasoning · web) → independent KEDA
  scaling, matching ARCHITECTURE §9.

## 🧱 Service boundaries (who talks to whom)

- `web` → `serving` (REST) and `reasoning` (SSE). **The UI never calls a model.**
- `reasoning` → `serving` (MCP, tier-propagated) and Vertex (Claude).
- `worker` → `store` + Vertex (Gemini); `serving` → `store`.

(Full picture: ARCHITECTURE §2.)

## 🛠️ Conventions per language

| Runtime                     | Style guide                              |
| --------------------------- | ---------------------------------------- |
| Go (serving · worker · pkg) | [CODE_STYLE_GO.md](./CODE_STYLE_GO.md)   |
| TypeScript (reasoning)      | [CODE_STYLE_TS.md](./CODE_STYLE_TS.md)   |
| Vue (web)                   | [CODE_STYLE_VUE.md](./CODE_STYLE_VUE.md) |
| Docs                        | [DOC_STYLE.md](../DOC_STYLE.md)          |

Cross-cutting: [TOOLCHAIN.md](./TOOLCHAIN.md) (versions/linters) ·
[CI-CD.md](./CI-CD.md) (build/release) · [TESTING.md](./TESTING.md) ·
[OBSERVABILITY.md](./OBSERVABILITY.md) · [API-CONTRACT.md](../design/API-CONTRACT.md).

## 🗺️ Scaffold Defaults

- Keep `pkg/` for Go libraries because the banhmi/laksa engine already uses that layout and mise
  reuses it directly.
- Put Helm-based GKE/KEDA assets under `deploy/`; raw manifests can be generated from the chart
  for review or constrained adopters.
- `packages/contract` types and MCP schemas are **generated** from the Go API/MCP schema —
  [API-CONTRACT.md](../design/API-CONTRACT.md) §5.
