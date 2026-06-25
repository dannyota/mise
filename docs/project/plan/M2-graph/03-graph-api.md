# Mise — M2 · Workstream 3: Graph API

Expose the graph spine as a tier-filtered read API: repository queries, bounded control-chain
walks, the MCP `graph` tool, and REST endpoints for nodes and chains. Part of
[Milestone M2](./README.md).

See also:

- [M2 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §4/§6
- [API-CONTRACT](../../../design/API-CONTRACT.md) §2/§3/§5
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2
- [TESTING](../../../engineering/TESTING.md) §4
- Previous workstream: [Extraction (Method A)](./02-extraction-method-a.md)

---

## 1. Tasks

Each row = one reviewable PR. The API is read-only in M2; promotion/rejection writes arrive in M3.

| ID    | Task                                                                                                             | Deliverable / done-when                                                                                                | Design ref                   | Depends on | Size | Review | Risk |
| ----- | ---------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- | ---------------------------- | ---------- | ---- | ------ | ---- |
| M2-10 | **Graph read repository (new):** query nodes, edges, evidence, and `doc_ref` targets under the caller's RLS role | repository returns tier-filtered graph rows with verbatim evidence refs and no app-side tier override                  | DATA-MODEL §4 · DATA-GOV §2  | M2-4, M2-9 | M    | Heavy  | High |
| M2-11 | **Bounded chain walk (new):** build SOP→Policy→Group chain traversal with max depth and cycle guard              | chain walk returns ordered hops; malformed/cyclic graph terminates with a bounded error                                | ARCH §4                      | M2-10      | S    | Medium | Med  |
| M2-12 | **MCP `graph` tool schema (new):** define request/response schema for nodes, edges, evidence, and chain output   | JSON Schema published; response includes citation/provenance per hop and hides denied rows                             | API-CONTRACT §2/§5           | M2-10      | S    | Medium | Med  |
| M2-13 | **REST graph endpoints (new):** `GET /graph/nodes/{ref}` and `GET /graph/chain/{ref}`                            | endpoints return generated contract types and RFC 9457 errors; lower-tier caller sees only permitted graph rows        | API-CONTRACT §3/§5           | M2-12      | M    | Heavy  | Med  |
| M2-14 | **Contract generation + provider tests (new):** generate OpenAPI/JSON Schema from Go and test provider responses | un-regenerated schema drift fails the build; MCP + REST provider fixtures validate                                     | API-CONTRACT §5 · TESTING §4 | M2-13      | S    | Medium | Med  |
| M2-15 | **Graph cross-tier deny test (new):** prove graph joins cannot leak stricter-tier nodes or edges                 | real AlloyDB Omni test: low-tier caller cannot read/traverse confidential edges through node, edge, or chain endpoints | DATA-GOV §2 · TESTING §2     | M2-14      | M    | Heavy  | High |

---

## 2. Notes

- **M2-10 and M2-15 are the confidentiality boundary for graph reads.** The repository must run
  under the caller's RLS role; the deny test proves no REST/MCP path can join around that boundary.
- **The chain walk is deliberately bounded.** The graph should normally be shallow
  SOP→Policy→Group, but the implementation must survive malformed/cyclic data.
- **Contract generation starts here.** M4 and M5 consume the same graph contract, so provider
  drift must fail before consumers start hand-mirroring types.
- **The API is read-only.** Human promotion/rejection and relink writes are part of the M3 review
  queue, not this graph-spine milestone.
