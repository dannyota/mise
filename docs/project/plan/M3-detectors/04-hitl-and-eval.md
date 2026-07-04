# Mise — M3 · Workstream 4: HITL & eval

Expose the review queue and findings API, support promote/reject/relink with recomputation, seed
the bootstrap golden set, and run the first cross-corpus mapping eval. Part of
[Milestone M3](./README.md).

See also:

- [M3 overview](./README.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §4/§5/§6/§7
- [API-CONTRACT](../../../design/API-CONTRACT.md) §4/§5
- [TESTING](../../../engineering/TESTING.md) §5
- [DECISIONS](../../DECISIONS.md) 11/18
- Previous workstream: [Detectors](./03-detectors.md)

---

## 1. Tasks

Each row = one reviewable PR. Review APIs write human-attested evidence; UI screens arrive in M5.

| ID    | Task                                                                                                                          | Deliverable / done-when                                                                                          | Design ref                      | Depends on   | Size | Review | Risk |
| ----- | ----------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | ------------------------------- | ------------ | ---- | ------ | ---- |
| M3-16 | **Review queue + findings API (new):** `GET /reviews`, promote/reject/relink, `GET /findings`, and resolution write endpoints | contract tests green; promote writes `human_attested` evidence; reject/resolution state is durable and auditable | API-CONTRACT §3 · DATA-MODEL §7 | M3-10, M3-15 | L    | Heavy  | High |
| M3-17 | **Bootstrap golden set (new):** curate the initial VN/Malay `satisfies` examples and negative cases for mapping eval          | seed set versioned with provenance and reviewer notes; DEC 18 has enough data to produce the first baseline      | TESTING §5 · DEC 18             | M3-13        | M    | Heavy  | Med  |
| M3-18 | **Mapping eval + threshold calibration (new):** run precision/recall eval, report baseline, and calibrate judge thresholds    | mapping precision/recall emitted; first run sets baseline; DEC 11 threshold settings recorded for M3 exit        | TESTING §5 · DEC 11/18          | M3-16, M3-17 | M    | Heavy  | High |

---

## 2. Notes

- **M3-16 is an API milestone, not the UI.** It creates the write surface that M5's Review
  Workbench consumes later.
- **Relink is a recompute trigger.** A corrected target must rerun detection with the correction as
  constraint and recompute dependent findings for affected nodes only.
- **DEC 18 closes through evidence, not opinion.** The bootstrap set needs provenance and review
  notes because it becomes the first baseline for a metric the old engine did not have.
- **The first mapping score is not a pass/fail gate.** It establishes baseline quality; inherited
  retrieval/citation/current-law floors still gate.
