<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M1 · Workstream 3: Metadata & ownership

Turn parsed internal documents into the durable control envelope: signer, approver, current owner,
validity/version fields, citation paths, `org_role` ownership, leaver-safe history, and access-tier
stamping. Part of [Milestone M1](./README.md).

See also:

- [M1 overview](./README.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §2/§3/§9
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§6
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §9
- [DECISIONS](../../DECISIONS.md) 13/17
- Previous workstream: [Internal connectors](./02-internal-connectors.md)
- Next workstream: [Embed · index · serve · eval](./04-embed-index-serve-eval.md)

---

## 1. Tasks

Each row = one reviewable PR. Signer/current-holder/email fields here are bank-internal corporate
control metadata; they are part of the evidence envelope and ownership trail, not customer data.

| ID    | Task                                                                                                                                | Deliverable / done-when                                                                                                       | Design ref                  | Depends on   | Size | Review | Risk |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | --------------------------- | ------------ | ---- | ------ | ---- |
| M1-16 | **Doc-control envelope parser (new):** extract title, version, effective date, owner role, signer, approver, and control references | parser fills the internal envelope from parsed blocks; absent fields fall back to `metadata_config`, never guessed            | DATA-MODEL §2               | M1-13, M1-14 | M    | Heavy  | High |
| M1-17 | **Ownership contact fields (new):** persist signer/current-holder/approver email and role metadata as internal control contacts     | document rows carry the contact metadata needed for review routing and audit trace; schema docs and fixtures match            | DATA-MODEL §2 · DATA-GOV §6 | M1-16        | S    | Medium | Med  |
| M1-18 | **`org_role` resolver (new):** resolve document owner to durable role identity instead of a fragile person-only assignment          | document owner resolves to `org_role`; unresolved roles fail ingest or route to review instead of silently assigning a user   | DATA-MODEL §2               | M1-16        | M    | Heavy  | High |
| M1-19 | **`org_role_history` + leaver survival (new):** preserve ownership when people rotate out of a role                                 | a leaver scenario keeps the document accountable to the role and records historical holder changes                            | DATA-MODEL §2 · DATA-GOV §6 | M1-18        | M    | Heavy  | Med  |
| M1-20 | **Tier stamping + RLS data tests (new):** stamp every document/section with the descriptor tier and test cross-tier denial          | internal rows carry the correct `access_tier`; low-tier caller cannot read group/local confidential rows on real AlloyDB Omni | DATA-GOV §2 · TESTING §2    | M1-15, M1-16 | M    | Heavy  | High |

---

## 2. Notes

- **Do not treat internal control contacts as customer PII.** They are bank-internal corporate
  accountability metadata used for ownership, routing, and audit trail. They still need access
  control and audit handling, but they are not customer/user datasets under DEC 17's platform data
  posture.
- **The parser never invents ownership.** Parsed block wins, then `metadata_config`, then a
  review/error path. A missing control header is an ingestion quality issue, not permission to
  guess an owner or edge.
- **Role identity is the stable anchor.** People move; `org_role` and `org_role_history` keep
  attestations and current ownership durable across leavers.
- **M1-20 is the M1 confidentiality gate.** M0 only created RLS structure; this workstream proves
  row visibility against real, tier-stamped data.
