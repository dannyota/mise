# Mise — Phase Map

The nine build phases and the milestone each phase reaches. This file owns traceability from
phase to design docs; [ROADMAP](../ROADMAP.md) owns critical path, exit gates, and review load.

See also:

- [PLAN](../PLAN.md) (build-plan index)
- [ROADMAP](../ROADMAP.md) (critical path · exit gates)
- [CROSS-CUTTING](./CROSS-CUTTING.md) (rules every phase follows)
- [ARCHITECTURE](../../design/ARCHITECTURE.md)
- [DATA-MODEL](../../design/DATA-MODEL.md)

---

## 1. Phase → Milestone

Nine phases, each reaching one milestone and tracing to the design it implements. Task
breakdown, dependencies, exit criteria, and per-task sizing live in the linked milestone plan.

| Phase              | Milestone | Implements (design)                                                      | Milestone plan                                    |
| ------------------ | --------- | ------------------------------------------------------------------------ | ------------------------------------------------- |
| **0 Skeleton**     | M0        | ARCHITECTURE §2/§9/§10 · DATA-MODEL §1/§9 · FOLDER_STRUCTURE · LOCAL-DEV | [M0-skeleton](./M0-skeleton/README.md) — detailed |
| **1a Law ingest**  | M1a       | ARCHITECTURE §3 · DATA-MODEL §1/§2/§3 (public corpora)                   | [M1-ingest](./M1-ingest/README.md) WS1+4          |
| **1b Int. ingest** | M1b       | DATA-GOVERNANCE §2/§3 · DATA-MODEL §2 (internal corpora)                 | [M1-ingest](./M1-ingest/README.md) WS2+3          |
| **2 Graph spine**  | M2        | DATA-MODEL §4/§5/§2 · ARCHITECTURE §4 · API-CONTRACT §2/§3               | [M2-graph](./M2-graph/README.md)                  |
| **3 Detectors**    | M3        | DATA-MODEL §5/§6/§7/§8 · AI-GOVERNANCE §1/§4/§9 · ARCHITECTURE §5        | [M3-detectors](./M3-detectors/README.md)          |
| **4 Audit Q&A**    | M4        | ARCHITECTURE §6 · AI-GOVERNANCE §5/§8 · API-CONTRACT §1/§2/§4/§5         | [M4-audit-qa](./M4-audit-qa/README.md)            |
| **5 Web UI**       | M5        | UI-DESIGN (all) · API-CONTRACT §3/§4/§5 · DATA-MODEL §7/§10              | [M5-web-ui](./M5-web-ui/README.md)                |
| **6 Scale**        | M6        | ARCHITECTURE §8 · DATA-MODEL §9 · AI-GOVERNANCE §4/§9                    | [M6-scale](./M6-scale/README.md)                  |
| **7 Review**       | M7        | all design + engineering docs (consistency audit)                        | [M7-review](./M7-review/README.md)                |
| **8 Release**      | M8        | DELIVERY-MODEL · LICENSES · CI-CD §3                                     | [M8-release](./M8-release/README.md)              |

---

## 2. Detail Level

- **Every milestone (M0–M8) is detailed** — PR-sized tasks split across workstream files, in the
  milestone folders under [plan/](./README.md).
- [ROADMAP](../ROADMAP.md) decides sequence and exit gates; this file only maps phase to design.
