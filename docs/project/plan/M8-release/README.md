# Mise — Milestone M8: Release v0.1.0

Ship the **first public release** of mise as open-source software under **AGPL-3.0**. The
design-doc set is reviewed (M7) and this milestone prepares the repo, licensing, and release
artifacts for an enterprise to evaluate, build from source, and self-host — and for the author to
retain a clean path to a future commercial/managed distribution under the existing dual-license
(DELIVERY-MODEL §1, DECISIONS 9).

This is a **release milestone — no new design surface.** It produces release artifacts, not
features.

See also:

- [PLAN](../../PLAN.md)
- [ROADMAP](../../ROADMAP.md)
- [DELIVERY-MODEL](../../../engineering/DELIVERY-MODEL.md)
- [LICENSES](../../../engineering/LICENSES.md)
- [CI-CD](../../../engineering/CI-CD.md)
- Previous: [Milestone M7](../M7-review/README.md)

---

## 1. Scope & exit (Definition of Done)

- **Outcome:** the `dannyota/mise` repo is published as a **v0.1.0 tagged release** on GitHub,
  under AGPL-3.0, with all the artifacts an enterprise needs to evaluate and adopt.
- **Exit criteria (DoD):**
  - **License posture locked** — `LICENSE` is AGPL-3.0; `CONTRIBUTING.md` requires sole-author
    copyright assignment (no external contributions merged); the dual-license door is documented
    in DELIVERY-MODEL §1. Every source file carries the SPDX header `SPDX-License-Identifier:
AGPL-3.0-only`.
  - **Copyright & headers** — copyright line is `Copyright (C) 2026 Danny Ota` (sole holder);
    every source and doc file carries the header.
  - **Dependency license audit** — all direct and transitive dependencies are
    **permissive-only** (MIT, Apache-2.0, BSD-2/3, ISC, MPL-2.0); no GPL/LGPL/AGPL dependency
    (the project itself is AGPL, but dependencies must not impose additional copyleft on
    adopters beyond what AGPL already requires). LICENSES.md documents the audit.
  - **README for adopters** — root `README.md` covers: what mise is, license, quick-start
    (build from source → local stack → first corpus), link to `docs/` for the full spec.
  - **CHANGELOG** — `CHANGELOG.md` with a `v0.1.0` entry summarizing M0–M6 capabilities.
  - **Git tag** — `v0.1.0` tag on the release commit, annotated.
  - **GitHub release** — a GitHub release from the tag with a release-notes body linking the
    changelog and the doc set.
  - **Repo hygiene** — `.gitignore` covers build artifacts, IDE files, local env; no secrets,
    credentials, or internal paths in the repo; `PLAN.md` (working state) is gitignored.
  - **No code yet (design-doc release)** — this is a **spec release**: the deliverable is the
    reviewed design-doc set, not running software. The version signals "this spec is stable
    enough to build from." Code scaffolding begins after release.

---

## 2. Workstreams

Two workstreams: licensing/headers, then release prep.

| #   | Workstream                                     | Tasks       | Theme                                                   |
| --- | ---------------------------------------------- | ----------- | ------------------------------------------------------- |
| 1   | [License & headers](./01-license-headers.md)   | M8-1 … M8-4 | SPDX headers, copyright, dep audit, CONTRIBUTING review |
| 2   | [Release artifacts](./02-release-artifacts.md) | M8-5 … M8-8 | README, CHANGELOG, repo hygiene, tag + GitHub release   |

---

## 3. License posture

**AGPL-3.0-only** — the right license for this stage:

- **Enterprise-safe for self-host:** an enterprise that builds from source and runs internally
  is not distributing or offering a network service to third parties — AGPL's copyleft does not
  trigger. The adopter can evaluate, modify, and deploy without publishing its modifications,
  as long as it does not offer mise as a service to external users.
- **Startup-safe for the author:** the sole copyright holder can offer a **commercial license**
  (signed releases, support, hosted offering) under different terms at any time — the
  dual-license model documented in DELIVERY-MODEL §1. No external contributor holds copyright,
  so no re-licensing negotiation is needed.
- **Discourages competitors from reselling:** AGPL prevents a third party from wrapping mise
  in a proprietary SaaS without releasing their modifications — the standard open-core
  protection.

---

## 4. Test gates (this milestone)

- **Lint:** full doc set passes Prettier + markdownlint with zero errors.
- **License headers:** every `.go`, `.ts`, `.vue`, `.md` file under version control carries the
  SPDX header (scripted check).
- **Dependency audit:** `LICENSES.md` lists every direct dependency with its license; no
  non-permissive dependency.
- **Link validation:** all README/doc links resolve.
- **Tag:** `git tag -a v0.1.0` succeeds on a clean tree.

---

## 5. Risks (this milestone)

- **Premature version signal** — v0.1.0 on a spec-only repo may set expectations of runnable
  software; the README and release notes must clearly state this is a **design-doc release**, not
  a code release.
- **Dependency license surprise** — a transitive dependency with a non-permissive license could
  block adopter use; the audit in M8-3 catches this before release.
- **Copyright header churn** — adding SPDX headers to every file is mechanical but touches every
  file; do it in one commit to keep the diff reviewable.
