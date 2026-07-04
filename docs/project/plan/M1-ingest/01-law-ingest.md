<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M1 · Workstream 1: Law ingest

Re-home the banhmi/laksa public law crawlers into mise's medallion pipeline, put Doc AI Layout
Parser behind a real+fake seam, and normalize law into the validity envelope (in-force model +
`amendment_event`). Public corpora — **never gated**. Part of [Milestone M1](./README.md).

See also:

- [M1 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §3
- [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§2/§3
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §3
- [DECISIONS](../../DECISIONS.md) 2
- Next workstream: [Internal connectors](./02-internal-connectors.md)

---

## 1. Tasks

Each row = one reviewable PR. Reuse = adapt existing banhmi/laksa engine code; New = net-new to
mise. The law path is public-tier, so nothing here is gated by managed-AI policy.

| ID   | Task                                                                                                                                                  | Deliverable / done-when                                                                                                         | Design ref                           | Depends on | Size | Review | Risk |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ---------- | ---- | ------ | ---- |
| M1-1 | **`ingest.Source` interface + Discover/Fetch stages (reuse→adapt):** generalize the engine's crawler entry into the corpus-keyed medallion stages     | one `ingest.Source` interface; Discover (delta/watermark) + Fetch-to-GCS stages drive off the corpus descriptor (DATA-MODEL §9) | ARCH §3 · DATA-GOV §3                | —          | M    | Medium | Low  |
| M1-2 | **Re-home `vn-reg` crawlers (reuse):** VBPL · Công Báo · SBV · vanban.chinhphu.vn behind the new `ingest.Source`                                      | `vn-reg` Discover+Fetch runs against the real public sources; raw docs land immutable in GCS with provenance                    | DATA-MODEL §1                        | M1-1       | M    | Medium | Low  |
| M1-3 | **Re-home `my-reg` crawlers (reuse):** AGC LoM · BNM · SC behind the new `ingest.Source`                                                              | `my-reg` Discover+Fetch runs against the real public sources; raw docs land in GCS with provenance                              | DATA-MODEL §1                        | M1-1       | M    | Medium | Low  |
| M1-4 | **Fetch → GCS immutable raw store (new):** every fetched artifact written once to GCS, keyed by content hash; re-fetch is a no-op                     | raw bytes immutable in GCS; `ingest_run_id`/`observed_at`/`source_url`/`content_type` recorded; re-run dedups by hash           | DATA-GOV §3 · DATA-MODEL §2          | M1-1       | S    | Light  | Low  |
| M1-5 | **Doc AI Layout Parser seam — interface + fake (new):** extend the M0 parse seam into a real Parse stage interface + a deterministic fake             | Parse stage sits behind a Go interface; the fake returns canned section/heading shapes; offline run needs no GCP (Mode B)       | ARCH §3 · DATA-GOV §3 · LOCAL-DEV §4 | M1-1       | S    | Light  | Low  |
| M1-6 | **Doc AI Layout Parser — real adapter (new):** the real Vertex Doc AI call for law docs; chunk into `section` rows with `heading_path` citation paths | `VERTEX=real` parses law docs into sections with ancestral heading → citation path; public-tier only (no gate)                  | ARCH §3 · DATA-MODEL §2              | M1-5       | M    | Medium | Low  |
| M1-7 | **Normalize for law — validity envelope (new):** map parsed law into `document`/`section` with `validity_status`, issued/effective/expiry dates       | law `document` rows carry the §3 in-force state + dates + citation scheme; section inherits doc validity (DATA-MODEL §2/§3)     | DATA-MODEL §2/§3                     | M1-6       | M    | Medium | Med  |
| M1-8 | **`amendment_event` capture (new):** record `(target_doc, amending_doc, clause, date)` at ingest; transition validity (amended/superseded/repealed)   | amending acts captured as `amendment_event` rows; targets transition state; repealed/not-yet-effective never read as current    | DATA-MODEL §3                        | M1-7       | M    | Medium | Med  |

---

## 2. Notes

- **Why law first:** the public corpora are the proven, **ungated** path (banhmi/laksa already
  ingest them) — re-homing them validates the whole medallion shape (Discover→Fetch→Parse→
  Normalize) before the internal connectors add the connector + confidentiality complications.
- **`amendment_event` is split from Normalize** (M1-8 vs M1-7): validity-state transitions touch
  the in-force model the whole product depends on (`current-law precision = 1.0`, TESTING §5) and
  deserve their own focused review — a wrong transition silently presents repealed text as current.
- **Doc AI parse is split into the seam+fake (M1-5) and the real adapter (M1-6)** so the offline
  Mode-B path lands first and CI never needs GCP to exercise the parse stage (LOCAL-DEV §4).
- The law parse is **public-tier**. The same Doc AI seam is reused for the internal corpora in
  [Workstream 2](./02-internal-connectors.md), where the confidential-tier policy controls apply.
