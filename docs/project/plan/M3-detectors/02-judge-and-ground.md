# Mise — M3 · Workstream 2: Judge & ground

Turn Method-B candidate pairs into grounded, auditable `satisfies` candidates: Gemini judge,
Check Grounding, threshold gating, candidate writes, and the RelationDetect workflow. Part of
[Milestone M3](./README.md).

See also:

- [M3 overview](./README.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §5
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §1/§4/§9
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §5
- [DECISIONS](../../DECISIONS.md) 10/11/17
- Previous workstream: [Findings & candidate pipeline](./01-findings-and-candidates.md)
- Next workstream: [Detectors](./03-detectors.md)

---

## 1. Tasks

Each row = one reviewable PR. DEC 11 is locked (provisional) — model escalation and thresholds
ship with defaults and are calibrated against the golden set before final lock.

| ID    | Task                                                                                                                            | Deliverable / done-when                                                                                            | Design ref                   | Depends on | Size | Review | Risk |
| ----- | ------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | ---------------------------- | ---------- | ---- | ------ | ---- |
| M3-6  | **Judge seam + prompt contract (new):** define the structured judge interface and prompt/hash record for `satisfies` candidates | fake and real judge adapters share one schema; prompt hash/model id captured for every call                        | AI-GOV §4/§9 · DATA-MODEL §5 | M3-5       | M    | Heavy  | High |
| M3-7  | **Gemini judge adapter (new):** run candidate pairs through the configured Gemini judge on the adopter's Vertex project         | `VERTEX=real` returns edge type, confidence, rationale, and quoted spans; `VERTEX=fake` covers CI                  | AI-GOV §4 · DEC 10/17        | M3-6       | M    | Heavy  | High |
| M3-8  | **Check Grounding gate (new):** verify the judge rationale is entailed by both verbatim source texts                            | ungrounded candidate is discarded; grounded candidate carries grounding score and quoted spans                     | AI-GOV §4                    | M3-7       | M    | Heavy  | High |
| M3-9  | **Threshold config + audit logging (new):** version confidence/grounding thresholds and model routing settings                  | thresholds logged with run id; DEC 11 calibration inputs are visible to eval and review                            | DEC 11 · AI-GOV §9           | M3-8       | S    | Heavy  | Med  |
| M3-10 | **Candidate edge/evidence write (new):** persist only grounded candidates as `promoted=false` with full audit evidence          | `relation_edge`/`relation_evidence` rows include run id, model, prompt hash, confidence, grounding, and created_by | DATA-MODEL §4/§5             | M3-9       | M    | Heavy  | High |
| M3-11 | **RelationDetect workflow (new):** orchestrate embed→ANN→rerank→judge→ground→write as a Temporal workflow                       | deterministic workflow test green with `testing/synctest`; retries/backoff do not duplicate candidate edges        | ARCH §5 · TESTING §2         | M3-10      | M    | Heavy  | High |

---

## 2. Notes

- **AI proposes; grounding gates; humans attest.** This workstream writes candidates only
  (`promoted=false`). Nothing becomes accepted evidence without the review path in workstream 4.
- **Thresholds are logged and versioned (DEC 11 provisional).** The implementation must make
  calibration observable; the provisional values are confirmed only after eval.
- **Managed AI is the reference deployment, not an upstream service.** Candidate texts are sent to
  the adopter's Vertex project by default (DEC 10/17), or to the self-hosted seam if policy selects
  that variant.
- **The workflow owns idempotence.** Retries can rerun judge/ground activities, but they must not
  duplicate candidates or evidence rows.
