# Mise — M1 · Workstream 4: Embed · index · serve · eval

Embed all normalized sections in the single locked 1536-d space, build per-corpus indexes, expose
`search`/`document` over MCP, and run the first retrieval eval gate. Part of
[Milestone M1](./README.md).

See also:

- [M1 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §3/§6
- [DATA-MODEL](../../../design/DATA-MODEL.md) §1/§2/§3
- [API-CONTRACT](../../../design/API-CONTRACT.md) §2/§5
- [TESTING](../../../engineering/TESTING.md) §4/§5
- [DECISIONS](../../DECISIONS.md) 1/10/14/17
- Previous workstream: [Metadata & ownership](./03-metadata-ownership.md)

---

## 1. Tasks

Each row = one reviewable PR. The embedding call site is locked (DEC 14, in-app Go embedder) — the
contract every row here inherits is one 1536-d `gemini-embedding-001` space for every corpus.

| ID    | Task                                                                                                                                      | Deliverable / done-when                                                                                                 | Design ref                       | Depends on  | Size | Review | Risk |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- | -------------------------------- | ----------- | ---- | ------ | ---- |
| M1-21 | **Embed all sections @1536-d (reuse→adapt):** run normalized law and internal sections through the M0 embedder seam                       | every `section.embedding` is `gemini-embedding-001` @1536-d; non-1536 vector fails the ingest/index path                | DATA-MODEL §1 · DEC 1/14         | M1-8, M1-20 | M    | Medium | Med  |
| M1-22 | **AlloyDB ScaNN indexes per corpus (new):** create per-corpus vector indexes and hybrid retrieval support                                 | ScaNN indexes build for all five corpora; query path supports vector + lexical filters and validity-aware law retrieval | ARCH §3 · DATA-MODEL §3          | M1-21       | M    | Heavy  | Med  |
| M1-23 | **Per-corpus MCP `search`/`document` (reuse→adapt):** publish read-only tools returning verbatim text, citation, provenance, and validity | responses validate against JSON Schemas; RLS role applied; in-force law is the default retrieval view                   | API-CONTRACT §2/§5 · DATA-GOV §2 | M1-22       | M    | Heavy  | High |
| M1-24 | **Ingest workflow + retrieval eval (reuse→adapt):** orchestrate full ingest and score inherited retrieval floors                          | end-to-end workflow green; recall@k/MRR/current-law/abstention/citation floors pass on the VN/Malay golden set          | TESTING §5 · ARCH §3             | M1-23       | L    | Heavy  | High |

---

## 2. Notes

- **DEC 14 is locked: in-app Go embedder**, not DB-side `google_ml.embedding()`. The embedder
  interface still isolates the call site, but the user-visible contract (the shared 1536-d space)
  was never in question either way.
- **M1-23 is the first serving contract.** Later reasoning and web consumers depend on this
  evidence surface returning verbatim text plus citation/provenance under the caller's RLS role.
- **Retrieval eval is inherited from the existing engine.** M1 gates current-law precision and
  citation quality before cross-corpus mapping exists; mapping precision waits for M3.
- **Managed AI exposure is already settled by DEC 10/17.** Internal embeddings run in the
  adopter's GCP reference deployment unless the adopter selects the self-hosted policy variant.
