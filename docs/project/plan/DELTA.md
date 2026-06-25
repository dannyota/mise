# Mise — Build Delta

The reuse · change · new delta from the existing banhmi/laksa engine to mise. This file owns the
build-shape delta; implementation sequence is [PHASES](./PHASES.md).

See also:

- [PLAN](../PLAN.md) (build-plan index)
- [ARCHITECTURE](../../design/ARCHITECTURE.md)
- [DELIVERY-MODEL](../../engineering/DELIVERY-MODEL.md)
- [COST](../COST.md)

---

## 1. Reuse

Reuse the open-source banhmi/laksa engine, same author:

- Temporal medallion pipeline.
- pgvector-compatible store patterns.
- validity model.
- intra-corpus relations.
- evidence-only MCP surface.
- ingest ledger.

---

## 2. Change

Upgrade the engine for mise:

- Parser → Doc AI Layout Parser.
- Embedder → `gemini-embedding-001`.
- Store → AlloyDB Omni + ScaNN, while keeping pgvector-compatible code paths where possible.
- Serve reasoning → Claude Agent SDK in the separate reasoning endpoint.

---

## 3. New

Build mise-specific product surface:

- corpus registry.
- compliance graph.
- four cross-corpus detectors.
- reasoning endpoint.
- Vue Web UI.
- auth/access tiers.
