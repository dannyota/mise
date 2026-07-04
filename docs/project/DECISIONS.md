# Mise — Decision Log

The decisions that shape mise. Cited elsewhere as **`DECISIONS N`** (e.g. `DECISIONS 10`).
**Locked** = settled for the reference design. **Open** = needs implementation-time review,
eval evidence, or security design before it can be locked. Adopter-owned deployment settings
(credentials, region, sizing, security sign-offs) are not project decisions. Cost figures live in
[COST.md](./COST.md); the build sequence that honours these decisions is [PLAN.md](./PLAN.md).

See also:

- [PLAN.md](./PLAN.md) (build plan)
- [ARCHITECTURE.md](../design/ARCHITECTURE.md)
- [DATA-GOVERNANCE.md](../design/DATA-GOVERNANCE.md)
- [AI-GOVERNANCE.md](../design/AI-GOVERNANCE.md)
- [DELIVERY-MODEL.md](../engineering/DELIVERY-MODEL.md)
- [COST.md](./COST.md)

---

## ⚖️ Locked

<!-- prettier-ignore-start -->

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
10. **Confidential-tier Vertex processing:** **yes for the reference deployment.** Confidential
    internal text may be processed by Vertex-hosted models because mise is deployed
    single-tenant in the adopter bank's **own GCP project**, under the bank's **own** Vertex
    contract, IAM, region choice, VPC-SC/CMEK posture, and security review. This is **not**
    upstream/vendor runtime access: upstream ships source only, holds no data, and cannot reach
    the instance (decision 9; DELIVERY-MODEL). This applies to Doc AI parse, document/query
    embedding, Gemini judge/grounding, the Claude reasoning endpoint, and confidential-tier
    translation when enabled. Controls stay mandatory: server-side model calls only, RLS/tier
    filtering before read-side evidence reaches a model, audit of every model turn, and a
    deployment-time capability flag for optional translation. The **self-hosted AI stack remains
    a fallback variant**, not the default design, if an adopter's procurement/legal review
    forbids managed Vertex processing.
12. **Runtime sizing is a reference default, not an upstream gate.** The published deployment
    ships conservative defaults (one GKE cluster, no HA, small node pool, 1–2 vCPU AlloyDB Omni
    with **2 vCPU as the safe default**) and the cost model shows the trade-offs. Because mise is
    AGPL source deployed by the adopter, each bank can scale node size/count, DB vCPU, and worker
    concurrency in its own project without an upstream design change.
13. **Internal-source access contract:** the connector shape is locked by decision 2. Upstream
    ships the `ingest.Source` interface, the authenticated SharePoint web-crawl default, the SMB
    option, the optional Graph connector path, Secret Manager wiring, and the metadata-envelope
    config. The adopter supplies its approved AD service account or Graph app, any MFA /
    Conditional-Access exception, source URLs/shares, crawl cadence, and source-column mapping in
    deploy config. Build and CI use fixtures or a non-bank test site until a real adopter
    environment is available; that is integration input, not a mise decision.
14. **Embedding call site:** **in-app Go embedder** — the ingest pipeline calls `pkg/rag/embed`'s
    `Embedder` interface directly, not AlloyDB's in-DB `google_ml.embedding()`. Rationale:
    `google_ml.embedding()` still calls Vertex over the network, so it gives no true-offline path;
    the Go interface gives Mode B (LOCAL-DEV §4) a real fake seam and keeps the store portable off
    AlloyDB Omni if that's ever needed. Locked 2026-07.
15. **Scale-down is decided; tuning is deploy config.** Auto scale-down stays in the reference
    deployment (KEDA + cluster autoscaler; workers scale to zero; DB idle-stop/pre-warm policy in
    DEPLOYMENT/COST). Exact cadence, idle thresholds, and pre-warm windows are operator settings,
    not product decisions.
16. **AlloyDB Omni remains the reference store.** Decision 4 locks AlloyDB Omni for the reference
    deployment because filtered ScaNN lands on the always-filtered read path. License metering and
    vCPU rates belong in COST.md; if an adopter's economics differ, it can resize or carry a
    pgvector fork, but upstream DB strategy stays settled.
17. **Data class and region posture:** the reference corpora are public regulation plus
    bank-internal control documents, **not customer/user datasets**. Vietnamese personal-data
    cross-border rules apply to data that identifies a person (e.g. Luật Bảo vệ dữ liệu cá nhân
    91/2025/QH15 Điều 2) and Decree 53/2022/NĐ-CP Article 26 localizes service-user data; neither
    makes generic policy/SOP/control text a blocker. Internal names/emails/signers, where
    kept, are bank-controlled corporate contact metadata and are minimized/role-anchored
    (DATA-MODEL §2, DATA-GOVERNANCE §9). The adopter selects a supported GCP/Vertex region in
    deploy config and may choose stricter in-country hosting by policy; that is an operator
    control, not an upstream decision gate.

20. **Graph tier via GENERATED column, not trigger.** `graph.relation_edge.access_tier` is
    `GENERATED ALWAYS AS (graph.stricter_tier(graph.corpus_tier(from_corpus_id),
    graph.corpus_tier(to_corpus_id))) STORED` — three IMMUTABLE functions (`corpus_tier`,
    `tier_rank`, `stricter_tier`), no trigger, no app-set column. RLS policies reference
    `access_tier` directly, so a new corpus only needs one new `corpus_tier` CASE branch; no
    app deploy. Locked 2026-07.
21. **doc_ref stub model for forward references.** `graph.doc_ref` rows can exist with
    `document_id IS NULL` (an unresolved stub) before the target document is ingested.
    `relation_edge.to_ref_id` FK's to `doc_ref.id`, not to the document directly, so edges
    written today survive a later ingest that resolves the stub — no edge rewrites. A partial
    index (`doc_ref_unresolved_idx`) finds unresolved stubs for batch resolution. Locked 2026-07.
22. **direction='up' convention.** Every `relation_edge.direction` value today is `'up'` — the
    control-chain authority direction (SOP -> Policy -> Group -> law). Reverse (`'down'`)
    traversal is documented in the API contract but rejected at runtime until M5 Graph Explorer.
    Locked 2026-07.
23. **huma v2 for REST.** The graph REST surface (`GET /graph/nodes/{ref}`,
    `GET /graph/chain/{ref}`) uses huma v2 on chi — its struct-tag-driven OpenAPI generation
    keeps the Go source and `api/openapi.yaml` in lock-step, validated by an `openapi_drift_test`
    `cmp.Diff` guard. Locked 2026-07.
24. **depguard scoping: pkg/graph is Method-A-pure.** golangci-lint's depguard rule restricts
    `pkg/graph` to `pkg/corpus`, `encoding/json`, `strings`, `github.com/google/uuid` — no
    model client, no store, no network. Method A extraction is a pure function over
    already-resolved data; any store or model call must happen in the caller (`pkg/store`),
    never inside `pkg/graph`. Locked 2026-07.
25. **org_role forward-pull.** `graph.org_role` is seeded in M2 (migration 011) with the
    resolver (`OrgRole.Resolve`) available for attestation-owner lookup at extraction time.
    The full as-of-date history and UI surface are deferred to M4; M2 uses it as a simple
    current-holder lookup. Locked 2026-07.

11. **Threshold defaults (provisional).** Confidence ≥ 0.7, grounding ≥ 0.6
    (`detect.ThresholdConfig`). Both values are **env-configurable** and **provisional** — set
    from the first real-corpus eval run, not a priori reasoning. Severity auto-set by kind:
    conflict = critical, staleness = high, gap = medium. Locked (provisional) 2026-07.
18. **Mapping eval baseline.** The first eval run sets the baseline; `min-precision=0`,
    `min-recall=0` (provisional). The eval harness runs but does not gate until thresholds are
    calibrated from real data. Human attestations append to the golden set over time
    (TESTING §5, DATA-MODEL §8). Locked (provisional) 2026-07.
26. **Detectors live in `pkg/detect` (outside depguard fence).** Method B (model-calling)
    detectors import `pkg/vertex` (judge/grounder) and `pkg/rag/embed` (embedder) — imports
    `pkg/graph`'s depguard rule forbids. `pkg/detect` is the clean boundary: it calls models,
    `pkg/graph` never does. Locked 2026-07.
27. **FactEmbedder optional interface pattern.** `detect.FactEmbedder` (single-fact embed) is a
    separate interface from `embed.Embedder` (batch). Callers type-assert when they need the
    single-fact path; the fake embedder satisfies both. Locked 2026-07.
28. **ConflictDetect = two-stage (judge then grounder).** A candidate edge is judged first (Gemini
    classify), then grounded (Check Grounding) — both must pass `ThresholdConfig.Gate` before the
    edge is written. The two-stage pipeline avoids grounding calls on candidates the judge already
    rejects. Locked 2026-07.
29. **Transitive covers only (DEC 7 enforced).** `pkg/detect` never runs a direct SOP→law judge
    call. SOP reaches law only transitively via Policy (SOP→Policy→Group→law). DEC 7's constraint
    is enforced by `graph.EdgeTypeForPair` rejecting the `(local-sop, vn-reg)` pair. Locked
    2026-07.

<!-- prettier-ignore-end -->

## 🗺️ Open

<!-- prettier-ignore-start -->

19. **Webhook egress policy:** before the M5 webhook surface ships, specify the SSRF-safe egress
    policy for `endpoint_url`: allowlist model, URL validation rules, private-address handling,
    DNS re-resolution behavior, timeout/retry limits, and audit fields. DATA-MODEL §10 owns the
    subscription schema; THREAT-MODEL §6 flags the egress restriction as the build-phase security
    decision.

<!-- prettier-ignore-end -->
