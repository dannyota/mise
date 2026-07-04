<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# mise

Evidence-only **regulatory & policy intelligence** for banks — ingest public law
(Vietnam, Malaysia) and internal controls, build a **compliance graph** with
AI-proposed, human-attested edges, detect gaps/conflicts/staleness, and answer
audit questions with **cited, grounded evidence**. mise never asserts compliance.

> Single-tenant. Bank-operated. Open source (AGPL-3.0).

New here? **[Overview](guides/overview.md) &rarr;
[Deploy](guides/deploy.md).**
Building it? **[Architecture](design/ARCHITECTURE.md).**

## At a glance

| Layer      | Stack                                                                  |
| ---------- | ---------------------------------------------------------------------- |
| **Serve**  | Go + AlloyDB Omni (ScaNN) — fast, evidence-only read path              |
| **Reason** | Claude Agent SDK (Haiku 4.5 / Sonnet 4.6 on Vertex) — server-side only |
| **Ingest** | Temporal workers + Gemini 3.5 Flash + Doc AI + Check Grounding         |
| **Web**    | Vue 3.5 SPA (Vite + Tailwind 4)                                        |
| **Deploy** | Single-tenant GKE — one instance per bank, scale-to-zero               |

## Guides

- [Overview](guides/overview.md) — what mise does
- [Deploy](guides/deploy.md) — local dev (Podman) or production (GKE)
- [First corpus](guides/first-corpus.md) — ingest public law, no credentials needed
- [Graph & detectors](guides/graph-detectors.md) — build the compliance graph
- [Audit Q&A](guides/audit-qa.md) — cited answers via Claude agent
- [Web UI](guides/web-ui.md) — the full product in a browser
- [Operations](guides/operations.md) — monitoring, backup, upgrades

## Design

- [Architecture](design/ARCHITECTURE.md) — components, read/write planes, storage
- [Data model](design/DATA-MODEL.md) — schema, compliance graph, findings
- [AI governance](design/AI-GOVERNANCE.md) — model gates, agent guardrails
- [Data governance](design/DATA-GOVERNANCE.md) — access tiers, RLS, HITL
- [UI design](design/UI-DESIGN.md) — screens, stack, principles
- [API contract](design/API-CONTRACT.md) — REST + MCP + SSE

## Engineering

- [Toolchain](engineering/TOOLCHAIN.md) — Go 1.26, pnpm 11, golangci-lint v2
- [Folder structure](engineering/FOLDER_STRUCTURE.md) — monorepo layout
- [CI/CD](engineering/CI-CD.md) — pipeline + supply chain
- [Testing](engineering/TESTING.md) — pyramid, contract, eval, load
- [Deployment](engineering/DEPLOYMENT.md) — GKE runtime
- [Local dev](engineering/LOCAL-DEV.md) — Podman stack
- [Delivery model](engineering/DELIVERY-MODEL.md) — tenancy, upgrades
- [Observability](engineering/OBSERVABILITY.md) — tracing, metrics, SLOs
- [Threat model](engineering/THREAT-MODEL.md) — STRIDE, trust boundaries
- [Licenses](engineering/LICENSES.md) — dep inventory + CI gate

## Project

- [Roadmap](project/ROADMAP.md) — M0–M8 (all ✅)
- [Decisions](project/DECISIONS.md) — locked + open
- [Risks](project/RISKS.md) — delivery risk register
- [Cost](project/COST.md) — unit rates, build, recurring

## Meta

- [Doc style](DOC_STYLE.md) — how these docs are written
- [Changelog](/CHANGELOG.md)
