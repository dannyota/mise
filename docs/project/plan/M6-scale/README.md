# Mise — Milestone M6: Scale & multimodal

The **final milestone, planned in detail.** Make the corpus registry **GA** — a new corpus or
scope ships as a **descriptor + source plugin + scope seed with no core change** — and prove it by
landing two new corpus kinds: a **reports** corpus (audit/risk) whose finding-nodes map into the
graph (a living audit map), and a **multimodal `diagram`** path that verbalizes figures/images
(Layout Parser + Gemini-vision captions, caption text embedded @1536-d, the image ref kept). All
under the one hard constraint — the **single embedding space** (DECISIONS 1), enforced fail-closed
at registration. It adds **no new subsystem**: everything rides the M0–M5 spine.

This is the **last milestone — no successor.** The breakdown is split across the three workstream
files below; tasks are PR-sized and dependency-ordered (`M6-1 … M6-13`). Each task: **Size** XS–L ·
**Review** Light/Medium/Heavy · **Risk** Low/Med/High.

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [RISKS](../../RISKS.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §8
- [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§2/§4/§6/§9
- [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §4/§7/§9
- [DELIVERY-MODEL](../../../engineering/DELIVERY-MODEL.md)
- [DECISIONS](../../DECISIONS.md) 1/8/10/17
- Previous: [Milestone M5](../M5-web-ui/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** a new corpus is added **by descriptor only** — zero edits to detector/graph/serve
  code — and two new kinds (a `report` corpus and a `diagram` multimodal path) ingest, map, and
  retrieve under the locked single embedding space, reaching **M6 — registry GA + multimodal**.
- **Implements (design):** [ARCHITECTURE](../../../design/ARCHITECTURE.md) §8 ·
  [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§2/§4/§6/§9 ·
  [AI-GOVERNANCE](../../../design/AI-GOVERNANCE.md) §4/§7/§9 · [DECISIONS](../../DECISIONS.md)
  1/8/10/17.
- **Exit criteria (DoD):**
  - **GA = descriptor-only** — adding a corpus touches **no core file**; a **CI no-core-diff guard**
    asserts it (the load-bearing acceptance — if a code edit is still needed, GA is not met).
  - **One embedding space, fail-closed at registration** — a descriptor whose
    `embed{model,dims,task_type}` differs from the locked space is **refused before any ingest**;
    native image vectors (e.g. `gemini-embedding-2`) are barred (DECISIONS 1; [RISKS](../../RISKS.md)
    R12).
  - **`graph_role` enforced** — the descriptor's `graph_role{can_source,can_target,default_edges}`
    governs which edges a corpus may source/target and seeds its default edges, applied at edge
    write (DATA-MODEL §4, ARCHITECTURE §8).
  - **`metadata_config` GA** — the per-source metadata config (defaults + parse locations) deferred
    from M1 is finalized and **validated on registration** (DATA-MODEL §2/§9; [RISKS](../../RISKS.md)
    R6).
  - **Reports corpus + living audit map** — a `report` corpus ingests through the medallion, and its
    finding-nodes map into `relation_edge` (finding → the obligation/policy it concerns), turning the
    graph into a living audit map (ARCHITECTURE §8, DATA-MODEL §6).
  - **Multimodal `diagram` path** — Layout Parser verbalizes figures/tables, Gemini-vision captions
    them on the **bank's own** Vertex, the **caption text** is embedded @1536-d, and the **image ref**
    is kept for human verification; no native image vector enters the space (ARCHITECTURE §8,
    AI-GOVERNANCE §4/§7).
  - **New scope, descriptor-only** — a second `local-*` scope with a different `jurisdiction` routes
    `satisfies` correctly with **no new tier** and no core edit (DATA-MODEL §9, DECISIONS 8).
  - **Registry surface + contract** — a registry admin + MCP/REST surface lists/adds/inspects corpora
    through the **generated contract**; the contract test is green (API-CONTRACT §5;
    [RISKS](../../RISKS.md) R8).
  - **Eval floors hold with the new corpora present** — retrieval + mapping + caption retrieval are
    scored against the golden set and the inherited floors do not regress (TESTING §5).
- **Out of scope:** no new subsystem; runtime scale-out / HA tuning is a DEPLOYMENT + COST knob, not
  a build task (DECISIONS 9); a future commercial/managed distribution under the dual-license
  (DELIVERY-MODEL §6).

---

## 2. Dependencies & gates

- **Upstream:** the whole M0–M5 spine — the **corpus registry** + embedder seam (M0), the medallion
  pipeline + `metadata_config` + tier-tagged ingest (M1), the `graph` schema + tier-inheritance + the
  graph API (M2), the findings model + detectors (M3), the evidence/serve surface (M4), and the
  Corpus Admin screen the registry surface extends (M5). M6 generalizes, it does not add a layer.
- **Decision/configuration gates:**
  - **DECISIONS 1 (LOCKED — single embedding space).** The registry **rejects** any non-conforming
    `embed{}` at registration; this is M6's headline enforced invariant, not a soft check
    ([RISKS](../../RISKS.md) R12).
  - **DECISIONS 8 (LOCKED — all-banking-regulation scope).** New corpus kinds/scopes (reports,
    diagrams, a second operating-entity scope) are **in scope by design** — the registry is the
    mechanism that scope decision anticipated.
  - **DECISIONS 10/17 (LOCKED — bank-owned Vertex for internal control text).** Confidential report
    findings and internal diagrams are content sent to **Gemini-vision / the parser on the bank's
    own** GCP Vertex by default. If an adopter policy requires it, the caption/parse seam swaps to
    the **self-hosted variant** (AI-GOVERNANCE §7) without changing the registry or graph-mapping
    shape ([RISKS](../../RISKS.md) R1).
- **External prerequisites:** for the **Mode A** quality work — the dev GCP project with Doc AI
  Layout Parser + Gemini-vision enabled in the configured region; sample report exports and
  diagram-bearing documents as fixtures. The **Mode B** offline path (fake parse/caption/embed seams)
  gates CI with no GCP call (LOCAL-DEV §4).

---

## 3. Workstreams

The M6 breakdown, three reviewable workstreams, dependency-ordered (IDs `M6-1 … M6-13`). The
registry-GA core comes first (every new kind rides it), then the two proof corpora — reports and
multimodal — land independently. Each task: **Size** XS–L · **Review** Light/Medium/Heavy · **Risk**
Low/Med/High.

| #   | Workstream                                | Tasks         | Theme                                                                             |
| --- | ----------------------------------------- | ------------- | --------------------------------------------------------------------------------- |
| 1   | [Registry GA](./01-registry-ga.md)        | M6-1 … M6-7   | descriptor + loader · fail-closed embed-space · `graph_role` · no-core-diff guard |
| 2   | [Reports corpus](./02-reports-corpus.md)  | M6-8 … M6-10  | `report` ingest · reports→graph (living audit map) · reports eval                 |
| 3   | [Multimodal diagrams](./03-multimodal.md) | M6-11 … M6-13 | vision-caption seam · verbalize + caption-embed · diagram retrieval + eval        |

---

## 4. Test & eval gates (this milestone)

- **Build/lint:** `go build`/`go test` green at Go 1.26; `golangci-lint run` (+ `modernize`) +
  gts/markdownlint clean; the regenerated registry/admin contract types are drift-clean (TOOLCHAIN
  §4; TESTING §1/§4).
- **Unit (offline = Mode B):** the **embed-space validator** (a mismatched `embed{}` fails closed),
  the `graph_role` edge-eligibility rule, the `metadata_config` validator, and the vision-caption
  seam are tested against fakes — no GCP call (TESTING §2, LOCAL-DEV §4; DECISIONS 1).
- **GA acceptance (the headline gate):** the **no-core-diff guard** adds a new corpus by descriptor
  and asserts **zero diff** to detector/graph/serve core; a second `local-*` scope routes `satisfies`
  with no new tier (registry-GA leak — [RISKS](../../RISKS.md)).
- **Integration (testcontainers, AlloyDB Omni):** the `report` corpus ingests end-to-end and its
  finding-nodes map into `relation_edge`; **RLS stays mandatory** — a confidential report finding or
  diagram caption is tier-tagged and a low-tier caller cannot read it through the join
  (DATA-GOVERNANCE §2, TESTING §2; [RISKS](../../RISKS.md) R2).
- **Contract:** the registry admin + MCP/REST surface validates against the published schemas; an
  un-regenerated change fails the build (API-CONTRACT §5, TESTING §4; [RISKS](../../RISKS.md) R8).
- **Eval (with the new corpora present):** retrieval recall@k / current-law floors hold, mapping
  precision/recall does not regress, and **caption retrieval** is scored against the golden set with
  the image ref kept for human verification (TESTING §5; [RISKS](../../RISKS.md) R5).

---

## 5. Risks (this milestone)

- **One-embedding-space drift** (M6-2) — a new corpus or caption embedded in a different model/dim
  fragments the shared space; the registry **fails closed** on a mismatched `embed{}` at
  registration and bars native image vectors (DECISIONS 1 LOCKED; [RISKS](../../RISKS.md) R12 — the
  headline).
- **"Registry GA" leaking core changes** (M6-6/M6-7) — if adding a corpus still needs a code edit,
  GA is not met; the **no-core-diff CI guard** is the acceptance test, not a nice-to-have.
- **Managed-AI policy drift on reports/diagrams** (M6-9/M6-12) — confidential report findings and
  internal diagrams reach Gemini-vision / the parser on the **bank's own** Vertex by default
  (DECISIONS 10/17); the caption/parse seam keeps a stricter adopter policy a config swap to the
  self-hosted variant (AI-GOVERNANCE §7), not a rebuild ([RISKS](../../RISKS.md) R1).
- **Caption hallucination** (M6-12/M6-13) — a vision caption can misdescribe a figure; keep the
  **image ref** for human verification, carry the full model record (AI-GOVERNANCE §9), and score
  caption retrieval against the golden set ([RISKS](../../RISKS.md) R5).
- **Reports-mapping precision** (M6-9) — a finding mapped to the wrong obligation/policy pollutes the
  living audit map; ground the mapping in the same evidence discipline as M3's detectors and gate it
  in the reports eval ([RISKS](../../RISKS.md) R5/R7).
