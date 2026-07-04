<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->
<!-- markdownlint-disable MD041 MD013 -->

<div align="center">

<a href="https://mise.danny.vn"><img src="docs/assets/banner.svg" alt="mise — evidence-only regulatory intelligence for banks" width="600"></a>

# 🍱 mise

**Evidence-only regulatory & policy intelligence for banks.**

[Docs](https://mise.danny.vn) · [Guides](https://mise.danny.vn/#/guides/overview) · [Decisions](docs/project/DECISIONS.md) · [Changelog](CHANGELOG.md)

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)

</div>

---

Ingest public law (Vietnam, Malaysia) and an institution's internal controls into **one
shared vector space**, build a **compliance graph** with AI-proposed, **human-attested**
edges, detect gaps/conflicts/staleness, and answer audit questions with **cited, grounded
evidence**. mise **never asserts compliance** — it serves verbatim evidence only.

> **Status: design stage.** This repo is the design-doc set — no application code yet.
> The deliverable is a clear, consistent, non-overlapping set of docs under [`docs/`](docs/).

## At a glance

| Layer      | Stack                                                                               |
| ---------- | ----------------------------------------------------------------------------------- |
| **Serve**  | Go + AlloyDB Omni (ScaNN) — fast, evidence-only read path                           |
| **Reason** | Claude Agent SDK (Haiku 4.5 / Sonnet 4.6 on Vertex) — server-side, never in browser |
| **Ingest** | Temporal workers + Gemini 3.5 Flash + Doc AI + Check Grounding                      |
| **Web**    | Vue 3.5 SPA (Vite + Tailwind 4)                                                     |
| **Deploy** | Single-tenant GKE — one instance per bank, bank-operated, scale-to-zero             |

## Docs

Full navigator: [mise.danny.vn](https://mise.danny.vn) · [docs/README.md](docs/README.md)

| Folder                                   | Contents                                              |
| ---------------------------------------- | ----------------------------------------------------- |
| [`docs/guides/`](docs/guides/)           | Deploy, ingest, graph, Q&A, UI, operations            |
| [`docs/design/`](docs/design/)           | Architecture, data model, AI/data governance, UI, API |
| [`docs/engineering/`](docs/engineering/) | Toolchain, CI/CD, testing, deployment, threat model   |
| [`docs/project/`](docs/project/)         | Roadmap (M0–M8 ✅), decisions, risks, cost            |

## Why "mise"?

From _mise en place_ — the kitchen discipline of having every ingredient prepped, measured,
and **in its place** before service. That's the product: verbatim regulatory evidence —
cited, validity-checked, and access-scoped — staged and ready the moment it's asked for.
Its source knowledge bases keep the kitchen theme: **banhmi** (Vietnam) and **laksa**
(Malaysia); mise is the line that brings them together.

> Unrelated to [jdx/`mise`](https://mise.jdx.dev) — both borrow the _mise en place_ metaphor.

## License

Copyright © 2026 Danny <danh.software@outlook.com>

**AGPL-3.0-only** — see [LICENSE](LICENSE). Dual-license (commercial/proprietary) available
on request. No external contributions accepted ([CONTRIBUTING.md](CONTRIBUTING.md)).
