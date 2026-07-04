<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Build Plan

The thin index for the build plan. Detailed plan content lives under
[plan/](./plan/README.md): goals, phase map, reuse/change/new delta, cross-cutting build rules,
and per-milestone task plans.

See also:

- [plan/](./plan/README.md) (build-plan workspace)
- [ROADMAP](./ROADMAP.md) (milestones · critical path · exit gates)
- [RISKS](./RISKS.md) (delivery risk)
- [DECISIONS](./DECISIONS.md)
- [COST](./COST.md)

---

## 1. Plan Set

| Doc                                      | Owns                                                           |
| ---------------------------------------- | -------------------------------------------------------------- |
| [GOALS](./plan/GOALS.md)                 | product goals, non-goal, success shape                         |
| [PHASES](./plan/PHASES.md)               | phase → milestone map, design traceability                     |
| [DELTA](./plan/DELTA.md)                 | reuse · change · new delta from the banhmi/laksa engine        |
| [CROSS-CUTTING](./plan/CROSS-CUTTING.md) | rules every phase follows: delivery, governance, tests, review |
| [Milestone plans](./plan/README.md)      | per-milestone scope, DoD, workstreams, gates, and risks        |

---

## 2. How To Use

1. Read [GOALS](./plan/GOALS.md) for the outcome and boundary.
2. Read [PHASES](./plan/PHASES.md) with [ROADMAP](./ROADMAP.md) to understand sequence and exit
   gates.
3. Read [DELTA](./plan/DELTA.md) to separate reused engine work from mise-specific work.
4. Apply [CROSS-CUTTING](./plan/CROSS-CUTTING.md) to every milestone.
5. Execute from the current milestone folder under [plan/](./plan/README.md).
