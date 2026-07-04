<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M6 · Workstream 2: Reports corpus

Land the first new corpus **kind** through the GA registry: a `report` (audit/risk) corpus that
ingests through the medallion and whose finding-nodes **map into the graph** — turning the compliance
graph into a **living audit map** — then prove it holds the eval floors. Rides Workstream 1's
descriptor + `graph_role`; no core change. Part of [Milestone M6](./README.md).

See also:

- [M6 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §8
- [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§6
- [TESTING](../../../engineering/TESTING.md) §5
- [DECISIONS](../../DECISIONS.md) 8/10/17
- Previous workstream: [Registry GA](./01-registry-ga.md)
- Next workstream: [Multimodal diagrams](./03-multimodal.md)

---

## 1. Tasks

Each row = one reviewable PR. New = net-new to mise. The `report` corpus is added **by descriptor**
(Workstream 1) — these tasks add the source plugin + the graph mapping, not core graph/detector code.

| ID    | Task                                                                                                                                                                              | Deliverable / done-when                                                                                                                                     | Design ref              | Depends on | Size | Review | Risk |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- | ---------- | ---- | ------ | ---- |
| M6-8  | **`report` corpus descriptor + ingest (new):** a reports (audit/risk) corpus ingests through the medallion via its source plugin; tier-tagged at write                            | a fixture report corpus ingests Discover→…→Index; rows are tier-tagged; the RLS suite stays green (DATA-MODEL §1)                                           | DATA-MODEL §1 · ARCH §3 | M6-1       | M    | Medium | Med  |
| M6-9  | **Reports→graph mapping — living audit map (new):** report finding-nodes map into `relation_edge` (finding → the obligation/policy it concerns) via the descriptor's `graph_role` | a report finding creates an edge to the obligation/policy it concerns; the graph reads as a living audit map; tier inherits stricter-of-two (DATA-MODEL §6) | DATA-MODEL §6 · ARCH §8 | M6-8, M6-3 | M    | Heavy  | Med  |
| M6-10 | **Reports eval pass (new):** reports retrieval + mapping scored against the golden set; floors hold with the new corpus present                                                   | the eval harness scores the reports corpus; retrieval/mapping floors do not regress with reports present (TESTING §5)                                       | TESTING §5              | M6-9       | S    | Medium | Med  |

---

## 2. Notes

- **The mapping is the value.** A report on its own is just another corpus; mapping its findings into
  `relation_edge` (M6-9) is what makes the graph a **living audit map** — an auditor can walk from a
  finding to the obligation/policy/SOP it touches (ARCHITECTURE §8, DATA-MODEL §6).
- **Reports ride the descriptor, not a new branch.** The corpus is declared by descriptor (M6-1) and
  joins the graph through its `graph_role` (M6-3); if M6-8/M6-9 needed a core graph/detector edit,
  the no-core-diff guard (M6-6) would fail — that is the point of sequencing GA first.
- **Mapping precision is graded, not assumed.** A finding mapped to the wrong obligation pollutes the
  audit map; ground the mapping in the same evidence discipline as M3's detectors and gate it in the
  reports eval ([RISKS](../../RISKS.md) R5/R7).
- **Confidential reports stay tier-tagged.** A confidential report finding inherits the stricter tier
  on its graph edge; the mandatory cross-tier-deny test covers the new finding/edge surface
  ([RISKS](../../RISKS.md) R2).
