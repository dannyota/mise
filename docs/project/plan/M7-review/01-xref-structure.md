# M7 WS1 — Cross-Reference & Structure

Link integrity, doc ownership, terminology, customer-neutrality, and the README index. Every task
is a single reviewable pass.

---

## Tasks

### M7-1: Link & section-reference audit

**Size** S · **Review** Medium · **Risk** Low

Scan every `[text](path)` link and every `§N` / `DECISIONS N` citation across the full doc set.
Fix dead links, stale section numbers, and incorrect decision references. Verify that the
`CLAUDE.md` ownership table matches actual doc locations.

### M7-2: Ownership dedup

**Size** S · **Review** Medium · **Risk** Low

Identify content duplicated across docs (same concept explained in two places). Replace the
non-owning copy with a cross-reference to the owning doc. The ownership table in `CLAUDE.md` is
authoritative.

### M7-3: Terminology consistency

**Size** S · **Review** Light · **Risk** Low

Audit key terms across the doc set for uniform usage. Build a pass list: corpus, tier, access_tier,
evidence, finding, edge, promoted, attested, descriptor, obligation, provision, section, node_ref,
doc_ref, graph_role, metadata_config, medallion, seam, fake/real mode. Fix synonyms and ambiguities.

### M7-4: Customer-neutral audit

**Size** XS · **Review** Light · **Risk** Low

Grep for real bank names, customer names, host names, or internal references that should be
placeholders. Replace with `<tenant>`, `vn`/`my`, or "the reference deployment."

### M7-5: README index sync

**Size** XS · **Review** Light · **Risk** Low

Verify `docs/README.md` and root `README.md` list every doc with correct links and accurate
one-line descriptions. Add any missing entries (M7/M8 plans, any doc added during M3–M6 that was
not indexed).
