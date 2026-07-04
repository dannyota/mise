<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — M4 · Workstream 1: Serving MCP surface

Finalize the read-only evidence tools the reasoning endpoint will call: `search`, `document`, and
`graph`, each returning verbatim text, citations, provenance, and tier-filtered results. Part of
[Milestone M4](./README.md).

See also:

- [M4 overview](./README.md)
- [ARCHITECTURE](../../../design/ARCHITECTURE.md) §6
- [API-CONTRACT](../../../design/API-CONTRACT.md) §2/§5
- [DATA-GOVERNANCE](../../../design/DATA-GOVERNANCE.md) §2/§4
- [TESTING](../../../engineering/TESTING.md) §4
- Next workstream: [Endpoint core](./02-endpoint-core.md)

---

## 1. Tasks

Each row = one reviewable PR. These tools are evidence-only and read-only; the agent cannot write
through them.

| ID   | Task                                                                                                                          | Deliverable / done-when                                                                                             | Design ref                      | Depends on       | Size | Review | Risk |
| ---- | ----------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- | ------------------------------- | ---------------- | ---- | ------ | ---- |
| M4-1 | **Finalize MCP `search` (reuse→adapt):** validity-aware retrieval over all tier-permitted corpora with provenance             | tool returns ranked verbatim sections with citation path, source URL/provenance, validity, and corpus/tier metadata | API-CONTRACT §2 · DATA-MODEL §2 | M1-23            | M    | Heavy  | Med  |
| M4-2 | **Finalize MCP `document` (reuse→adapt):** fetch full document/section context by ref under RLS                               | tool returns verbatim text and metadata for permitted refs; denied refs are indistinguishable from not found        | API-CONTRACT §2 · DATA-GOV §2   | M1-23            | S    | Heavy  | High |
| M4-3 | **Finalize MCP `graph` (reuse→adapt):** expose control chain and relation evidence for the agent                              | tool returns tier-filtered nodes/edges/evidence and bounded chain output from M2                                    | API-CONTRACT §2 · DATA-MODEL §4 | M2-15            | M    | Heavy  | High |
| M4-4 | **MCP JSON Schema publish (new):** publish schemas for all evidence tools as the MCP-native contract                          | schemas generated from Go types; provider fixture responses validate; docs point consumers at generated package     | API-CONTRACT §5 · TESTING §4    | M4-1, M4-2, M4-3 | S    | Medium | Med  |
| M4-5 | **Evidence provenance contract test (new):** prove every tool response includes citation/provenance and no denied row content | cross-tier-deny provider test green; missing citation/provenance fails schema/test                                  | DATA-GOV §4 · TESTING §4        | M4-4             | M    | Heavy  | High |

---

## 2. Notes

- **The agent gets evidence, not authority.** These tools only read tier-scoped evidence; all
  write surfaces remain outside the reasoning loop.
- **Denied rows must not be observable.** A denied ref should not leak existence, title, or
  metadata through a different error shape.
- **Citation/provenance is part of the contract.** If a tool cannot return a citation path and
  source provenance, the answer path must abstain later.
- **M4-5 is the provider-side confused-deputy precheck.** M4-9 proves propagation through the
  reasoning endpoint; this workstream proves the tools themselves respect RLS.
