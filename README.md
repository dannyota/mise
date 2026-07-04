<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# 🍱 mise

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)

Evidence-only **regulatory & policy intelligence** for **banks** — spanning
Vietnamese & Malaysian banking regulation and an institution's own internal control documents
(Group standards, Local policies, SOPs). mise serves **verbatim evidence and
machine-proposed, human-attested mappings**; it **never asserts compliance**. It is **open
source** and deployed **single-tenant — one instance inside each bank's own GCP project**,
bank-operated
(see [docs/engineering/DELIVERY-MODEL.md](docs/engineering/DELIVERY-MODEL.md)).

> **Why "mise"?** From _mise en place_ — the kitchen discipline of having every ingredient
> prepped, measured, and **in its place** before service. That's the product: verbatim
> regulatory evidence — cited, validity-checked, and access-scoped — staged and ready the
> moment it's asked for. Its two source knowledge bases keep the kitchen theme:
> **banhmi** (Vietnam regulation) and **laksa** (Malaysia regulation); mise is the line that
> brings them together and maps an institution's controls onto them.
>
> **Note:** unrelated to [jdx/`mise`](https://mise.jdx.dev), the polyglot dev-tool version
> manager — both just borrow the _mise en place_ metaphor.
>
> **Status: design stage.** This repo is the design-doc set — no application code yet.
> The deliverable is a clear, consistent, non-overlapping set of docs under [`docs/`](docs/).

## 🧭 What it does

- **Ingests 5 corpora** (VN & MY regulation + Group standards / Local policies / SOPs)
  into one AlloyDB instance, embedded into **one shared vector space**.
- **Builds a cross-corpus compliance graph** — which control _satisfies_ which authority,
  which policy _implements_ which standard — with every AI proposal gated by a **grounding
  check** and **human attestation**.
- **Answers audit questions** with **cited, grounded** answers via a backend Claude agent.
  The read path is evidence-only and fast (Go + AlloyDB); reasoning runs server-side.

## 📚 Docs

Grouped by folder (full navigator: [docs/README.md](docs/README.md)).

**Design** — `docs/design/`

| Doc                                               | Purpose                                                                                   |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| [ARCHITECTURE](docs/design/ARCHITECTURE.md)       | System design — components, the read/write planes, storage, tech stack                    |
| [DATA-MODEL](docs/design/DATA-MODEL.md)           | The schema — corpora, metadata, the compliance graph, control-detection, findings         |
| [AI-GOVERNANCE](docs/design/AI-GOVERNANCE.md)     | Governing the **models** — control gates, agent guardrails, model risk/approval, AI audit |
| [DATA-GOVERNANCE](docs/design/DATA-GOVERNANCE.md) | Governing the **data** — access tiers/RLS, data flow, human-in-the-loop, data audit       |
| [UI-DESIGN](docs/design/UI-DESIGN.md)             | The Vue Web UI — screens, stack, design principles                                        |
| [API-CONTRACT](docs/design/API-CONTRACT.md)       | REST + MCP tools + SSE surfaces; one generated contract                                   |

**Engineering** — `docs/engineering/`

| Doc                                                                                                                                          | Purpose                                                                                                 |
| -------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| [FOLDER_STRUCTURE](docs/engineering/FOLDER_STRUCTURE.md)                                                                                     | Repo layout (monorepo) — read before scaffolding                                                        |
| [TOOLCHAIN](docs/engineering/TOOLCHAIN.md)                                                                                                   | Pinned versions · golangci-lint v2 + Go-1.26 idiom modernization · pre-commit                           |
| [CODE_STYLE_GO](docs/engineering/CODE_STYLE_GO.md) · [\_TS](docs/engineering/CODE_STYLE_TS.md) · [\_VUE](docs/engineering/CODE_STYLE_VUE.md) | Per-language code style (Go · TS · Vue)                                                                 |
| [CI-CD](docs/engineering/CI-CD.md)                                                                                                           | Build/release pipeline + supply-chain (SAST · SCA · SBOM · scan · sign)                                 |
| [TESTING](docs/engineering/TESTING.md)                                                                                                       | Test pyramid · MCP contract test · eval golden-set · load                                               |
| [OBSERVABILITY](docs/engineering/OBSERVABILITY.md)                                                                                           | Tracing · metrics · logs · SLOs (vs the audit trail)                                                    |
| [DELIVERY-MODEL](docs/engineering/DELIVERY-MODEL.md)                                                                                         | Delivery & operating model — single-tenant per enterprise, upstream↔adopter split, onboarding, upgrades |
| [DEPLOYMENT](docs/engineering/DEPLOYMENT.md)                                                                                                 | GKE deploy (runtime) — one cluster, no HA, scale-down, backup/DR, ingest connector ops                  |
| [LOCAL-DEV](docs/engineering/LOCAL-DEV.md)                                                                                                   | Run the full stack locally on Podman — separated from GKE                                               |
| [LICENSES](docs/engineering/LICENSES.md)                                                                                                     | Tool/dep license inventory + the CI license gate (permissive-only)                                      |
| [THREAT-MODEL](docs/engineering/THREAT-MODEL.md)                                                                                             | Assets · trust boundaries · STRIDE · residual risks (design-level)                                      |

**Project** — `docs/project/`

| Doc                                    | Purpose                                                                           |
| -------------------------------------- | --------------------------------------------------------------------------------- |
| [PLAN](docs/project/PLAN.md)           | Thin build-plan index                                                             |
| [plan/](docs/project/plan/README.md)   | Build-plan workspace — goals, phases, delta, cross-cutting rules, milestone plans |
| [ROADMAP](docs/project/ROADMAP.md)     | Milestones M0–M8, critical path, exit gates, review-load                          |
| [RISKS](docs/project/RISKS.md)         | Delivery/execution risk register (vs the security THREAT-MODEL)                   |
| [DECISIONS](docs/project/DECISIONS.md) | The decision log — locked + open                                                  |
| [COST](docs/project/COST.md)           | The cost model — unit rates, one-time build, recurring                            |

**Meta:** [DOC_STYLE](docs/DOC_STYLE.md) — how these docs are written.

## ⚙️ At a glance

- **Backend:** Go + Temporal (self-hosted on GKE).
- **Store:** **AlloyDB Omni** (ScaNN index) on GKE — one instance, schema-per-corpus +
  graph schema, RLS per access tier.
- **AI (all on Vertex):** **write path** = Gemini 3.5 Flash + Doc AI + embeddings +
  Check Grounding; **serve path** = Claude Haiku 4.5 / Sonnet 4.6 via the **Claude Agent
  SDK** (backend reasoning endpoint, read-only, never in the browser).
- **Web:** **Vue 3.5** SPA (Vite + Tailwind 4).
- **Deploy:** **single-tenant — one instance per enterprise, in the bank's own GCP project**
  (bank-operated); one GKE cluster, no HA, auto scale-down.

See [docs/design/ARCHITECTURE.md](docs/design/ARCHITECTURE.md) for the full picture.

## 🔒 Security

Security posture and how to report a vulnerability: [SECURITY.md](SECURITY.md).

## ⚖️ License

Copyright © 2026 Danny <danh.software@outlook.com>

mise is licensed under the **GNU Affero General Public License v3.0 or later**
(AGPL-3.0) — see [LICENSE](LICENSE).

As the **sole copyright holder**, the author may also offer mise under other terms — a
**commercial / proprietary license** (e.g. to deploy without AGPL obligations) is
available on request. **This project does not accept external contributions** (no pull
requests or patches are merged), so the author keeps full copyright and this
dual-licensing always remains possible. See [CONTRIBUTING.md](CONTRIBUTING.md).
