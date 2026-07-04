<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M1 · Workstream 2: Internal connectors

Build the internal-source side of the ingest pipeline behind the same `ingest.Source` interface:
an authenticated SharePoint web crawl as the default, fixture/non-bank test sources for CI and
early validation, Doc AI parsing for internal documents, and source-level `metadata_config`.
Part of [Milestone M1](./README.md).

See also:

- [M1 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §3
- [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§2/§9
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§3
- [DELIVERY-MODEL](../../../engineering/DELIVERY-MODEL.md)
- [DECISIONS](../../DECISIONS.md) 2/10/13/17
- Previous workstream: [Law ingest](./01-law-ingest.md)
- Next workstream: [Metadata & ownership](./03-metadata-ownership.md)

---

## 1. Tasks

Each row = one reviewable PR. The real adopter source access is configured by the adopter
(DEC 13); this workstream builds the connector and tests it against fixtures or a non-bank test
site until that access exists.

| ID    | Task                                                                                                                                        | Deliverable / done-when                                                                                                    | Design ref              | Depends on   | Size | Review | Risk |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ----------------------- | ------------ | ---- | ------ | ---- |
| M1-9  | **Internal `ingest.Source` adapter (new):** adapt Discover/Fetch for authenticated internal sources while keeping the law-source interface  | `group-std`/`local-policy`/`local-sop` can resolve source descriptors and run Discover/Fetch through one interface         | ARCH §3 · DATA-MODEL §9 | M1-1         | M    | Medium | Med  |
| M1-10 | **SharePoint auth/session seam (new):** browser/web crawl login, token/session storage, throttle, and retry behavior behind an adapter seam | a non-bank SharePoint test site authenticates; offline fake covers CI; failed auth fails closed with a useful ingest error | DATA-GOV §3 · DEC 13    | M1-9         | M    | Heavy  | High |
| M1-11 | **SharePoint discovery + watermark (new):** crawl libraries/folders, detect changed files, and persist per-source watermarks                | unchanged rerun is a no-op; changed file is fetched once; crawl cadence comes from source config                           | DATA-MODEL §9           | M1-10        | M    | Heavy  | Med  |
| M1-12 | **Fixture + non-bank test sources (new):** seed representative internal control docs for CI and pre-adopter validation                      | fixture source and non-bank test-site seed cover group standard, local policy, and SOP templates; no bank access needed    | TESTING §2 · DEC 13     | M1-9         | S    | Medium | Med  |
| M1-13 | **`metadata_config` loader (new):** per-source rules for owner defaults, control-header fields, citation paths, and source-specific columns | source descriptors load `metadata_config`; validation rejects missing required mappings for internal corpora               | DATA-MODEL §2/§9        | M1-9         | S    | Medium | Med  |
| M1-14 | **Internal Doc AI parse adapter (reuse→adapt):** run internal docs through the M1 parse seam with confidential-tier stamping                | `VERTEX=real` parses internal docs on the adopter's GCP; `VERTEX=fake` returns deterministic fixture sections              | ARCH §3 · DEC 10/17     | M1-5, M1-13  | M    | Heavy  | High |
| M1-15 | **Connector integration workflow (new):** Discover→Fetch→Parse smoke workflow for the three internal corpora                                | workflow ingests fixture/test-site docs into bronze/silver stages with source provenance, content hash, and tier metadata  | ARCH §3 · TESTING §2    | M1-11, M1-14 | M    | Medium | Med  |

---

## 2. Notes

- **Internal source access is adopter-provided config, not an upstream platform decision.** The
  product ships the connector shape and the config contract; each adopter supplies the scoped
  account, allowed URLs/shares, cadence, and metadata mapping (DEC 13).
- **The SharePoint crawl is split into auth/session (M1-10) and discovery/watermark (M1-11)**
  because Conditional Access/MFA failures and crawl correctness fail in different ways and deserve
  separate review.
- **Fixtures are first-class.** The delivery model is source-only and self-hosted, so CI cannot
  depend on a bank intranet. M1-12 keeps connector behavior testable before any adopter grants
  access.
- **Managed AI is a configured reference path.** Internal parse uses the adopter's Vertex project
  by default (DEC 10/17), behind the same parse seam that can route to a self-hosted parser if a
  stricter policy selects it.
