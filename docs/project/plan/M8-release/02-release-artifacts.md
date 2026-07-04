<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# M8 WS2 — Release Artifacts

README, CHANGELOG, repo hygiene, and the v0.1.0 tag + GitHub release.

---

## Tasks

### M8-5: README for adopters

**Size** M · **Review** Medium · **Risk** Low

Rewrite root `README.md` for the release audience: an enterprise evaluating mise. Structure:
what mise is (one paragraph), license (AGPL-3.0 + dual-license note), status (v0.1.0 — design
spec, code scaffolding next), link to `docs/` for the full spec, link to CONTRIBUTING.md. Keep
the 🍱 signature emoji on the H1 title. Remove or update any pre-release internal notes.

### M8-6: CHANGELOG

**Size** S · **Review** Light · **Risk** Low

Create `CHANGELOG.md` with a `v0.1.0` entry. Summarize M0–M6 capabilities in a few bullet
points per milestone — what the spec covers, not implementation detail. Follow
[Keep a Changelog](https://keepachangelog.com/) format.

### M8-7: Repo hygiene

**Size** S · **Review** Light · **Risk** Low

Audit `.gitignore` for completeness: build artifacts, IDE files (`.idea/`, `.vscode/`), local
env (`.env*`), working-state files (`PLAN.md`). Verify no secrets, credentials, sample data with
real names, or internal paths are committed. Remove any stale files that should not ship.

### M8-8: Tag & GitHub release

**Size** XS · **Review** Light · **Risk** Low

Create annotated tag `v0.1.0` on a clean, lint-passing tree. Create a GitHub release from the
tag with release notes: link to CHANGELOG, link to `docs/`, clear statement that this is a
design-doc release (spec v0.1.0, code scaffolding follows). Mark as pre-release if GitHub
supports it and the team prefers signaling that.
