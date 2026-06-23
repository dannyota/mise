# Mise — Build Plan

> **Design-phase build plan.** It owns the **goals** and the **phased build sequence**, with
> each phase **traced to the design docs it implements** — the bridge from design → build.
> Detailed task breakdown, estimates, and per-phase exit criteria are the **next (planning)
> phase**. Decisions live in [DECISIONS.md](./DECISIONS.md); costs in [COST.md](./COST.md).

See also: [DECISIONS.md](./DECISIONS.md) (decision log) ·
[ARCHITECTURE.md](../design/ARCHITECTURE.md) (system design) ·
[DATA-MODEL.md](../design/DATA-MODEL.md) · [DELIVERY-MODEL.md](../engineering/DELIVERY-MODEL.md)
(how it ships) · [COST.md](./COST.md).

---

## 1. 🗺️ Goals

1. Maintain **5 separate evidence-only corpora**, each homogeneous in structure, owner, and
   access tier (full banking regulation, not just the tech subset).
2. Use Vertex AI to **detect**: relations (mappings), conflicts, coverage gaps, and staleness
   — across corpora.
3. **Answer audit questions** with grounded, citation-backed, verified answers.
4. **Scale** to new corpora/scopes (reports, diagrams, circular feeds, …) with no core change.

**Non-goal:** mise never _asserts compliance_. It surfaces evidence, machine-proposed
mappings, and verified findings; humans promote/attest.

---

## 2. 🚀 Build plan — phased, traced to design

Each phase names the **design docs/sections it implements**, so the plan stays anchored to the
design. **Task detail, sequencing, estimates, and exit criteria are the planning phase's job**,
not this table.

| Phase             | Builds (design refs)                                               | Deliverable                                                                                                                                                                                                                     |
| ----------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **0 Skeleton**    | ARCHITECTURE §2/§9 · DATA-MODEL §1 · FOLDER_STRUCTURE · LOCAL-DEV  | Reuse + improve the banhmi/laksa engine into mise; generalize `jurisdiction` → corpus registry; swap embedder → `gemini-embedding-001`; provision AlloyDB (schema-per-corpus) + `graph` schema + Temporal; local podman compose |
| **1 Ingest 5**    | ARCHITECTURE §3 (write AI) · DATA-MODEL §1/§2 · DATA-GOVERNANCE §3 | Law via existing crawlers; internal 3 via the authenticated SharePoint web crawl (DECISIONS 2) + Doc AI Layout Parser; extract the metadata envelope; embed @1536-d; per-corpus MCP                                             |
| **2 Graph spine** | DATA-MODEL §4/§5 · ARCHITECTURE §4                                 | `graph` schema; extract explicit internal edges (SOP→Policy→Group) in Normalize; graph API                                                                                                                                      |
| **3 Detectors**   | DATA-MODEL §5/§6 · AI-GOVERNANCE §4 · ARCHITECTURE §5              | RelationDetect → Conflict → Gap → Staleness as Temporal workflows; the review queue                                                                                                                                             |
| **4 Audit Q&A**   | ARCHITECTURE §6 · AI-GOVERNANCE §5 · API-CONTRACT §2/§4            | Evidence (Go+AlloyDB) → reasoning endpoint (Claude Agent SDK) → cited answer; REST + MCP + SSE                                                                                                                                  |
| **5 Web UI**      | UI-DESIGN (all) · API-CONTRACT §3 · DATA-MODEL §7/§10              | Dashboards · Graph Explorer · Review Workbench · Q&A chat · Resolution · Change Timeline · Notifications · Reports · cross-lingual side-by-side · Admin                                                                         |
| **6 Scale**       | ARCHITECTURE §8 · DATA-MODEL §9                                    | Corpus registry GA; multimodal (reports/diagrams); new scopes via descriptor only                                                                                                                                               |

### Build delta — reuse · change · new

- **Reuse** (the open-source banhmi/laksa engine): Temporal medallion pipeline, pgvector store, validity
  model, intra-corpus relations, evidence-only MCP surface, ingest ledger.
- **Change** (→ Vertex / Claude): parser → Doc AI Layout Parser; embedder →
  `gemini-embedding-001`; store → AlloyDB + ScaNN (pgvector-compatible, code unchanged);
  serve reasoning → Claude Agent SDK.
- **New:** corpus registry, compliance graph, 4 cross-corpus detectors, reasoning endpoint,
  Vue Web UI, auth/access tiers.

### Cross-cutting (every phase)

The build honours these throughout; each is owned by its doc.

- **Delivery:** open-source, single-tenant, self-hosted per enterprise — DELIVERY-MODEL.
- **Evidence-only + grounding gates:** AI-GOVERNANCE §1/§4; **access control (RLS):**
  DATA-GOVERNANCE §2; **audit:** DATA-GOVERNANCE §6 + AI-GOVERNANCE §9.
- **Evals:** mapping precision/recall + retrieval recall@k vs a golden set — TESTING §5.
- **Engineering standards:** pinned toolchains (TOOLCHAIN); CI gates + supply chain (CI-CD);
  tracing + SLOs (OBSERVABILITY); the test pyramid + contract test (TESTING); one generated
  API/MCP contract (API-CONTRACT).
