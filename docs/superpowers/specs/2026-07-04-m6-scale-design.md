# M6: Scale — Registry GA + Reports Corpus + Multimodal Diagrams

## Goal

Make the corpus registry **GA** — a new corpus ships as a descriptor + source plugin with **no
core change** — and prove it by landing two new corpus kinds: a `report` corpus whose
finding-nodes map into the graph (living audit map), and a `diagram` multimodal path that
verbalizes figures/images (caption text embedded @1536-d, image ref kept). All under the single
embedding space invariant (DECISIONS 1), enforced fail-closed at registration.

## Architecture

M6 generalizes existing code; it adds **no new subsystem**. Three workstreams:

1. **Registry GA (M6-1 → M6-7):** Formalize `corpus.Descriptor` as the single registration
   surface, add `MetadataConfig` field, embed-space validator (fail-closed), `graph_role`
   enforcement at edge write, registry admin REST/MCP surface, no-core-diff CI guard, and a
   descriptor-only new scope.

2. **Reports corpus (M6-8 → M6-10):** A `report` kind ingests through the medallion via its
   source plugin. Finding-nodes map into `relation_edge` through `graph_role`'s `default_edges`,
   turning the graph into a living audit map. Eval floors hold.

3. **Multimodal diagrams (M6-11 → M6-13):** Vision-caption seam (interface + fake for Mode B),
   real Gemini-vision adapter on bank's own Vertex, Layout Parser extracts figures, caption text
   embeds @1536-d, image ref kept for human verification. Caption retrieval scored.

## Components

### 1. Corpus descriptor model (`pkg/corpus`)

Extend `Descriptor` with `MetadataConfig` (per-source config shape — defaults + parse locations).
Add `Register(d Descriptor) error` that validates:

- `embed{model,dims,task_type}` matches the locked space (fail-closed)
- `graph_role` is internally consistent
- `metadata_config` schema is valid
- `id` doesn't collide with existing

Add `KindReport Kind = "report"` and `KindDiagram Kind = "diagram"`.

### 2. Embed-space validator (`pkg/corpus`)

`ValidateEmbedSpace(e EmbedConfig) error` — compares against `sharedEmbed`. Rejects any
mismatch on model, dims, or task_type. Bars native image vectors (any embed whose task_type
indicates image embedding). Called by `Register` before any ingest.

### 3. Graph role enforcement (`pkg/store`)

`WriteExtractedEdge` and `WriteCandidateEdge` already validate `from_corpus_id` is registered.
Add: check `graph_role.CanSource` on from-corpus and `graph_role.CanTarget` on to-corpus. Refuse
the edge if either fails. This makes the descriptor's `graph_role` the single authority for which
edges are allowed — no core code change needed per corpus.

### 4. MetadataConfig validation (`pkg/corpus`)

```go
type MetadataConfig struct {
    Defaults       map[string]string
    ParseLocations map[string]string
}
```

Validated at registration: keys must be non-empty strings, values must match known parse-location
patterns. The 5 existing corpora get their configs added to the registry.

### 5. Registry admin REST surface (`pkg/httpapi`)

New operations on the huma API:

- `GET /registry` — list all registered corpus descriptors
- `GET /registry/{id}` — get one descriptor
- `POST /registry` — register a new corpus (admin-only)

Contract types added to `@mise/contract` and the generated OpenAPI spec. The M5 Corpus Admin
screen already exists — the registry surface extends it.

### 6. No-core-diff CI guard (`scripts/`)

A shell script that:

1. Adds a fixture corpus by descriptor (calls `Register`)
2. Asserts zero diff in detector/graph/serve core files
3. Fails CI if any core file changed

This is the GA acceptance test.

### 7. New operating-entity scope

A second `local-*` scope (e.g. `local-policy-my`) with `jurisdiction: "my"`. Routes `satisfies`
correctly via the existing `GraphRole.SatisfiesTarget` field. The no-core-diff guard stays green.

### 8. Report corpus descriptor + ingest (`pkg/ingest`)

A `report` source plugin that ingests audit/risk reports through the medallion pipeline. Tier-
tagged at write. Uses the existing `discover → process → index` stages.

### 9. Reports→graph mapping

Report finding-nodes map into `relation_edge` via `graph_role.default_edges`. A finding creates
an edge to the obligation/policy it concerns. Tier inherits stricter-of-two (DB-generated column).

### 10. Vision caption seam (`pkg/vertex`)

```go
type Captioner interface {
    Caption(ctx context.Context, image []byte, contentType string) (CaptionResult, error)
}

type CaptionResult struct {
    Text      string
    Model     string
    Prompt    string
    CreatedAt time.Time
}
```

Fake implementation returns canned captions. Real implementation calls Gemini-vision on the
bank's own Vertex. Model record logged per AI-GOVERNANCE §9.

### 11. Diagram verbalize + caption embed

Layout Parser extracts figures/tables from documents. Gemini-vision captions them. The caption
**text** (not image vector) embeds @1536-d in the locked space. The image ref (blob path) is
kept alongside the section for human verification.

### 12. Caption retrieval + eval

Caption chunks are retrievable in the shared embedding space. Scored against the golden set.
Image ref surfaced in search results for verification.

## Data flow

```
report doc → medallion → sections → embed @1536-d → index
                ↓
          finding extraction → graph_role.default_edges → relation_edge
                                                              ↓
                                                    living audit map

diagram doc → Layout Parser → figure extraction
                                    ↓
                            Gemini-vision caption
                                    ↓
                            caption text → embed @1536-d → index
                            image ref → blob store (kept for verification)
```

## Error handling

- **Embed space mismatch:** `Register` returns error, no ingest starts.
- **Graph role violation:** `WriteExtractedEdge`/`WriteCandidateEdge` return
  `ErrGraphRoleViolation`.
- **Caption failure:** Gemini-vision errors are retried once, then the figure is skipped with a
  warning log. The document continues ingesting without that figure's caption.
- **Invalid metadata config:** `Register` returns error, corpus rejected.

## Testing

- **Unit (Mode B):** embed-space validator, graph_role enforcement, metadata_config validation,
  vision-caption seam against fakes — no GCP call.
- **GA acceptance:** no-core-diff guard adds a fixture corpus and asserts zero core diff.
- **Integration (testcontainers):** report corpus ingests end-to-end, finding-nodes map into
  edges, RLS stays mandatory.
- **Contract:** registry admin surface validates against published schemas.
- **Eval:** retrieval/mapping floors hold with new corpora present; caption retrieval scored.

## Constraints

- **Single embedding space (DECISIONS 1 LOCKED):** fail-closed at registration. No native image
  vectors.
- **Bank's own Vertex (DECISIONS 10/17 LOCKED):** confidential content on bank's GCP.
- **Go 1.26, golangci-lint v2** with modernize linter.
- **Descriptor-only GA:** if adding a corpus needs a core code change, GA is not met.
- **All-banking-regulation scope (DECISIONS 8 LOCKED):** new kinds/scopes are in-scope by design.
