# Mise — Decision Log

The locked and open decisions that shape mise. Cited elsewhere as **`DECISIONS N`** (e.g.
`DECISIONS 10`). **Locked** = settled for this design; **Open** = still to resolve (several
feed the planning phase). Cost figures live in [COST.md](./COST.md); the build sequence that
honours these decisions is [PLAN.md](./PLAN.md).

See also: [PLAN.md](./PLAN.md) (build plan) · [ARCHITECTURE.md](../design/ARCHITECTURE.md) ·
[DATA-GOVERNANCE.md](../design/DATA-GOVERNANCE.md) ·
[AI-GOVERNANCE.md](../design/AI-GOVERNANCE.md) ·
[DELIVERY-MODEL.md](../engineering/DELIVERY-MODEL.md) · [COST.md](./COST.md).

---

## ⚖️ Locked

1. **Embedding:** `gemini-embedding-001` @ **1536-d**, shared across all corpora. On read the
   query is embedded with the **same** model and the vector is **cached** (key = a **hash** of
   the normalized query + a _space version_ of model/dims/`task_type`). Entries never go stale
   — only a **space change** busts them, so there is **no manual cleanup**. The cache is
   tier-agnostic and shared (the vector is identical regardless of who asks; access-tier/RLS
   filtering happens at retrieval, not here).
2. **Internal doc source:** behind one `ingest.Source` interface. **The realistic default is
   the authenticated SharePoint web crawl** (AD-account login, no Graph API) — used wherever
   **no Azure AD app is provisioned** for Graph (the common case — a Graph app carries setup
   cost + a security sign-off). **SMB file-share loader** for docs on a
   Windows/SMB share. **Microsoft Graph** (delta query) stays _optional_ — its only real edge
   is **incremental sync**; its "rich metadata" is the **uploader (an assistant), not the
   signer/owner**, so it doesn't justify its setup cost. **Authoritative metadata is parsed
   from the document (signature/control block) + a structured per-source config** — the same
   for every connector (DATA-MODEL §1/§2); parsing detail is implementation-phase.
3. **Temporal:** **self-hosted on GKE** (server + Go workers; persistence reuses the shared DB
   instance).
4. **Store:** **AlloyDB Omni in GKE from day 1**, right-sized to **1–2 vCPU** (1 vCPU viable
   at low concurrency — vector search is memory-bound and the AI work is on Vertex; **2 vCPU =
   safe default** for headroom + index builds; free in dev). One database, schema-per-corpus +
   `graph` schema, ScaNN, RLS. Cost scales with **vCPU count, not usage** (COST.md). NOT
   Vertex Vector Search. (pgvector removes the license.) Justified here because the read path
   is **always filtered** (access-tier/RLS) — ScaNN's filtered-search edge lands on the
   user-facing latency (see decision 5).
5. **Serving = evidence-only (read-fast) + a backend reasoning endpoint.** The query path is
   **Go + AlloyDB Omni only**: hybrid retrieve (ScaNN + FTS, RRF) with access-tier/RLS filter
   - graph/validity joins → ranked evidence + citations, in ms. **Reasoning** (rerank,
     compose, cite) runs at the **reasoning endpoint** — a **separate TypeScript service**
     (Node 24 LTS + Hono) on the **Claude Agent SDK**, running **Claude Haiku 4.5 / Sonnet 4.6
     on Vertex** (`CLAUDE_CODE_USE_VERTEX=1` + ADC), MCP client to mise's tools, **read-only +
     permission-gated**, hooks → audit, streaming to the UI. Model calls stay **server-side,
     never the browser**. _(TS over Go/`anthropic-sdk-go`: the Agent SDK is Python/TS-only and
     gives the full harness — hooks, permissions, MCP client; a separate service isolates the
     polyglot cost behind an MCP/SSE boundary.)_ **Write-path AI** (parse · embed-docs · judge ·
     ground) is **Gemini 3.5 Flash** + the Google stack at ingest. **UI:** reuse the proven Vue
     stack (Vite + Vue 3.5 + Vue Flow/elkjs + TanStack Table + reka-ui/Tailwind 4 + mermaid).
6. **Seed order:** internal extractable edges first (SOP→Policy→Group), then inferred
   law-facing `satisfies`.
7. **SOP→law:** transitive-only (via Policy), never a direct judge run.
8. **Scope:** corpora cover **all banking regulation**, not just digital/technology.
9. **Deployment & tenancy:** **single-tenant — one instance deployed per enterprise, in the
   bank's own GCP project, operated by the bank.** mise is **open source (AGPL-3.0)**: the
   adopter **builds from the public source and self-hosts** — **no** proprietary vendor binary,
   **no** upstream runtime access, **no** upstream-held data; Vertex is the bank's own GCP
   Vertex. A future commercial/managed distribution stays possible under the dual-license
   (DELIVERY-MODEL §6). **One GKE cluster — no Cloud Run, no HA** (single replicas). Query path
   always-on; ingestion workers scale to zero (indexing is infrequent); **auto scale-down**
   (KEDA + cluster autoscaler). Cost ranges in COST.md (the license floor stays; see decision
   16); delivery model in DELIVERY-MODEL; runtime in DEPLOYMENT; threat view in THREAT-MODEL §3.

## 🗺️ Open

10. **Confidentiality gate — does confidential internal text go to Vertex at all?** The one
    open AI question; it hits Doc AI parse, embedding, the Gemini judge, grounding, and the
    serve agent. In the per-enterprise model (decision 9) this is the **bank's own GCP Vertex**
    under the **bank's** enterprise terms/region (optionally VPC-SC + CMEK), so the bank owns
    the call. If **yes** (data terms — not used for training, region-pinned), the design
    stands. If **no**, self-host the AI stack — lead embedder **Qwen3-Embedding-8B**
    (Apache-2.0, 32K ctx, MRL→1536), picked via a VN/Malay legal golden-set bake-off vs
    gemini-embedding-001 / BGE-M3. _(The embedding mechanism itself is resolved — decision 1.)_
    The component-by-component **swap map** for this branch is AI-GOVERNANCE §7.
11. **Model split (all on Vertex — one auth/governance boundary):** **write path** = **Gemini
    3.5 Flash** for judge/extraction (can move to Haiku / Sonnet — COST.md). **Serve path** =
    **Claude Haiku 4.5** (default) / **Sonnet 4.6** (hardest) via the **Agent SDK**
    (decision 5). Confirm per-task thresholds in Phase 3/4.
12. **GKE node sizing:** one larger node vs two small; free zonal cluster tier.
13. **Internal-source access (web crawl is the default — decision 2):** which AD **service
    account** the crawler/SMB loader uses and how its creds are scoped/stored. **Key risk —
    MFA / Conditional Access:** a headless AD login is often blocked by the bank's MFA /
    Conditional-Access policy, so the crawl needs an **approved exception** (a scoped,
    read-only service account, IP-allowlisted) — a security sign-off, not just config. Also:
    crawl cadence + how the web UI's version-history/columns map to the §2 metadata envelope.
14. **In-DB vs in-app embedding:** `google_ml.embedding()` in AlloyDB vs the Go embedder
    calling Vertex — portability vs fewer moving parts (still a network hop either way; see
    open item 10).
15. **Scale-down tuning:** auto scale-down is **decided** (decision 9); what remains is
    operational tuning — KEDA cron cadence, the idle-DB stop threshold, and pre-warm timing
    before busy windows.
16. **AlloyDB Omni license metering — decides DB strategy:** confirm whether it meters on
    running-vCPU-hour (stopping the DB when idle saves it) vs a fixed subscription (a 24/7
    floor). If fixed and under heavy scale-down, pgvector is cheaper; but the corrected rate +
    ScaNN's filtered-search edge on the always-filtered read path keep AlloyDB Omni the choice
    (decisions 4–5; figures in COST.md §6).
17. **Vietnam data residency & region (a legal axis of decision 10, elevated).** Beyond the
    _data-terms_ question (10 — is confidential text used for training?), a VN bank faces
    **data-localization / personal-data law** — **Decree 53/2022** (localization) and **Decree
    13/2023 (PDPD)** — and the practical question of **whether Claude-on-Vertex and the Gemini
    write-stack are served from an acceptable region** (in-country / Singapore). This can force
    the self-host fallback (decision 10's "no" branch) **regardless of data terms**, so resolve
    it early: confirm (a) which Vertex models are available in an acceptable region, (b) whether
    PDPD/Decree 53 permit cross-border processing of the confidential tiers, (c) the residency
    of GCS raw files + AlloyDB. Drives the same parse/embed/judge/serve touchpoints as decision
    10 (AI-GOVERNANCE §7, DATA-GOVERNANCE §3/§9).
18. **Eval cold-start (golden-set bootstrap):** before any human attestation exists, seed a
    small hand-labelled VN/Malay set so the eval harness produces a baseline at first release;
    otherwise mapping precision/recall has nothing to score against (TESTING §5).
