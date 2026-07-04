<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Milestone M10: REST surface completion

A Playwright walk-through of the running Web UI against the live serving process exposed the
M5↔backend gap: the SPA's screens call [API-CONTRACT §3](../../../design/API-CONTRACT.md)
endpoints that serving never implemented, and the SPA itself drifts from the contract on
three paths. Screens degrade to raw JSON-parse error banners instead of data.

**Found gaps** (HTTP 404 from the Vite proxy → serving):

| Web calls               | Contract says               | Serving has   | Fix                                                    |
| ----------------------- | --------------------------- | ------------- | ------------------------------------------------------ |
| `/api/v1/dashboard`     | `GET /dashboards/summary`   | nothing       | implement + fix web path                               |
| `/api/v1/graph`         | — (not specified)           | per-node only | add `GET /graph` (canvas view) + contract addition     |
| `/api/v1/review`        | `GET /reviews`              | `/reviews` ✅ | fix web path                                           |
| `/api/v1/timeline`      | `GET /timeline`             | nothing       | implement (derive from `amendment_event`)              |
| `/api/v1/notifications` | `GET /notifications` + read | nothing       | implement (needs migration 014, DATA-MODEL §8)         |
| `/api/v1/webhooks`      | `GET/POST/DELETE /webhooks` | nothing       | implement CRUD (tier-capped; DEC 19 egress validation) |

Out of scope (separate ask): `GET /search` REST mirror, `/documents/{corpus}/{id}`,
reports export, translate, corpora admin — screens either don't call them yet or degrade
cleanly.

## 1. Scope & exit (Definition of Done)

- **Outcome:** every sidebar screen of the running Web UI loads against the local stack
  without an error banner — live-verified by the Playwright walk-through; all four CI
  workflows green.
- **In scope:** migration 014 (notification · webhook_subscription · notification_delivery,
  DATA-MODEL §8); five serving endpoints + stores + tests; three web path/robustness fixes;
  API-CONTRACT addition for `GET /graph`.
- **Out of scope:** email/webhook **dispatch** (delivery pipeline is M-later; only the
  subscription CRUD + inbox read model ship), the deferred REST mirrors listed above.

## 2. Work breakdown

| Task  | Title                                                                                                | Size | Review | Risk |
| ----- | ---------------------------------------------------------------------------------------------------- | ---- | ------ | ---- |
| M10-1 | Migration 014: `notification` · `webhook_subscription` · `notification_delivery` (+ RLS/grants)      | S    | Medium | Low  |
| M10-2 | `GET /dashboards/summary` — aggregates over findings/reviews/`ingest.run`/registry                   | S    | Medium | Low  |
| M10-3 | `GET /graph` — tier-filtered whole-graph nodes+edges (capped) + API-CONTRACT §3 row                  | M    | Medium | Med  |
| M10-4 | `GET /timeline` — `amendment_event` range query + impacted downstream docs                           | S    | Medium | Low  |
| M10-5 | `GET /notifications` · `POST /notifications/{id}/read` · webhooks CRUD (URL egress validation)       | M    | Medium | Med  |
| M10-6 | Web: `/dashboard`→`/dashboards/summary` · `/review`→`/reviews` · non-JSON responses → readable error | XS   | Light  | Low  |
| M10-7 | Playwright re-run: all screens load with data/empty states; screenshot evidence                      | XS   | Light  | Low  |

## 3. Gates & risks

- **Gates:** `golangci-lint`/`gts` clean · unit + integration green · OpenAPI drift test
  regenerated · Playwright walk-through shows no error banner on any screen.
- **Risk — graph canvas size:** `GET /graph` on a large corpus set must cap nodes/edges
  (server-side limit + `truncated` flag) or the canvas renders unusably.
- **Risk — webhook egress (DEC 19):** subscription URLs are validated (scheme/host
  allowlist seam) even though dispatch isn't wired yet — the table must never hold a URL
  that would violate the egress policy later.
