# Mise — Docs

Design-doc set for mise. Grouped by concern; each doc has **one responsibility**
([DOC_STYLE.md](./DOC_STYLE.md)).

## 🧭 design/ — the product

- [ARCHITECTURE](./design/ARCHITECTURE.md) — components, read/write planes, storage, stack
- [DATA-MODEL](./design/DATA-MODEL.md) — schema, compliance graph, control-detection, findings
- [AI-GOVERNANCE](./design/AI-GOVERNANCE.md) — model gates, agent guardrails, AI audit
- [DATA-GOVERNANCE](./design/DATA-GOVERNANCE.md) — access tiers/RLS, data flow, HITL, data audit
- [UI-DESIGN](./design/UI-DESIGN.md) — Vue Web UI screens, stack, principles
- [API-CONTRACT](./design/API-CONTRACT.md) — REST + MCP + SSE surfaces, one generated contract

## 🛠️ engineering/ — how we build & run it

- [FOLDER_STRUCTURE](./engineering/FOLDER_STRUCTURE.md) — monorepo layout
- [TOOLCHAIN](./engineering/TOOLCHAIN.md) — versions, linters/formatters, Go-1.26 idioms
- [CODE_STYLE_GO](./engineering/CODE_STYLE_GO.md) ·
  [CODE_STYLE_TS](./engineering/CODE_STYLE_TS.md) ·
  [CODE_STYLE_VUE](./engineering/CODE_STYLE_VUE.md)
- [CI-CD](./engineering/CI-CD.md) — pipeline + supply chain (SAST · SCA · SBOM · sign)
- [TESTING](./engineering/TESTING.md) — pyramid, contract, eval, load
- [OBSERVABILITY](./engineering/OBSERVABILITY.md) — tracing, metrics, logs, SLOs
- [DELIVERY-MODEL](./engineering/DELIVERY-MODEL.md) — single-tenant per enterprise;
  upstream↔adopter split, onboarding, upgrades
- [DEPLOYMENT](./engineering/DEPLOYMENT.md) — GKE runtime (one cluster, no HA, scale-down, DR)
- [LOCAL-DEV](./engineering/LOCAL-DEV.md) — full stack on Podman, separated from GKE
- [LICENSES](./engineering/LICENSES.md) — tool/dep license inventory + CI license gate
- [THREAT-MODEL](./engineering/THREAT-MODEL.md) — assets, trust boundaries, STRIDE, residual risks

## 🗺️ project/ — direction

- [PLAN](./project/PLAN.md) — thin build-plan index
- [plan/](./project/plan/README.md) — build-plan workspace: goals, phases, delta, cross-cutting
  rules, milestone plans
- [ROADMAP](./project/ROADMAP.md) — milestones M0–M6, critical path, exit gates, review-load
- [RISKS](./project/RISKS.md) — delivery/execution risk register
- [DECISIONS](./project/DECISIONS.md) — the decision log (locked + open)
- [COST](./project/COST.md) — cost model

## 📚 meta

- [DOC_STYLE](./DOC_STYLE.md) — how these docs are written
