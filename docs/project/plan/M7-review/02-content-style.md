<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# M7 WS2 — Content & Style

Decision-log consistency, diagram audit, schema/interface consistency, and doc-style compliance.

---

## Tasks

### M7-6: Decision log consistency

**Size** S · **Review** Medium · **Risk** Low

For every locked decision in DECISIONS.md, verify that every citing doc (design, engineering,
milestone plan) reflects the correct decision content and status. Flag any open decision cited as
locked, or any locked decision whose design-doc text contradicts the decision entry.

### M7-7: Diagram audit

**Size** S · **Review** Light · **Risk** Low

Render every mermaid diagram in the doc set. Verify: renders without error, colour conventions
(violet `#6d28d9` = AI/model, green = allowed, red = denied), no ASCII-art diagrams, no
hand-drawn boxes. Fix rendering issues and colour inconsistencies.

### M7-8: Schema & interface consistency

**Size** M · **Review** Heavy · **Risk** Med

Cross-check code fences (SQL migrations, Go interfaces, API schemas, JSON examples) across docs.
DATA-MODEL schema definitions must match the migration SQL in milestone plans; API-CONTRACT route
definitions must match the huma routes described in M4/M5; Go interface signatures in
ARCHITECTURE must match the canonical blocks in milestone plans. Fix discrepancies — the
milestone plan's canonical block is authoritative for implementation detail; the design doc is
authoritative for the contract.

### M7-9: Doc-style compliance & lint

**Size** S · **Review** Light · **Risk** Low

Run Prettier + markdownlint across the full doc set. Fix all errors. Verify H2
numbering/emoji convention (numbered OR emoji, never both; never on H1 except root README).
Verify prose stays under ~250 lines per doc (mermaid + code fences don't count). Split any doc
that has sprawled past its concern.
