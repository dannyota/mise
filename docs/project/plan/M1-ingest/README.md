# Mise — Milestone M1: Ingest the five corpora

The **ingest milestone, planned in detail.** Drive all five corpora through the medallion pipeline
— Discover → Fetch → Parse → Normalize → Embed → Index — into AlloyDB: the two public law corpora
(`vn-reg`, `my-reg`) via the re-homed banhmi/laksa crawlers, and the three internal corpora
(`group-std`, `local-policy`, `local-sop`) via the authenticated SharePoint web crawl + Doc AI
Layout Parser, with the full metadata envelope and durable `org_role` ownership extracted. Every
chunk is embedded @1536-d, tier-tagged at write (RLS), and queryable **per-corpus** over MCP
`search`/`document` — reaching **M1: the five corpora are ingested, tier-isolated, and
retrievable**. It does **not** build any cross-corpus graph edge or detector (→ M2/M3), ship the
reasoning endpoint (→ M4), or the Web UI (→ M5).

The work is split across the four workstream files below; every milestone (M0–M8) is planned to
this depth — see the [build-plan workspace](../README.md).

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [RISKS](../../RISKS.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §3
- [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§2/§3/§9
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§3
- [TESTING](../../../engineering/TESTING.md) §2/§4/§5
- [DELIVERY-MODEL](../../../engineering/DELIVERY-MODEL.md)
- [DECISIONS](../../DECISIONS.md) 2/10/13/17
- Previous: [Milestone M0](../M0-skeleton/README.md)
- Next: [Milestone M2](../M2-graph/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** all 5 corpora flow Discover → Fetch → Parse → Normalize → Embed → Index into
  AlloyDB, metadata-complete and tier-tagged, queryable **per-corpus** over MCP — reaching
  **M1 — the five corpora are ingested, tier-isolated, and retrievable**.
- **Implements (design):** [ARCHITECTURE](../../../design/ARCHITECTURE.md) §3 ·
  [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§2/§3/§9 ·
  [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§3 ·
  [TESTING](../../../engineering/TESTING.md) §2/§4/§5 · [DECISIONS](../../DECISIONS.md) 2/10/13/17.
- **Exit criteria (DoD):**
  - **Law corpora re-ingest end-to-end:** `vn-reg` (VBPL · Công Báo · SBV) and `my-reg`
    (AGC LoM · BNM · SC) crawl → fetch-to-GCS → parse → normalize → embed → index, producing
    `document` + `section` rows with citation paths and validity (DATA-MODEL §1/§2/§3).
  - **Internal corpora ingest from a SharePoint test library:** the authenticated web-crawl
    `ingest.Source` (DEC 2) pulls `group-std`/`local-policy`/`local-sop` from a **non-bank test
    site**, parses via Doc AI, and lands tier-tagged rows.
  - **Metadata envelope complete:** signer/approver/owner, validity, version, and citation path
    populated per the priority order (parsed block → `metadata_config` → source columns); a test
    asserts ownership resolves through `org_role` and survives a leaver via `org_role_history`
    (DATA-MODEL §2).
  - **Embedded @1536-d:** every `section.embedding` is `gemini-embedding-001` @1536-d; the
    dim-assertion is green and a non-1536-d vector fails the run (DECISIONS 1).
  - **Tier tagged + RLS suite green:** every node/chunk carries `access_tier` stamped at ingest;
    the **mandatory cross-tier-deny RLS tests** pass on real AlloyDB Omni — a low-tier caller sees
    no `group-std`/`local-policy` rows (DATA-GOVERNANCE §2, TESTING §2; [RISKS](../../RISKS.md) R2).
  - **Per-corpus MCP `search`/`document` pass the contract test:** provider responses validate
    against the published JSON Schemas; retrieval is validity-aware (in-force by default)
    (API-CONTRACT, TESTING §4).
  - **Retrieval eval meets the inherited banhmi floors:** recall@k ≥ 0.90, MRR@k ≥ 0.85,
    current-law precision = 1.0, abstention ≥ 0.95, citation ≥ 0.95 on the VN/Malay golden set
    (TESTING §5). _(Cross-corpus mapping is **not** scored here — that metric is M3.)_
- **M1a — law-only partial gate:** WS1 (law ingest) + WS4 (embed/index/serve/eval) can exit
  independently on public law corpora alone — delivering per-corpus evidence search over
  `vn-reg` + `my-reg` via MCP before internal doc access materializes. M1a exit criteria: law
  corpora re-ingest end-to-end, embedded @1536-d, per-corpus MCP `search`/`document` pass the
  contract test, retrieval eval meets banhmi floors on the VN/Malay golden set. RLS
  cross-tier-deny tests use **fixture data** simulating confidential tiers (real tier-tagged
  internal rows land when WS2/WS3 complete). M1a unblocks M2-WS1 (graph schema migration).
- **Out of scope:** the full ingest workflow runs only the within-corpus extract; cross-corpus
  graph edges + the `satisfies` mapping → M2/M3; detectors (conflict/gap/staleness) → M3; the
  reasoning endpoint → M4; Web UI → M5; SMB + Graph connectors and registry GA → later (M6).

---

## 2. Dependencies & gates

- **Upstream — M0** provides: the `danny.vn/mise` Go module + monorepo; the **corpus registry**
  (5 resolved descriptors, one shared embed space); **AlloyDB Omni** with schema-per-corpus + RLS
  roles per tier; the **embedder seam** (`gemini-embedding-001` @1536-d real + fake) and the
  parse/judge/ground fake seams; **self-hosted Temporal** + a registered worker. M1 fills the
  empty machinery with real data and the first real workflow.
- **Decision/configuration gates:**
  - **DEC 2 (LOCKED — connector default):** the internal corpora load behind one `ingest.Source`
    with the **authenticated SharePoint web crawl** as the default; **SMB + Graph are out of scope
    here** (later options). Locks the connector surface M1 builds (M1-9 … M1-13).
  - **DEC 13 (LOCKED — adopter source-access config):** the real-intranet crawl uses the
    adopter's approved scoped AD service account, MFA / Conditional-Access exception, source
    URLs/shares, crawl cadence, and metadata mapping. M1 builds and tests the connector against a
    **non-bank test site** or fixtures until an adopter environment is available
    ([RISKS](../../RISKS.md) R3).
  - **DEC 10 (LOCKED — bank-owned Vertex allowed):** the internal corpora's Doc AI parse + embed
    run on the **bank's own** GCP Vertex, inside the bank's perimeter; this is the reference
    design and the data-terms gate is settled.
  - **DEC 17 (LOCKED — data class + region posture):** confidential tiers are internal control
    docs, not customer/user datasets; the adopter pins GCS/AlloyDB/Vertex to its selected region
    in deploy config. This does **not** gate M1. If an adopter later bars managed AI by policy,
    the parse/embed seam swaps to the **self-hosted variant** behind the M0 seam
    (AI-GOVERNANCE §7) — config, not rework.
- **External prerequisites:** a **non-bank SharePoint test site** seeded with representative
  control-doc templates (to exercise the crawl + envelope parsing before real adopter access); a
  **GCS bucket** for immutable raw files; a dev GCP project with Doc AI + Vertex for the Mode A
  fidelity path (the offline Mode B fakes gate CI).

---

## 3. Workstreams

The M1 breakdown, split into four reviewable workstreams. Tasks are PR-sized and dependency-
ordered (IDs `M1-1 … M1-24`); the two genuinely hard tasks — the **authenticated web-crawl
connector** and the **metadata-envelope parsing** — are each split so no single PR is oversized.
Each task: **Size** XS–L · **Review** Light/Medium/Heavy · **Risk** Low/Med/High.

| #   | Workstream                                                     | Tasks         | Theme                                                             |
| --- | -------------------------------------------------------------- | ------------- | ----------------------------------------------------------------- |
| 1   | [Law ingest](./01-law-ingest.md)                               | M1-1 … M1-8   | re-home law crawlers · Doc AI parse seam · law Normalize          |
| 2   | [Internal connectors](./02-internal-connectors.md)             | M1-9 … M1-15  | SharePoint web-crawl `ingest.Source` · Doc AI · `metadata_config` |
| 3   | [Metadata & ownership](./03-metadata-ownership.md)             | M1-16 … M1-20 | envelope parsing · `org_role` resolver + history · tier tagging   |
| 4   | [Embed · index · serve · eval](./04-embed-index-serve-eval.md) | M1-21 … M1-24 | embed @1536-d · index + ScaNN · per-corpus MCP · workflow · eval  |

---

## 4. Test & eval gates (this milestone)

- **Unit (offline = Mode B):** Discover/Fetch/Normalize logic + the Doc AI parse seam tested
  against fakes; the offline path **is** the unit-test path (TESTING §2, LOCAL-DEV §4). Dim
  assertion: a non-1536-d embedding fails (DECISIONS 1).
- **RLS (mandatory, real AlloyDB Omni via testcontainers):** with real tier-tagged data now in
  place, a low-tier caller must not see `group-std`/`local-policy`/`local-sop` rows — the
  cross-tier-deny suite M0 deferred for lack of data now lands here as a hard gate
  (DATA-GOVERNANCE §2, TESTING §2; [RISKS](../../RISKS.md) R2).
- **Integration (medallion):** each connector ingests a fixture corpus end-to-end into AlloyDB
  with the full envelope; idempotent re-run is a no-op (watermark/content-hash dedup).
- **Contract test:** per-corpus MCP `search`/`document` responses validate against the published
  JSON Schemas; `reasoning`/`web` are not consumers yet, but the provider half gates here
  (TESTING §4).
- **Concurrency:** `testing/synctest` covers the ingest workflow's activities, retries, and
  backoff under virtual time — no flakes (TOOLCHAIN §3).
- **Retrieval eval (first real gate):** the inherited banhmi `cmd/eval` harness scores
  recall@k ≥ 0.90, MRR@k ≥ 0.85, current-law = 1.0, abstention ≥ 0.95, citation ≥ 0.95 on the
  VN/Malay golden set (TESTING §5). The **cross-corpus mapping** metric is **not** scored here —
  there are no edges yet (M3).

---

## 5. Risks (this milestone)

- **Crawler MFA / Conditional-Access** (M1-9/M1-12) — a headless AD login is often blocked by the
  bank's MFA / Conditional-Access policy; the real-intranet crawl uses the adopter's **approved
  scoped service account** (DEC 13). Build against a **non-bank test site** until adopter access is
  available ([RISKS](../../RISKS.md) R3).
- **Managed-AI policy drift** (M1-14/M1-22) — confidential-tier parse + embed run on the
  **bank's own** Vertex by default (DEC 10/17). The seam keeps a stricter adopter policy a config
  swap to the self-hosted variant ([RISKS](../../RISKS.md) R1).
- **Doc-control / metadata parsing brittleness** (M1-16 … M1-19) — ragged internal templates break
  signer/owner extraction; drive parsing off the per-source `metadata_config` and treat an absent
  header as "fall back to config default," never a guess ([RISKS](../../RISKS.md) R6).
- **RLS correctness on the new tier-tagged data** (M1-20) — a mis-stamp or a policy bug collapses
  tier isolation across the cross-corpus surface; the mandatory cross-tier-deny suite is the gate
  ([RISKS](../../RISKS.md) R2).
- **One-embedding-space drift** (M1-21) — a corpus embedded in the wrong model/dim fragments the
  shared space; the dim assertion fails closed at index time ([RISKS](../../RISKS.md) R12,
  DECISIONS 1).
