# Mise — Toolchain

Pinned language/tool versions and the **modern-runtime features mise deliberately
adopts** (Go 1.26 · Node 24 LTS · the JS/TS stack). This doc owns _versions + standards_;
the per-language idioms live in the CODE_STYLE docs, the pipeline in
[CI-CD.md](./CI-CD.md).

See also:

- [CODE_STYLE_GO.md](./CODE_STYLE_GO.md)
- [CODE_STYLE_TS.md](./CODE_STYLE_TS.md)
- [CODE_STYLE_VUE.md](./CODE_STYLE_VUE.md)
- [FOLDER_STRUCTURE.md](./FOLDER_STRUCTURE.md)
- [CI-CD.md](./CI-CD.md) (build/release)
- [TESTING.md](./TESTING.md)

---

## 1. Pinned versions

One version per tool, pinned in-repo (`go.mod` toolchain · `package.json#engines` +
`packageManager` · `.tool-versions`/`mise.toml`). CI fails on drift.

| Tool                  | Pin                                           | Where pinned                                     |
| --------------------- | --------------------------------------------- | ------------------------------------------------ |
| **Go**                | **1.26.x** (`go 1.26` + `toolchain go1.26.x`) | `go.mod`                                         |
| **golangci-lint**     | **v2.x**                                      | `go.mod` `tool` directive (§2) · `.golangci.yml` |
| **sqlc**              | latest 1.x                                    | `go.mod` `tool` directive                        |
| **Node**              | **24 LTS** (`>=24.0`)                         | `package.json#engines`, `.tool-versions`         |
| **pnpm**              | **11.x**                                      | `package.json#packageManager` (corepack)         |
| **TypeScript**        | **6.0.x**                                     | root devDep, single version across the workspace |
| **Vite**              | **8.x** (Rolldown bundler)                    | `apps/web` devDep                                |
| **Vue**               | **3.5.x**                                     | `apps/web`                                       |
| **Tailwind**          | **4.x**                                       | `apps/web`                                       |
| **Vitest**            | **4.x**                                       | both apps                                        |
| **typescript-eslint** | **v8.x** (flat config)                        | root                                             |
| **Prettier**          | 3.x                                           | root                                             |
| **lefthook**          | latest                                        | root (pre-commit, §5)                            |

> Rule: **one TypeScript + one Vitest + one ESLint** version for the whole pnpm
> workspace (hoisted to root) so `web` and `reasoning` can't drift.

---

## 2. Go — toolchain & dev tools

- **`go 1.26` + `toolchain go1.26.x`** in `go.mod` — every machine/CI builds with the
  same compiler (auto-downloaded). Bump deliberately (a reviewed change).
- **Dev tools via the `tool` directive** (Go 1.24+) — `golangci-lint`, `sqlc`, `migrate`
  are declared in `go.mod` and run with `go tool <name>`. No "go install latest" drift;
  the tool versions are part of the lockfile.

```go
// go.mod (excerpt)
go 1.26

tool (
	github.com/golangci/golangci-lint/v2/cmd/golangci-lint
	github.com/sqlc-dev/sqlc/cmd/sqlc
)
```

---

## 3. Go 1.26 — features mise adopts

These are not "nice to know" — each maps to something mise does. (Versions noted; 1.24/1.25
features are baseline given the 1.26 pin.)

| Feature (since)                                      | Adopt for                                                                                                                                                                                                                            |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Container-aware `GOMAXPROCS`** (1.25)              | Everything runs on **GKE with CPU limits + KEDA**. The runtime reads the cgroup quota and re-reads it as limits change — **no `automaxprocs`, no manual tuning**. Document that we depend on it; don't override `GOMAXPROCS`.        |
| **`testing/synctest`** GA (1.25)                     | Deterministic, virtual-time tests for the **Temporal worker pools / goroutine fan-out** and any timeout/backoff logic — pairs with the `-race` rule (TESTING §2).                                                                    |
| **`encoding/json/v2`** (1.25, `GOEXPERIMENT=jsonv2`) | The ingest path is JSON-heavy (Doc AI · Graph delta · Vertex responses). Faster decode **and** correctness (no `float64`-by-default, stricter unmarshal). **Decision:** opt-in behind a build tag now; promote when it ships stable. |
| **Green Tea GC** default (1.26)                      | Page-scanning GC favours the heap-walking retrieval/vector workload — adopt the default, note it as the GC baseline for the latency budget (OBSERVABILITY §4).                                                                       |
| **revamped `go fix`** (1.26)                         | Push-button idiom/API modernization — run in CI as a **drift check** (CI-CD §2).                                                                                                                                                     |
| **`new(expr)`** (1.26)                               | Ergonomic init of the many **nullable metadata pointer fields** in the data model (`*time.Time`, `*string` for `expiry_date`, `superseded_by_id`, …).                                                                                |
| **`os.Root`** (1.24)                                 | Sandbox **GCS raw-file** staging and the local **folder+manifest** loader to a directory — defense-in-depth against path traversal in ingest.                                                                                        |
| **heap-address randomization, cgo −30%** (1.26)      | Free security/perf; no code change.                                                                                                                                                                                                  |

---

## 4. Lint & format — golangci-lint v2

- **One linter runner**, config `version: "2"` at repo root (`.golangci.yml`). v2 splits a
  dedicated **`formatters:`** block (run via `golangci-lint fmt`) from `linters:`.
- **`golangci-lint run`** in CI is a hard gate; **`golangci-lint fmt`** replaces ad-hoc
  `gofmt`/`goimports` invocations (gofumpt + goimports, with `local-prefixes` for import
  grouping).
- Baseline set beyond `standard` (errcheck · govet · ineffassign · staticcheck · unused):
  **errorlint · gosec · bodyclose · noctx · contextcheck · rowserrcheck · sqlclosecheck ·
  nilerr · prealloc · revive · gocritic · misspell · unconvert · unparam · whitespace**.
  The bank-relevant ones are `gosec` (security) and the `sql*`/`bodyclose` resource-leak
  linters (the read path holds DB rows + HTTP bodies). `gocritic` catches subtle bugs;
  `misspell`/`unconvert`/`unparam`/`whitespace` are low-noise hygiene (proven in s1ctl).
- **gosec runs with no excludes today** (`.golangci.yml` — zero findings to date). Add an
  exclude only with a written rationale comment when a real false positive lands, not
  preemptively; a path-validated read still needs an inline `//nolint:gosec` + reason at the
  call site (e.g. `blob.FS`, which checks key shape before ever joining a path — see its code).
  `os.Root` (1.24, TOOLCHAIN §3) remains the idiom for future file-handling code that needs
  directory-level sandboxing, not a blanket G304 exclude.
- **revive tuning:** disable `exported` (noisy godoc enforcement on internal types);
  keep `var-naming` + `file-length-limit`.
- Generated code (`*.sql.go` from sqlc) is excluded; `_test.go` relaxes
  `gosec`/`noctx`/`errcheck`/`unparam`.

### Modernization — detect + fix Go 1.26 idioms

The linter, not just the docs, keeps code current. golangci-lint v2 (≥ v2.6) bundles the
gopls **`modernize`** analyzer plus `usestdlibvars · intrange · copyloopvar · perfsprint ·
sloglint` — all enabled in `.golangci.yml`:

- **`modernize` is go-version-aware:** it reads `go 1.26` from `go.mod` and flags
  behavior-preserving upgrades to **1.26 idioms** — `min`/`max`, `slices`/`maps`,
  `any` over `interface{}`, `for range N`, `strings.CutPrefix`, `new(expr)`, etc. Bumping
  the `go` directive automatically raises the idiom target.
- **Auto-apply:** `go tool golangci-lint run --fix` writes the suggested fixes; the
  pre-commit hook (§6) runs it on staged files.
- **Belt and braces:** Go 1.26's revamped **`go fix ./...`** is the official modernizer —
  CI runs it as a **drift check** (CI-CD §2) so un-modernized code can't merge.

See the committed [`.golangci.yml`](../../.golangci.yml) — it also enforces **size**
(`funlen` · `lll` · revive `file-length-limit`) and **naming** (revive `var-naming`),
per CODE_STYLE_GO.

---

## 5. JS/TS — workspace standards

- **Base = Google style.** TS follows the **Google TypeScript Style Guide** via **`gts`**
  (ESLint + Prettier + Google `tsconfig`; `gts lint` / `gts fix`). **Google has no Vue style
  guide** → Vue uses the **official Vue Style Guide** with the same gts/Prettier TS rules
  inside SFCs (CODE_STYLE_TS · CODE_STYLE_VUE).
- **ESM-only** (`"type": "module"`, `moduleResolution: "nodenext"`), `strict: true`,
  **no `any`**.
- **ESLint flat config** (`eslint.config.ts`): typescript-eslint v8 + eslint-plugin-vue
  (templates) + Prettier-as-formatter. (Biome/oxlint can't lint Vue SFC templates yet —
  ESLint stays the runner.)
- **Size & naming, lint-enforced:** `max-lines` (≤300 TS / ≤400 `.vue`),
  `max-lines-per-function` (≤80), Prettier `printWidth` 80; **filenames** via
  `unicorn/filename-case` (kebab-case TS, PascalCase Vue components).
- **Vue:** type-check with **vue-tsc**; **Vite 8** (Rolldown) — confirm the Vue/Vite plugins are
  Rolldown-compatible before the 7→8 bump.
- **Dead-code gate:** `knip` in CI (unused files/exports/deps).

## 5a. Markdown (docs are linted like code)

- **Format** with **Prettier** ([`.prettierrc.json`](../../.prettierrc.json), `proseWrap:
preserve`); **lint** with **markdownlint-cli2**
  ([`.markdownlint-cli2.jsonc`](../../.markdownlint-cli2.jsonc)). Diagrams are **mermaid
  only** (DOC_STYLE), and doc length is budgeted (≤~250 lines, split past ~300).

---

## 6. Pre-commit (one hook, all three languages)

**lefthook** runs the right formatter/linter on staged files only, so Go · TS · Vue stay
uniform without a per-language ritual:

```yaml
# lefthook.yml
pre-commit:
  parallel: true
  commands:
    go: { glob: '*.go', run: 'golangci-lint fmt {staged_files} && golangci-lint run' }
    ts: { glob: '*.{ts,vue}', run: 'pnpm exec eslint --fix {staged_files}' }
    fmt: { glob: '*.{ts,vue,json,md}', run: 'pnpm exec prettier --write {staged_files}' }
    md: { glob: '*.md', run: 'pnpm exec markdownlint-cli2 {staged_files}' }
    leak: { run: 'scripts/check-leaks.sh' }
    lengths: { run: 'scripts/check-lengths.sh' }
```

### Leak guard (pre-commit)

`scripts/check-leaks.sh` scans staged diffs for secrets and tenant-specific terms — blocking
commit on match. Patterns: PEM private-key block headers, service-account JSON,
API token prefixes (GCP/AWS/GitHub), and a project-local denylist
(`.githooks/denylist.local`, gitignored) for bank-specific terms. Excludes known
placeholders (`your-*`, `000000`, etc.). Proven in s1ctl — critical for mise because the
ingest path handles bank internal documents.

### File-length guard (pre-commit)

`scripts/check-lengths.sh` enforces Go ≤500 lines, TS ≤300, Vue ≤400, docs ≤250 prose
(CODE_STYLE_GO/TS/VUE, DOC_STYLE). A grandfather list exempts files that existed before the
rule — new files must comply.

The same checks run in CI (CI-CD §2) — the hook is a fast local mirror, never the only gate.

---

## 7. Scaffold Defaults

- **JSON:** use stable `encoding/json`; do not opt into `encoding/json/v2` experiments unless the
  configured Go release makes them stable.
- **Version pinning:** the **Podman dev-container** is the reference polyglot pin. Developers may
  use local version managers, but the container is the reproducible baseline.
- **Dependency bumps:** use **Dependabot** by default. Mend-hosted Renovate remains a later
  operational option if the project needs richer grouping; dependency updates are reviewed changes
  (TOOLCHAIN §1, CI-CD §4).
