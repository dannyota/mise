# Mise — Build Goals

The outcome the build plan serves. This file owns the product goals and the explicit non-goal;
phase sequencing is [PHASES](./PHASES.md), milestone detail is in the milestone folders.

See also:

- [PLAN](../PLAN.md) (build-plan index)
- [PHASES](./PHASES.md) (phase map)
- [ROADMAP](../ROADMAP.md) (critical path)
- [DECISIONS](../DECISIONS.md)
- [AI-GOVERNANCE](../../design/AI-GOVERNANCE.md)

---

## 1. Goals

1. Maintain **5 separate evidence-only corpora**, each homogeneous in structure, owner, and
   access tier (full banking regulation, not just the tech subset).
2. Use Vertex AI to **detect** relations, conflicts, coverage gaps, and staleness across corpora.
3. **Answer audit questions** with grounded, citation-backed, verified answers.
4. **Scale** to new corpora/scopes (reports, diagrams, circular feeds, ...) with no core change.

---

## 2. Non-Goal

mise never _asserts compliance_. It surfaces evidence, machine-proposed mappings, and verified
findings; humans promote/attest.
