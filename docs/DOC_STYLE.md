# Mise — Doc Style

The contract every mise doc follows: **clear, single-purpose, non-overlapping**, at the bar of
[AI-GOVERNANCE.md](./design/AI-GOVERNANCE.md). Keep it short; keep it true to the design.

---

## 🗂️ Where a doc goes

| Folder         | Owns                                                                                        |
| -------------- | ------------------------------------------------------------------------------------------- |
| `design/`      | the product — architecture, data model, AI/data governance, UI, API contract                |
| `engineering/` | how we build & run — toolchain, CI/CD, tests, observability, deploy, licenses, threat model |
| `project/`     | direction — PLAN, DECISIONS, COST                                                           |

`DOC_STYLE.md` stays at the `docs/` root. **One responsibility per doc** — the ownership table in
the root `CLAUDE.md` is authoritative. If content belongs elsewhere, **point to it**
(`[NAME](./NAME.md) §N`), never duplicate: one doc owns a topic, siblings reference it.

## ✍️ Voice

- **Short, dense, technical, professional.** State what's true; cut filler, hedging, and history
  ("we tried…", "it turns out…"). The reader's time is the budget.
- **Lead with the answer** — bottom line first, supporting detail after.
- **Customer-neutral.** No real bank, customer, host, or one-off internal constraint. The worked
  setup is "the **reference deployment**"; use placeholders (`<tenant>`, `vn` / `my`) for specifics.

## 🎨 Format

- **Emoji signpost the main sections** — exactly one per **H2**, none on **H1 titles** (sole
  exception: the root `README.md` title carries the project's one signature emoji, 🍱), none on
  H3+, none mid-sentence. Where a section is cross-referenced by number, the emoji follows the
  number: `## 3. 🤖 AI components` (the `§N` reference is unaffected).
- **Reserved palette** — reuse for recurring meaning; pick one fitting glyph otherwise:

  🧭 overview / principle · 🤖 AI / model / agent · 🗄️ data / schema / store ·
  🔗 graph / relations · ⚖️ governance / decision · 🔒 access / security gate ·
  ⚠️ risk / threat · ✅ allowed / done · 💰 cost · 🚀 build / deploy / plan ·
  🧪 testing · 📊 observability.

- **Tables and lists over prose** for any set: **tables** for keyed or comparative sets
  (components · flags · mappings), **numbered lists** for ordered steps/sequences, **bullets**
  for everything else.
- **Code fences** for schemas and commands; **mermaid** for every flow or structure (below).
- A number lives **once**, in its owning doc (cost → COST, decisions → DECISIONS); elsewhere,
  reference it. Inline glyphs: `·` separates, `→` is flow, `§N` is a section ref.

## 📐 Structure of a doc

- **H1:** `# Mise — <Name>`, no emoji. One H1 per file; sentence-case headings.
- **Purpose line:** 1–2 sentences — what the doc owns (and what it doesn't).
- **See also:** one line linking siblings, each with a parenthetical scope.
- `---` between sections; numbered `## N. 🔵 Title` where sections are referenced by `§N`.
- **Keep it short:** ≤ ~250 lines of **prose** (mermaid + schema/code fences don't count). When
  prose sprawls past one concern, split and leave pointers (ARCHITECTURE → DEPLOYMENT / LOCAL-DEV);
  length is a smell that a doc owns more than one thing.

## 📊 Mermaid

- **All diagrams are mermaid — never ASCII art or hand-drawn boxes.** Flows, component views, and
  state machines go in ` ```mermaid ` fences. Directory trees and schemas stay plain code fences —
  they are listings, not diagrams.
- **Colour convention:**
  - violet `fill:#6d28d9,stroke:#a78bfa,color:#fff` = an **AI / model / control-gate** node;
  - green `fill:#14532d,stroke:#86efac,color:#fff` = **allowed**;
  - red `fill:#7f1d1d,stroke:#fca5a5,color:#fff` = **denied**.
- Keep node labels short; push detail to the prose/table beside the diagram. Pair every diagram
  with a one-line "what to read here". GitHub stacks two `mermaid` blocks vertically.

## 🔧 Tooling

- **Format with Prettier** (`.prettierrc.json`, `proseWrap: preserve` — line wrapping is yours)
  and **lint with markdownlint** (`.markdownlint-cli2.jsonc`); both run in pre-commit + CI
  (TOOLCHAIN §6, CI-CD §2) — docs are linted like code.
- **Blank line before every table, list, and fenced block, and after every heading** — renders
  cleanly on GitHub and any static site.
- Schemas and directory trees use intentionally language-less fences (configured in markdownlint,
  not an oversight).

## 🔗 Cross-references & naming

- Link siblings `[NAME.md](./NAME.md)`; deep refs `(NAME §N)`. Renaming a doc touches **every**
  `[…](./OLD.md)` link and prose `OLD §N` across the set — **grep first**, and keep each **See
  also** line and the `README.md` doc index in sync.
- Files are `UPPER-KEBAB.md`. Avoid names that collide with the product domain — mise's domain _is_
  governance, so the data/security doc is `DATA-GOVERNANCE.md`, not `GOVERNANCE.md`.
- **Design-stage:** the docs are the deliverable. When code lands, design changes ship **with** the
  code in the same change — docs and code never drift.
