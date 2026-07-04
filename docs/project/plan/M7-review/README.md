<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Milestone M7: Design Review

A **quality gate before release.** The design-doc set (M0–M6) is feature-complete; this milestone
audits it for consistency, completeness, correctness, and cross-reference integrity — so the
published v0.1.0 source is a clean, trustworthy spec an enterprise can evaluate and build from.

This is a **review-only milestone — no new design surface.** It produces fixes, not features.
Production hardening, threshold calibration, internal connectors (M1b), and security hardening are
deferred to a future milestone.

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [RISKS](../../RISKS.md)
- [DOC_STYLE](../../../DOC_STYLE.md)
- Previous: [Milestone M6](../M6-scale/README.md)
- Next: [Milestone M8](../M8-release/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** every design doc, engineering doc, and project doc is **internally consistent**,
  **cross-referenced correctly**, and **lint-clean** — the doc set reads as one coherent spec, not a
  collection of milestones.
- **Exit criteria (DoD):**
  - **Cross-reference audit** — every `§N` reference, `[NAME](./path)` link, and `DECISIONS N`
    citation resolves to the correct target; no dead links, no stale section numbers.
  - **Ownership-table compliance** — content lives in the doc that owns the topic (per the
    `CLAUDE.md` ownership table); duplicated content is removed and replaced with a cross-reference.
  - **Terminology consistency** — key terms (corpus, tier, access_tier, evidence, finding, edge,
    promoted, attested, descriptor, etc.) are used uniformly across docs; no synonyms or ambiguities.
  - **Customer-neutral** — no real bank, customer, or host name anywhere; all examples use the
    reference deployment with placeholders.
  - **Doc style compliance** — every doc passes DOC_STYLE rules: H2 numbering/emoji convention,
    voice, format, line-length prose target (≤ ~250 lines).
  - **Lint clean** — `prettier --write` + `markdownlint-cli2` report **zero errors** across the
    full doc set.
  - **Decision log consistency** — every locked decision cited in a design/engineering doc matches
    its entry in DECISIONS.md (number, status, content); open decisions are flagged as open.
  - **Diagram audit** — every mermaid diagram renders correctly; colour conventions (violet AI,
    green allowed, red denied) are consistent; no ASCII-art diagrams remain.
  - **Schema/interface consistency** — code fences showing schemas, SQL, Go interfaces, or API
    shapes are consistent with each other across docs (e.g. DATA-MODEL §5 matches the
    migration SQL in M3's plan; API-CONTRACT matches the huma routes in M4).
  - **README doc index** — `docs/README.md` and root `README.md` list every doc, every link
    resolves, and the one-line descriptions are accurate.

---

## 2. Workstreams

Two reviewable workstreams, each a single pass across the doc set from different angles.

| #   | Workstream                                            | Tasks       | Theme                                                         |
| --- | ----------------------------------------------------- | ----------- | ------------------------------------------------------------- |
| 1   | [Cross-reference & structure](./01-xref-structure.md) | M7-1 … M7-5 | links, ownership, terminology, customer-neutral, README index |
| 2   | [Content & style](./02-content-style.md)              | M7-6 … M7-9 | decisions, diagrams, schemas, doc-style compliance, lint      |

---

## 3. Test gates (this milestone)

- **Lint:** Prettier (`--check`) + markdownlint-cli2 — zero errors across the full doc set.
- **Link validation:** every internal `[text](path)` link resolves; every `§N` reference matches a
  real H2 number in the target doc.
- **No new content:** the diff is fixes only — no new design surface, no new decisions, no new
  milestones beyond M7/M8 themselves.

---

## 4. Risks (this milestone)

- **Scope creep into design changes** — the review may surface design gaps or inconsistencies that
  tempt a design change; the rule is **fix the doc to match the settled design, or flag the
  discrepancy for a future milestone** — never silently redesign during review.
- **Stale cross-references compound** — fixing one section number can cascade through multiple docs;
  batch related fixes and re-lint after each batch.
