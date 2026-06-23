# Mise — Tool & Dependency Licenses

License posture for everything mise uses. **Bottom line:** every library we ship or link
is **permissive** (MIT / Apache-2.0 / BSD / ISC / PostgreSQL). The only copyleft is in
**stand-alone tools** used at arm's length — and the **one to avoid is Renovate (AGPL-3.0)**;
use Dependabot instead. Paid items (AlloyDB Omni, Vertex, GKE, M365) are managed services,
not OSS — costs in [COST.md](../project/COST.md).

See also: [CI-CD.md](./CI-CD.md) (the license gate that enforces this) ·
[TOOLCHAIN.md](./TOOLCHAIN.md) · [COST.md](../project/COST.md).

---

## 1. 🧭 The rule

- **Shipped code + linked deps:** permissive only (MIT · Apache-2.0 · BSD · ISC · PostgreSQL).
- **Copyleft is OK only at arm's length:** a GPL/LGPL/EPL **CLI tool** we _run_ (lint, scan)
  but neither link nor distribute imposes **no obligation** on our source. Verify per case.
- **AGPL is restricted in _dependencies_** (common bank policy) — avoid any AGPL dep we
  ship or link. _(This constrains the **dependency tree**, not mise's own license — mise
  itself ships under AGPL-3.0 by the owner's choice; see the README.)_
- **Enforced, not trusted:** a CI **license gate** (§4) fails the build on a disallowed
  license, so this list can't silently drift.

---

## 2. ⚖️ Copyleft — the items to track

| Tool                       | License      | How we use it                              | Verdict                                                                                                                                                                                             |
| -------------------------- | ------------ | ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **golangci-lint**          | **GPL-3.0**  | dev CLI, run over our code                 | **OK** — not linked/shipped; no obligation on our code                                                                                                                                              |
| **Semgrep CE**             | **LGPL-2.1** | SAST CLI, run standalone                   | **OK** — invoked as a tool, not linked                                                                                                                                                              |
| **elkjs**                  | **EPL-2.0**  | graph layout, **bundled into the web app** | **OK** — weak (file-level) copyleft, and elkjs's source is already public, so the only obligation (offer its source) is trivially met even under AGPL. Swap to **dagre** (MIT) if EPL is disallowed |
| **Renovate** (self-hosted) | **AGPL-3.0** | dependency-update bot                      | **AVOID** → use **Dependabot** (GitHub-native, no AGPL) or Mend's hosted app unmodified (CI-CD §4)                                                                                                  |

> golangci-lint bundles many analyzers (staticcheck MIT · gosec Apache-2.0 · revive MIT …)
> — these run inside the GPL CLI, not in our binaries.

---

## 3. 📚 Inventory (permissive + proprietary)

**Permissive — no concern** (MIT unless noted):

| Area                     | Tools                                                                                                                                                                                                                                                                            |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go core                  | Go _(BSD-3)_ · gofumpt _(BSD-3)_ · goimports/govulncheck _(BSD-3)_ · sqlc · pgx · chi · connect-go _(Apache-2.0)_ · Temporal                                                                                                                                                     |
| Go linters (in golangci) | staticcheck · revive · gosec _(Apache-2.0)_ · errcheck · bodyclose …                                                                                                                                                                                                             |
| TS/Vue                   | TypeScript _(Apache-2.0)_ · Node _(MIT)_ · pnpm · Vue · Vite · Rolldown · Tailwind · Pinia · vue-router · @vueuse · Vue Flow · TanStack Table · reka-ui · shadcn-vue · CVA _(Apache-2.0)_ · lucide _(ISC)_ · mermaid · Hono · zod · jose · pino · @anthropic-ai/claude-agent-sdk |
| Lint/format/test         | ESLint · typescript-eslint · eslint-plugin-vue · eslint-plugin-unicorn · Prettier · gts _(Apache-2.0)_ · knip _(ISC)_ · markdownlint-cli2 · lefthook · Vitest · Vue Test Utils · Playwright _(Apache-2.0)_                                                                       |
| Security/supply-chain    | osv-scanner _(Apache-2.0)_ · gitleaks · Trivy _(Apache-2.0)_ · syft · cosign/Sigstore _(Apache-2.0)_                                                                                                                                                                             |
| Infra/DB                 | KEDA _(Apache-2.0)_ · Helm _(Apache-2.0)_ · PostgreSQL + pgvector _(PostgreSQL License)_ · ScaNN _(Apache-2.0)_                                                                                                                                                                  |

**Proprietary / paid (managed services — expected, not OSS):**

| Item                                                                                     | Terms                                                              |
| ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| **AlloyDB Omni**                                                                         | Google commercial license — **free dev**, ~$40/vCPU-mo prod (COST) |
| **Vertex AI** (Gemini 3.5 Flash · Doc AI · Ranking · Check Grounding · Claude on Vertex) | Google managed, usage-priced                                       |
| **GKE**                                                                                  | Google managed                                                     |
| **Microsoft 365 / SharePoint / Graph / Azure AD**                                        | Microsoft commercial (bank already licenses)                       |

> Exact versions/licenses must be re-confirmed **at pin time** — the CI license gate (§4) is
> the source of truth, not this hand-maintained table.

---

## 4. ✅ Enforcement — the CI license gate

Automate it so it can't drift (CI-CD §4):

- **Go:** `go-licenses check ./...` against an allowlist (deny GPL/LGPL/AGPL in _module_
  deps; golangci-lint et al. are `tool`-directive CLIs, not module deps, so they're excluded).
- **JS:** `pnpm licenses list` / `license-checker` against the same allowlist.
- **Containers/SBOM:** Trivy + syft already emit per-package licenses (CI-CD §3) — fail on a
  disallowed license in any image layer.
- **Allowlist:** MIT · Apache-2.0 · BSD-2/3 · ISC · PostgreSQL · (EPL-2.0 only for elkjs, by
  exception). **Denylist:** AGPL-\*; GPL/LGPL allowed **only** for arm's-length CLI tools (§2).
