# Mise — Vue Code Style (frontend)

For the **Vue 3.5 SPA** (Vite 8). **Google publishes no Vue
style guide**, so the base is the **official Vue Style Guide** (Priority A/B) +
**Composition API** with `<script setup lang="ts">`; the TypeScript inside SFCs follows the
Google TS rules via **gts** ([CODE_STYLE_TS.md](./CODE_STYLE_TS.md)). Tooling: **TS strict**,
**ESLint (eslint-plugin-vue)** + **Prettier**, type-check with **vue-tsc**. Pinned versions
(Vite 8/Rolldown, Vue 3.5, Tailwind 4, …): [TOOLCHAIN.md](./TOOLCHAIN.md) §1/§5.

---

## 🧱 Components

- One component per file, **PascalCase** filename (`ReviewWorkbench.vue`) — enforced by
  `eslint-plugin-vue` (`vue/component-name-in-template-casing`); composables are `useX.ts`.
- SFC block order: `<script setup>` → `<template>` → `<style>`.
- Typed **`defineProps<T>()`** and **`defineEmits<T>()`** — no runtime prop objects.
- Keep components small; extract shared logic into **composables** (`useX()` in `composables/`).
- `ref` / `computed` over `reactive` for primitives; every `v-for` has a stable `:key`.

## 🛠️ Size (lint-enforced)

- **File length** ≤ 400 lines per `.vue` (`max-lines`); split a large screen into
  child components + composables. **Line length** = 80 (Prettier). `eslint --fix` /
  `prettier --write` auto-format; same checks in pre-commit + CI.

## 🗄️ State & data

- Local state + composables first; reach for **Pinia** only for genuinely global state.
- A **typed API client** wraps `fetch` to the Go serving API; the chat uses **SSE** to the
  reasoning endpoint. The browser **never calls a model** (AI-GOVERNANCE §5).
- Access tier comes from the **server** (RLS); the UI filter is convenience, never the boundary.

## 🖥️ Styling & UI

- **Tailwind 4** utilities + **CVA** (class-variance-authority) for component variants;
  **reka-ui** primitives (shadcn-vue) — don't hand-roll accessibility. Icons:
  **lucide-vue-next**. No ad-hoc global CSS.

## 📚 Feature libraries

- **Graph Explorer:** **Vue Flow** + **elkjs** layout; custom nodes are components.
- **Tables** (Review Workbench / Findings / Resolution): **TanStack Vue Table**.
- **Chains / diagrams:** **mermaid**.

## 🧪 Testing

- **Vitest** + **Vue Test Utils** for components.
