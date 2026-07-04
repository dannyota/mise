<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Build Plan Workspace

The build-plan workspace. It owns the focused plan docs plus the detailed milestone plans, one
subfolder per milestone (M0–M8, matching [ROADMAP](../ROADMAP.md)).

See also:

- [PLAN](../PLAN.md) (thin build index)
- [ROADMAP](../ROADMAP.md) (milestones · critical path · exit gates)
- [RISKS](../RISKS.md) (delivery risk)
- [DECISIONS](../DECISIONS.md)

---

## 1. Core Plan Docs

| Doc                                 | Owns                                                    |
| ----------------------------------- | ------------------------------------------------------- |
| [GOALS](./GOALS.md)                 | goals, non-goal, product outcome                        |
| [PHASES](./PHASES.md)               | phase → milestone map, design traceability              |
| [DELTA](./DELTA.md)                 | reuse · change · new delta from the banhmi/laksa engine |
| [CROSS-CUTTING](./CROSS-CUTTING.md) | build-wide delivery, governance, test, and review rules |

---

## 2. Milestone Layout

All milestones are detailed to the same depth — an overview `README.md` plus workstream files
with PR-sized tasks.

| Milestone | Folder                                                  | State                                                           |
| --------- | ------------------------------------------------------- | --------------------------------------------------------------- |
| **M0**    | [M0-skeleton/](./M0-skeleton/README.md)                 | **detailed** — overview + 4 workstreams, 19 tasks               |
| **M1**    | [M1-ingest/](./M1-ingest/README.md)                     | **detailed** — overview + 4 workstreams, 24 tasks               |
| **M2**    | [M2-graph/](./M2-graph/README.md)                       | **detailed** — overview + 3 workstreams, 15 tasks               |
| **M3**    | [M3-detectors/](./M3-detectors/README.md)               | **detailed** — overview + 4 workstreams, 18 tasks (crown-jewel) |
| **M4**    | [M4-audit-qa/](./M4-audit-qa/README.md)                 | **detailed** — overview + 4 workstreams, 23 tasks               |
| **M5**    | [M5-web-ui/](./M5-web-ui/README.md)                     | **detailed** — overview + 4 workstreams, 20 tasks               |
| **M6**    | [M6-scale/](./M6-scale/README.md)                       | **detailed** — overview + 3 workstreams, 13 tasks               |
| **M7**    | [M7-review/](./M7-review/README.md)                     | **detailed** — overview + 2 workstreams, 9 tasks (review-only)  |
| **M8**    | [M8-release/](./M8-release/README.md)                   | **detailed** — overview + 2 workstreams, 8 tasks (v0.1.0)       |
| **M9**    | [M9-internal-sources/](./M9-internal-sources/README.md) | **detailed** — single workstream, 4 tasks (library connector)   |
| **M10**   | [M10-rest-completion/](./M10-rest-completion/README.md) | **detailed** — single workstream, 7 tasks (Web UI REST surface) |

---

## 3. How A Milestone Is Planned

- **A milestone folder = one milestone from [ROADMAP](../ROADMAP.md).** Its `README.md` owns scope +
  exit (DoD), dependencies + gates, the work breakdown, test gates, and risks.
- **Every milestone is detailed to the same shape:** the work is split across **workstream files**
  (e.g. M0's [01-foundation](./M0-skeleton/01-foundation.md) …
  [04-seams-and-local-stack](./M0-skeleton/04-seams-and-local-stack.md)), so no single file owns more
  than one concern. Tasks are **PR-sized** and dependency-ordered, each sized **Size** (XS–L) ·
  **Review** (Light/Medium/Heavy) · **Risk** (Low/Med/High). No dates — the constraint is review, not
  typing ([CROSS-CUTTING](./CROSS-CUTTING.md) §2).
- **Re-pace at each milestone boundary:** firm up sequencing and re-score [RISKS](../RISKS.md) §4
  before starting a milestone — the plan is detailed, but the order is paced by review capacity.
- **Don't duplicate:** risks point to [RISKS](../RISKS.md); decisions to [DECISIONS](../DECISIONS.md);
  design to the owning design doc. The milestone plan references, it doesn't restate.
