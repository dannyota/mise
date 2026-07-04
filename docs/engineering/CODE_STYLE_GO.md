<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Go Code Style

For the Go backend — **serving · worker (ingest/detectors) · MCP · store**. Base:
**Effective Go** + the **Google Go Style Guide**; format + lint with **golangci-lint v2**
(`go tool golangci-lint fmt` / `run` — config `.golangci.yml`). This file is the
project-specific delta. Versions, the `tool` directive, and the Go-1.26 feature adoption
live in [TOOLCHAIN.md](./TOOLCHAIN.md).

---

## 🧭 Runtime & version

- **Go 1.26** (pinned via `go.mod` toolchain, TOOLCHAIN §1). Module: `danny.vn/mise`.
- **Lean on the runtime, don't fight it:** container-aware `GOMAXPROCS` (don't override it
  on GKE), Green Tea GC default, `os.Root` for file sandboxing, `new(expr)` for nullable
  metadata pointers — the full adoption list is [TOOLCHAIN.md](./TOOLCHAIN.md) §3.

---

## 🧱 Structure

- `cmd/<binary>/main.go` is **thin** — parse config, wire dependencies, start. Logic lives
  in `pkg/`.
- Small **interfaces, defined by the consumer**; accept interfaces, return concrete structs.
- **Functional options** for configurable constructors: `NewClient(opts ...Option)` where
  `type Option func(*Client)` — proven in s1ctl for SDK clients; use for `ingest.Source`,
  embedder, MCP clients, store constructors.
- `context.Context` is the **first parameter** of anything doing I/O; honour cancellation.
- **Every Vertex touchpoint sits behind a Go interface** (embed · parse · judge · ground)
  so it can be faked offline (LOCAL-DEV §4) — never hard-wire the SDK call.
- **Auth as `http.RoundTripper`:** wrap auth (OIDC token injection, ADC) as a transport
  decorator — composable, testable, keeps auth out of business logic.
- **REST handlers (huma v2, first use in `pkg/httpapi`):** one typed `Input`/`Output` struct
  pair per operation — path/query params via struct tags (`path:"ref"`, `query:"max_depth"`),
  the payload under `Output.Body`. Wire types (`*Wire` suffix, e.g. `EdgeWire`) map 1:1 to the
  domain type they mirror and are never the domain type itself, so a schema change is a
  deliberate, visible edit. Errors are `huma.ErrorNNNxxx(msg, err)` (RFC 9457
  `application/problem+json`) for an expected 4xx; return a plain wrapped `error` only for a
  genuine failure — huma maps that to a 500 itself.

## ⚠️ Errors

- Wrap with `fmt.Errorf("doing X: %w", err)`; classify with `errors.Is` / `errors.As`;
  define sentinel errors for known conditions.
- **Typed errors for external boundaries:** define `*APIError` (Vertex), `*StoreError`
  (AlloyDB), `*IngestError` (source connectors) with structured fields (status, code,
  message) — test with `errors.As`, not string matching.
- **No `panic` in library code.** Return errors; log them **once**, at the boundary
  (don't log-and-return).

## 🗄️ Data

- **sqlc** for type-safe queries. Hand-written SQL only for the retrieval CTEs the
  banhmi/laksa engine already maintains (hybrid ScaNN + BM25 / RRF) — keep them in one place,
  commented.
- RLS is enforced in the DB; bind the **caller's tier to the session role** per request.

## 🧩 Concurrency

- Always thread `context`; never leak goroutines; bound worker pools. Temporal owns ingest
  fan-out — don't reinvent retry/orchestration in app code.

## 📊 Logging

- `log/slog`, structured JSON; carry `run_id` / request id. No `fmt.Println` in services.

## 🧪 Testing

- **Table-driven** tests with `t.Run`; fakes via the Vertex interfaces; run with `-race` in CI.
- **`testing/synctest`** (Go 1.25) for concurrent / timeout / backoff logic in workers and
  Temporal activities — virtual time, deterministic. Full strategy: [TESTING.md](./TESTING.md).

## 📚 Naming & filenames

- Packages: short, lowercase, no underscores or plurals. Document every exported identifier
  (`revive` `exported` + `var-naming` enforce MixedCaps / initialisms).
- **Filenames:** lowercase, words run together or `snake_case` only where it aids reading
  (`hybrid_search.go`); `_test.go` for tests, `_<GOOS>.go` for platform files. One concern
  per file; the file name names the concern.

## 🛠️ Size — keep files & functions small

Enforced by `.golangci.yml`, not just asked for:

- **File length** ≤ ~500 lines (`revive` `file-length-limit`) — split a growing file by
  concern before it sprawls.
- **Function length** ≤ 80 lines / 50 statements (`funlen`); **line length** ≤ 120 (`lll`);
  **cognitive complexity** ≤ 30 (`revive`).
- Go has no official hard line limit (Google Go Style) — 120 is our cap for readability.
- `go tool golangci-lint fmt` auto-formats; `run --fix` auto-applies fixable findings.
