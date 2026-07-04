# Mise — M3 · Workstream 3: Detectors

Build the four cross-corpus detectors on top of the graph spine: transitive `covers`,
ConflictDetect, GapScan, and StaleScan. Part of [Milestone M3](./README.md).

See also:

- [M3 overview](./README.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §5/§6
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §4
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §5
- [DECISIONS](../../DECISIONS.md) 7
- Previous workstream: [Judge & ground](./02-judge-and-ground.md)
- Next workstream: [HITL & eval](./04-hitl-and-eval.md)

---

## 1. Tasks

Each row = one reviewable PR. Detectors may propose findings; none auto-promote evidence.

| ID    | Task                                                                                                                  | Deliverable / done-when                                                                                          | Design ref                         | Depends on  | Size | Review | Risk |
| ----- | --------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | ---------------------------------- | ----------- | ---- | ------ | ---- |
| M3-12 | **Transitive `covers` computation (new):** compute SOP→law coverage through promoted SOP→Policy→Group→law chains only | no direct `local-sop → law` judge call exists; test asserts DEC 7 transitive-only behavior                       | DATA-MODEL §5 · DEC 7/29           | M3-11       | M    | Heavy  | High |
| M3-13 | **ConflictDetect (new):** detect grounded contradictions between Group standard and VN/MY law verbatim texts          | conflict finding raised with quoted spans from both texts; contradiction check is grounded, not translation-only | DATA-MODEL §6 · AI-GOV §4 · DEC 28 | M3-11       | L    | Heavy  | High |
| M3-14 | **GapScan cron (new):** raise gap findings for obligations/standards/policies with missing downstream coverage        | scheduled scan raises deduped gap findings for 0 `satisfies`, 0 `implements`, or 0 SOP coverage cases            | DATA-MODEL §6 · ARCH §5            | M3-12       | M    | Heavy  | Med  |
| M3-15 | **StaleScan on amendment events (new):** cascade staleness findings when law amendments outdate downstream controls   | `amendment_event.date > downstream effective_date` raises staleness findings through policy/SOP descendants      | DATA-MODEL §3/§6                   | M3-12, M1-8 | M    | Heavy  | High |

---

## 2. Notes

- **`covers` is computed, not judged.** DEC 7 prevents a direct SOP→law model path; this keeps the
  explanation traceable through policy and group standard hops.
- **ConflictDetect is the highest-risk detector.** It must ground the contradiction in both
  verbatim texts and avoid treating translation artifacts as evidence.
- **Gap and stale findings are review inputs.** They create work for humans and downstream action
  plans; they do not update the graph by themselves.
- **Scans must be bounded and deduped.** A single law amendment can cascade; M3-15 should recompute
  only affected descendants and avoid duplicate findings on rerun.
