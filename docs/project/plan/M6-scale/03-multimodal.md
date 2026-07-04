<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M6 · Workstream 3: Multimodal diagrams

Land the second new kind: a **multimodal `diagram`** path that verbalizes figures/images — Layout
Parser extracts, Gemini-vision captions, the **caption text** embeds @1536-d, the **image ref** is
kept for human verification. No native image vector ever enters the locked space. Part of
[Milestone M6](./README.md).

See also:

- [M6 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §8
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §4/§7/§9
- [DATA-MODEL](../../../design/DATA-MODEL.md) §2/§9
- [TESTING](../../../engineering/TESTING.md) §5
- [DECISIONS](../../DECISIONS.md) 1/10/17
- Previous workstream: [Reports corpus](./02-reports-corpus.md)

---

## 1. Tasks

Each row = one reviewable PR. New = net-new to mise. The caption seam is split from the real adapter
(M6-11 vs M6-12) so the offline Mode-B path lands first and CI never needs GCP to exercise it.

| ID    | Task                                                                                                                                                                                                                   | Deliverable / done-when                                                                                                                                                          | Design ref                     | Depends on | Size | Review | Risk |
| ----- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------ | ---------- | ---- | ------ | ---- |
| M6-11 | **Gemini-vision caption seam — interface + fake (new):** the figure-caption stage behind a Go interface + a deterministic fake; the model record is captured                                                           | the caption stage sits behind an interface; the fake returns canned captions; an offline run needs no GCP (Mode B); the model record (model · prompt · ts) is logged (AI-GOV §9) | AI-GOV §9 · LOCAL-DEV §4       | M6-1       | S    | Medium | Low  |
| M6-12 | **Diagram verbalize — real adapter + caption embed (new):** Layout Parser extracts figures/tables; Gemini-vision captions on the **bank's own** Vertex; the **caption text** embeds @1536-d; the **image ref** is kept | `VERTEX=real` captions a diagram, embeds the caption text in the locked space, keeps the image ref; **no** native image vector enters the space (DEC 1, ARCH §8)                 | ARCH §8 · AI-GOV §4/§7 · DEC 1 | M6-11      | M    | Heavy  | High |
| M6-13 | **`diagram` retrieval wiring + caption eval (new):** caption chunks are retrievable in the shared space; caption retrieval scored against the golden set; the image ref is surfaced for human verification             | a query retrieves the caption chunk with its image ref; caption retrieval scores against the golden set; floors hold (TESTING §5)                                                | DATA-MODEL §2 · TESTING §5     | M6-12      | M    | Medium | Med  |

---

## 2. Notes

- **Caption text, never an image vector.** The single-space invariant (DECISIONS 1) is preserved by
  embedding the **verbalized caption** in the locked 1536-d space and barring native image vectors —
  the validator (M6-2) refuses them at registration ([RISKS](../../RISKS.md) R12, ARCHITECTURE §8).
- **Vision runs on the bank's own Vertex by default.** Internal diagrams are confidential content
  sent to Gemini-vision on the **bank's own** GCP Vertex (DECISIONS 10/17); the caption seam (M6-11)
  keeps a stricter adopter policy a swap to the self-hosted variant (AI-GOVERNANCE §7), not a rebuild
  ([RISKS](../../RISKS.md) R1).
- **Keep the image ref — captions can lie.** A vision caption can misdescribe a figure, so the image
  ref is kept for human verification, the full model record is carried (AI-GOVERNANCE §9), and
  caption retrieval is scored against the golden set rather than trusted blind
  ([RISKS](../../RISKS.md) R5).
- **The seam/adapter split keeps CI offline** — the fake (M6-11) lands the Mode-B path first so CI
  exercises the caption stage without a GCP call (LOCAL-DEV §4), exactly as the M1 Doc AI parse seam
  did.
