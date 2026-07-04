<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Cross-Cutting Build Rules

Rules every milestone follows. This file owns build-wide constraints; milestone-specific scope,
tasks, and risks stay in each milestone folder.

See also:

- [PLAN](../PLAN.md) (build-plan index)
- [DELIVERY-MODEL](../../engineering/DELIVERY-MODEL.md)
- [AI-GOVERNANCE](../../design/AI-GOVERNANCE.md)
- [DATA-GOVERNANCE](../../design/DATA-GOVERNANCE.md)
- [TESTING](../../engineering/TESTING.md)
- [TOOLCHAIN](../../engineering/TOOLCHAIN.md)
- [CI-CD](../../engineering/CI-CD.md)
- [OBSERVABILITY](../../engineering/OBSERVABILITY.md)
- [API-CONTRACT](../../design/API-CONTRACT.md)

---

## 1. Rules

- **Delivery:** open-source, single-tenant, self-hosted per enterprise (DELIVERY-MODEL).
- **Evidence-only + grounding gates:** AI-GOVERNANCE §1/§4.
- **Access control:** RLS at the database boundary (DATA-GOVERNANCE §2).
- **Audit:** data events in DATA-GOVERNANCE §6; model turns in AI-GOVERNANCE §9.
- **Evals:** mapping precision/recall + retrieval recall@k vs a golden set (TESTING §5).
- **Engineering standards:** pinned toolchains, CI gates, supply-chain checks, tracing/SLOs, the
  test pyramid, and one generated API/MCP contract.

---

## 2. Task Shape

AI agents implement; a solo part-time human directs and reviews, so the constraint is **review**,
not typing. Every task is:

- one reviewable PR.
- sized by **Size** (XS-L), **Review** (Light/Medium/Heavy), and **Risk** (Low/Med/High).
- ordered by dependency, not calendar date.

No person-days and no artificial dates. Re-pace at each milestone boundary against
[RISKS](../RISKS.md) §4.
