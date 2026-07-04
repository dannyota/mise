<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.0] — 2026-07-04

First public release — a **design-doc spec**, not running software. The version signals "this spec
is stable enough to build from." Code scaffolding begins after this release.

### Added

- **M0 Skeleton** — monorepo layout, Go module (`danny.vn/mise`), AlloyDB Omni schema-per-corpus,
  Temporal worker, Vertex/embed/judge/ground seams with Mode B (fakes), local Podman stack.
- **M1 Ingest** — law ingest (VN + MY public regulation via banhmi/laksa crawlers), Document AI
  parsing, medallion pipeline (Discover→Fetch→Parse→Normalize→Embed→Index), metadata envelope,
  org_role ownership, retrieval eval harness (recall@k, MRR@k, current-law, abstain, citation).
- **M2 Graph spine** — `relation_edge`/`relation_evidence` in the `graph` schema, tier-inheritance
  (GENERATED stricter-of-two), graph RLS, Method-A extraction (internal edges: `implements`,
  `derives`), graph read API (MCP `graph` tool + REST).
- **M3 Detectors** — RelationDetect (embed→ANN→rerank→Gemini judge→Check Grounding→write),
  ConflictDetect (Group-std vs law contradiction), GapScan, StaleScan, findings schema with
  tier-inherited RLS, review-queue API (promote/reject/relink), mapping eval baseline.
- **M4 Audit Q&A** — Claude Agent SDK reasoning endpoint (Haiku 4.5 / Sonnet 4.6 on Vertex),
  MCP evidence tools, SSE streaming, cited answers with grounding + abstain, Q&A eval harness.
- **M5 Web UI** — Vue 3.5 SPA (Vite + Vue Flow/elkjs + TanStack Table + reka-ui/Tailwind 4):
  Dashboard, Graph Explorer, Review Workbench, Findings & Resolution, Q&A chat, Notifications,
  Coverage Report, Change Timeline, Admin.
- **M6 Scale** — corpus registry GA (add a corpus by descriptor alone with zero core change),
  reports corpus (audit/risk → graph), multimodal diagram path (Layout Parser + Gemini vision
  caption → embed the caption text in the shared 1536-d space).
- **M7 Design review** — full consistency audit: cross-references, terminology, decisions,
  diagrams, schemas, ownership dedup, lint.
- **M8 Release** — SPDX headers, LICENSE (AGPL-3.0), CONTRIBUTING, CHANGELOG, repo hygiene.
