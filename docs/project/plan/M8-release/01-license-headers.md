# M8 WS1 — License & Headers

SPDX headers, copyright, dependency license audit, and CONTRIBUTING review.

---

## Tasks

### M8-1: SPDX headers

**Size** S · **Review** Light · **Risk** Low

Add `SPDX-License-Identifier: AGPL-3.0-only` and `Copyright (C) 2026 Danny Ota` to every source
and doc file under version control. Use a scripted pass (not manual). Add a CI-checkable script
that asserts every tracked file carries the header.

### M8-2: LICENSE file review

**Size** XS · **Review** Light · **Risk** Low

Verify `LICENSE` is the full AGPL-3.0 text. Verify the copyright line names the sole holder.
Verify `CONTRIBUTING.md` requires copyright assignment (no external contributions merged without
assignment — preserving the dual-license option).

### M8-3: Dependency license audit

**Size** S · **Review** Medium · **Risk** Med

Audit all direct and transitive dependencies (Go modules, npm packages) for license
compatibility. All must be permissive-only (MIT, Apache-2.0, BSD-2/3, ISC, MPL-2.0). Document
the audit in `docs/engineering/LICENSES.md`. Flag any non-permissive dependency for replacement.

### M8-4: DELIVERY-MODEL dual-license review

**Size** XS · **Review** Light · **Risk** Low

Verify DELIVERY-MODEL §1 accurately describes the dual-license posture: AGPL-3.0 public release,
sole copyright holder, commercial license possible under existing terms. No changes needed if
the current text is accurate — just confirm.
