# Mise — M2 · Workstream 2: Extraction (Method A)

Extract explicit internal control-chain edges from the parsed doc-control envelope:
`local-sop → local-policy` and `local-policy → group-std`. This is deterministic extraction with
no model call and no guessing. Part of [Milestone M2](./README.md).

See also:

- [M2 overview](./README.md)
- [DATA-MODEL](../../../design/DATA-MODEL.md) §2/§4/§5
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §4
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2
- [DECISIONS](../../DECISIONS.md) 6
- Previous workstream: [Graph schema](./01-graph-schema.md)
- Next workstream: [Graph API](./03-graph-api.md)

---

## 1. Tasks

Each row = one reviewable PR. Method A reads the M1 metadata envelope only; an absent or
unparseable header produces no edge.

| ID   | Task                                                                                                                     | Deliverable / done-when                                                                                                 | Design ref       | Depends on  | Size | Review | Risk |
| ---- | ------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- | ---------------- | ----------- | ---- | ------ | ---- |
| M2-5 | **Control-reference parser (new):** parse explicit "implements/derives from" references from the M1 doc-control envelope | parser returns target refs and quoted spans; absent header returns an empty result, not a guessed edge                  | DATA-MODEL §2/§5 | M1-16       | M    | Heavy  | High |
| M2-6 | **Reference resolver (new):** resolve parsed references to graph node refs or `doc_ref` stubs                            | resolved target is a corpus node when ingested, otherwise a `doc_ref` stub; unresolved ambiguity routes to review/error | DATA-MODEL §4    | M2-5, M2-1  | M    | Heavy  | Med  |
| M2-7 | **Extracted edge writer (new):** write `implements`/`derives` edges with `evidence_kind=extracted` and quoted spans      | Normalize writes idempotent edges; unchanged rerun creates no duplicates                                                | DATA-MODEL §4/§5 | M2-6, M2-2  | M    | Medium | Med  |
| M2-8 | **Attestation owner link (new):** attach extracted evidence to the durable `org_role` owner from M1                      | `relation_evidence.created_by`/owner metadata points to durable role identity; leaver-history test remains valid        | DATA-MODEL §2/§4 | M2-7, M1-18 | S    | Medium | Low  |
| M2-9 | **Normalize integration (new):** wire Method A into internal Normalize for SOP/policy/group transitions                  | internal Normalize emits the expected control-chain edges for fixtures; unit test asserts no model/judge call occurs    | ARCH §4 · DEC 6  | M2-7        | M    | Medium | Med  |

---

## 2. Notes

- **Method A is extraction, not inference.** If the document does not explicitly point to its
  upstream control, M2 writes no edge. The judge-based `satisfies` path is M3.
- **M2-5/M2-6 are split because parsing and resolving fail differently.** A ragged header is a
  source-template issue; an ambiguous target is a registry/linking issue.
- **No managed AI call happens here.** This milestone uses metadata already parsed in M1 and
  deterministically writes edges into the graph.
- **Extracted evidence is still evidence.** The `relation_evidence` row preserves the quoted spans
  and role owner so later review/audit can explain why the edge exists.
