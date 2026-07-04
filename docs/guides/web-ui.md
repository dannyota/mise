<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Web UI

The Vue 3.5 SPA provides the full product experience — dashboards, graph exploration, review,
Q&A, and administration. It **never calls a model** directly; all AI work is server-side.

## Screens

| Screen                    | Purpose                                                            |
| ------------------------- | ------------------------------------------------------------------ |
| **Dashboard**             | corpus health, finding counts by severity, recent activity         |
| **Graph Explorer**        | visual compliance chain (Vue Flow/elkjs), click-to-expand nodes    |
| **Review Workbench**      | pending candidates, promote/reject/relink with evidence side-panel |
| **Findings & Resolution** | finding list by kind/severity, disposition workflow, action plans  |
| **Q&A Chat**              | audit questions → cited answers (SSE streaming)                    |
| **Notifications**         | in-app inbox (finding events, overdue resolutions)                 |
| **Coverage Report**       | regulation → SOP chain with gaps highlighted                       |
| **Change Timeline**       | amendment impact view (which controls are now stale)               |
| **Admin**                 | corpus registry, user/role management, system health               |

## Running locally

```bash
cd apps/web
pnpm install
pnpm dev
# → http://localhost:5173
```

The dev server proxies API calls to the local serving process (`localhost:8080`).

## Access tiers in the UI

The UI respects the caller's access tier — screens show only data the user is cleared for.
Tier filtering is enforced by the API (RLS), not the frontend. The UI adapts by hiding
empty sections (e.g. a public-tier user sees no internal findings or review items).

## What's next

- [Operations](operations.md) — monitoring, backup, upgrades.
- [UI-DESIGN](/design/UI-DESIGN.md) — design principles and screen details.
