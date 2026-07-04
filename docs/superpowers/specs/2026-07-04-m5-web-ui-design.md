# M5 Web UI — Design Spec

## 1. Goal

Build the Vue 3.5 SPA (`apps/web`) that brings every mise screen into the browser: Dashboards,
Graph Explorer, Review Workbench, Findings, Resolution board, Audit Q&A chat, Change Timeline,
Notifications, Reports export, and Corpus Admin — consuming the generated `@mise/contract` types,
behind OIDC, with the browser **never calling a model**.

**Exit:** all UI-DESIGN §2 screens live, tier-gated, generated-contract-only, SSE chat streaming,
no model client in the browser.

## 2. Architecture

### 2.1. Stack

- **Framework:** Vue 3.5 + Vite 8 (Rolldown) + TypeScript 6.0 strict ESM
- **Components:** reka-ui (shadcn-vue primitives) + CVA (class-variance-authority) + Tailwind 4
- **Graph:** Vue Flow (`@vue-flow/core`) + elkjs (via `@vue-flow/layout`)
- **Tables:** TanStack Vue Table (`@tanstack/vue-table`)
- **Icons:** lucide-vue-next
- **Auth:** oidc-client-ts (authorization code flow + PKCE)
- **Utilities:** @vueuse/core, vue-router 4
- **Testing:** Vitest 4 + @vue/test-utils + happy-dom
- **Lint:** eslint-plugin-vue + gts Prettier (printWidth 80) + vue-tsc --noEmit
- **Contract:** `@mise/contract` workspace dependency (generated types, never hand-mirrored)

No SSR. SPA behind OIDC redirect. The browser calls the Go serving API (REST) and the reasoning
endpoint (SSE) — never a model provider.

### 2.2. Pattern — composable-first

Each screen is a route-level view (< 400 lines SFC) that composes child components + composables.

- **Views** (`views/`) — route targets, layout + orchestration only
- **Components** (`components/`) — reusable UI units (< 400 lines)
- **Composables** (`composables/`) — reactive logic extracted from views (`useX()`)
- **API** (`api/`) — typed client + SSE wrapper
- **Auth** (`auth/`) — OIDC + route guard + capability flags

State is local + composable by default. Pinia only for auth state (cross-component, survives
navigation) and notification inbox (polling + badge count).

## 3. File structure

```
apps/web/src/
├── main.ts                        # createApp + router + auth init
├── App.vue                        # shell: sidebar + header + RouterView
├── env.d.ts                       # VITE_* env type declarations
├── auth/
│   ├── oidc.ts                    # oidc-client-ts UserManager config
│   ├── guard.ts                   # router beforeEach → redirect unauthenticated
│   ├── store.ts                   # Pinia auth store (user, tier, capabilities)
│   └── useAuth.ts                 # composable wrapping the store
├── api/
│   ├── client.ts                  # typed fetch wrapper (bearer, RFC 9457, cursor, idempotency)
│   └── sse.ts                     # POST-based SSE reader for /chat (ReadableStream, not EventSource)
├── composables/
│   ├── useCapabilities.ts         # server-resolved tier + translate_allowed from bootstrap
│   ├── useChat.ts                 # SSE stream → reactive tokens/citations/chain/abstain/done
│   ├── useTranslate.ts            # POST /translate with capability gate
│   ├── usePagination.ts           # cursor-based pagination helper
│   └── useNotifications.ts        # notification inbox (Pinia store + polling composable)
├── components/
│   ├── EvidencePane.vue           # shared: verbatim side-by-side + citation + translate toggle
│   ├── TranslateToggle.vue        # gated toggle (disabled + refusal when !translate_allowed)
│   ├── ErrorBanner.vue            # RFC 9457 problem+json rendering
│   ├── LoadingState.vue           # skeleton/spinner states
│   ├── EmptyState.vue             # empty result placeholder
│   ├── SidebarNav.vue             # navigation sidebar
│   ├── HeaderBar.vue              # top header with user menu
│   ├── graph/
│   │   ├── GraphCanvas.vue        # Vue Flow + elkjs layout wrapper
│   │   ├── GraphNode.vue          # custom node: corpus badge + tier + label
│   │   ├── GraphEdge.vue          # custom edge: confidence badge
│   │   └── ChainDrawer.vue        # SOP→Policy→Group→law hop detail drawer
│   ├── review/
│   │   ├── ReviewTable.vue        # TanStack table: candidates + confidence + grounding
│   │   ├── ReviewActions.vue      # promote/reject/relink + confirmation dialog
│   │   └── RelinkDialog.vue       # relink target picker + recompute progress
│   ├── findings/
│   │   ├── FindingsTable.vue      # TanStack table: findings by kind/severity/status
│   │   ├── ResolutionForm.vue     # disposition (Map/Document/Accept/Escalate) + owner + status
│   │   └── ResolutionTimeline.vue # resolution status change history
│   ├── chat/
│   │   ├── ChatInput.vue          # question textarea + submit + stop button
│   │   ├── ChatStream.vue         # streaming tokens with inline citation markers
│   │   ├── CitationCard.vue       # citation reference card with source_url link
│   │   └── EvidenceBadge.vue      # "evidence checked" badge
│   ├── timeline/
│   │   └── TimelineList.vue       # filterable amendment/event list with deep-links
│   ├── notifications/
│   │   └── NotificationList.vue   # inbox list: read/unread + deep-links
│   ├── reports/
│   │   └── ReportExport.vue       # export buttons (coverage report, findings .xlsx)
│   ├── admin/
│   │   ├── CorpusCard.vue         # per-corpus ingest health + Temporal status
│   │   └── IngestRetry.vue        # safe retry controls (authorized operators only)
│   └── dashboard/
│       ├── SummaryTile.vue        # single metric tile (coverage %, conflicts, etc.)
│       └── IngestStatus.vue       # per-corpus ingest status indicator
├── views/
│   ├── DashboardView.vue          # summary tiles + ingest status
│   ├── GraphView.vue              # Graph Explorer: canvas + chain drawer + evidence pane
│   ├── ReviewView.vue             # Review Workbench: table + evidence pane + actions
│   ├── FindingsView.vue           # Findings register
│   ├── ResolutionView.vue         # Resolution board for a single finding
│   ├── ChatView.vue               # Audit Q&A: input + stream + citations
│   ├── TimelineView.vue           # Change Timeline
│   ├── NotificationsView.vue      # Notification inbox
│   ├── ReportsView.vue            # Reports export page
│   ├── AdminView.vue              # Corpus Admin
│   ├── LoginView.vue              # OIDC redirect landing
│   └── CallbackView.vue           # OIDC callback handler
└── router/
    └── index.ts                   # routes + auth guard middleware
```

## 4. API client

### 4.1. Typed REST client (`api/client.ts`)

```typescript
type ApiClient = {
  get<T>(path: string, params?: Record<string, string>): Promise<T>;
  post<T>(path: string, body?: unknown): Promise<T>;
  delete(path: string): Promise<void>;
  getCursor<T>(path: string, params?: Record<string, string>): AsyncGenerator<T[], void>;
};
```

- Base URL: `VITE_API_URL` (default `/api/v1`)
- Bearer token injected from auth store on every request
- Response errors → `ApiError` (RFC 9457 `type`, `title`, `status`, `detail`)
- Write endpoints accept optional `idempotencyKey` parameter
- `getCursor` yields pages via cursor-based pagination

### 4.2. SSE client (`api/sse.ts`)

```typescript
type SseReader = {
  start(question: string, opts?: { corpora?: string[]; locale?: string }): void;
  stop(): void;
};
```

Uses `fetch` with POST body (not `EventSource` — POST body required). Reads from `ReadableStream`
via `TextDecoderStream`. Parses `event:` + `data:` lines. Calls typed callbacks per event type.
Abort via `AbortController` on `stop()` or component unmount.

The reasoning endpoint URL: `VITE_REASONING_URL` (default `/reasoning`).

## 5. Auth

### 5.1. OIDC flow

`oidc-client-ts` UserManager configured with:

- `authority`: `VITE_OIDC_ISSUER`
- `client_id`: `VITE_OIDC_CLIENT_ID`
- `redirect_uri`: `${origin}/callback`
- `response_type`: `code`
- `scope`: `openid profile email`
- PKCE enabled (default in oidc-client-ts)

**Dev mode:** when `VITE_OIDC_ISSUER` is unset, auth is bypassed — the client sends
`X-Fake-Sub` / `X-Fake-Tier` headers (matching the reasoning endpoint's dev mode). The route
guard still runs but always passes.

### 5.2. Capability bootstrap

After OIDC callback, the app calls `GET /api/v1/bootstrap` which returns
`{ tier, capabilities: { translate_allowed, admin_allowed } }`.

The auth store exposes:

- `user` — OIDC user profile
- `tier` — server-resolved tier (never client-asserted)
- `capabilities` — feature flags resolved by tier
- `token` — bearer access token for API calls
- `isAuthenticated` — boolean

### 5.3. Route guard

`router.beforeEach` checks `isAuthenticated`. Unauthenticated → redirect to `/login`.
`/login` triggers `userManager.signinRedirect()`. `/callback` handles `signinCallback()`.
Admin routes (`/admin`) additionally check `capabilities.admin_allowed`.

## 6. Key composables

### 6.1. useChat

```typescript
function useChat(): {
  question: Ref<string>;
  tokens: Ref<string>; // accumulated answer text
  citations: Ref<SseCitationEvent['data'][]>;
  chain: Ref<SseChainEvent['data'][]>;
  evidenceChecked: Ref<SseEvidenceCheckedEvent['data'][]>;
  abstain: Ref<string | null>; // abstain reason, null if answered
  done: Ref<SseDoneEvent['data'] | null>;
  error: Ref<SseErrorEvent['data'] | null>;
  isStreaming: Ref<boolean>;
  send(): Promise<void>;
  stop(): void;
};
```

- `send()` opens SSE stream, resets all refs, begins parsing
- `stop()` aborts the stream (AbortController)
- Auto-stops on component unmount (`onUnmounted`)
- Never calls a model — only talks to the reasoning endpoint

### 6.2. useCapabilities

```typescript
function useCapabilities(): {
  tier: ComputedRef<string>;
  translateAllowed: ComputedRef<boolean>;
  adminAllowed: ComputedRef<boolean>;
  loaded: Ref<boolean>;
  refresh(): Promise<void>;
};
```

Wraps the auth store's capability data. The `loaded` ref prevents rendering tier-gated
content before the bootstrap call completes.

### 6.3. useTranslate

```typescript
function useTranslate(): {
  translate(text: string, sourceLang: string, targetLang: string): Promise<string>;
  isTranslating: Ref<boolean>;
  canTranslate: ComputedRef<boolean>; // false when !translate_allowed
};
```

- If `!canTranslate`, the UI shows the refusal state — no API call is made
- Calls `POST /api/v1/translate` with the evidence text
- Results cached by source hash (in a reactive Map)

## 7. Screen design notes

### 7.1. Graph Explorer (GraphView + graph/ components)

- Vue Flow renders nodes + edges from the graph API response
- elkjs computes layered layout (top-to-bottom: law → Group → Policy → SOP)
- Click node → side panel with edges, confidence scores, and EvidencePane
- Violet `#6d28d9` fill on AI-touched/model nodes
- Denied nodes are absent from API response (RLS) — no inference from gaps
- Split into GraphCanvas (< 200 lines) + GraphNode + GraphEdge + ChainDrawer

### 7.2. Review Workbench (ReviewView + review/ components)

- TanStack table: sortable by confidence, filterable by corpus/tier/kind/status
- Each row expands to show both verbatim texts side-by-side (EvidencePane)
- Actions: promote (confirm dialog), reject (confirm + reason), relink (RelinkDialog)
- All actions send `Idempotency-Key`; double-submit is safe
- Relink shows recompute progress indicator

### 7.3. Audit Q&A Chat (ChatView + chat/ components)

- ChatInput: textarea + submit + stop button (disabled when !isStreaming)
- ChatStream: renders accumulated tokens with inline `[N]` citation markers
- Citation markers link to CitationCard (corpus, document, citation_path, source_url)
- EvidenceBadge shows "evidence checked" when evidence_checked events arrive
- Abstain renders a distinct "insufficient evidence" state
- ChainDrawer shows the control chain (SOP→Policy→Group→law)
- Stop button calls `useChat().stop()` → AbortController

### 7.4. Findings + Resolution

- FindingsTable: TanStack table filterable by kind (gap/conflict/staleness), severity, status
- Click finding → ResolutionView with ResolutionForm + ResolutionTimeline
- Disposition: Map, Document, Accept, Escalate (radio group)
- Owner: department + role fields (durable, not person-specific)
- Evidence-verified closure: status shows "pending re-detection" until detection re-runs

### 7.5. Dashboard

- Summary tiles: coverage %, open conflicts count, staleness alerts, review queue depth
- Per-corpus ingest status (IngestStatus component)
- Lower-tier session sees only permitted corpora in all tiles

## 8. Testing

### 8.1. Component tests (Vitest + Vue Test Utils + happy-dom)

- Mock `api/client.ts` — inject fake responses
- Assert rendering: correct data in DOM, loading/empty/error states
- SSE test: mock fetch returning a ReadableStream of SSE events, assert all 7 types render
- Tier-hide tests: render with lower-tier mock data, assert denied rows never in DOM

### 8.2. No-model-client gate

ESLint `no-restricted-imports` in `apps/web/.eslintrc.cjs`:

```javascript
'no-restricted-imports': ['error', {
  patterns: [
    '@anthropic-ai/*', '@google-cloud/aiplatform',
    '@google-cloud/vertexai', 'openai',
    'claude-agent-sdk', '@anthropic-ai/sdk',
  ],
}],
```

A test file (`model-import.test.ts`) additionally greps `apps/web/src/` for these patterns as
a defense-in-depth check.

### 8.3. Contract drift gate

`vue-tsc --noEmit` in the build script. If `@mise/contract` types change without regeneration,
the web build fails.

### 8.4. Webhook egress

Webhook config UI refuses private-address endpoints. Delivery is disabled/internal-only until
DECISIONS 19 closes. Negative tests for `10.0.0.0/8`, `192.168.0.0/16`, `127.0.0.0/8`,
`[::1]`, and DNS-rebinding.

## 9. Dependencies

**Production:**

- vue@^3.5, vue-router@^4, @vueuse/core
- oidc-client-ts
- @vue-flow/core, @vue-flow/layout (elkjs)
- @tanstack/vue-table
- reka-ui, tailwindcss@^4, class-variance-authority, lucide-vue-next
- pinia
- @mise/contract (workspace)

**Dev:**

- vite@^8, @vitejs/plugin-vue
- vue-tsc@^3, typescript@^6
- vitest@^4, @vue/test-utils, happy-dom
- eslint, eslint-plugin-vue, prettier
- @types/node@^24
- @tailwindcss/vite

## 10. Env variables

| Variable              | Default            | Purpose                     |
| --------------------- | ------------------ | --------------------------- |
| `VITE_API_URL`        | `/api/v1`          | serving REST base URL       |
| `VITE_REASONING_URL`  | `/reasoning`       | reasoning endpoint base URL |
| `VITE_OIDC_ISSUER`    | (unset = dev mode) | OIDC issuer URL             |
| `VITE_OIDC_CLIENT_ID` | `mise-web`         | OIDC client ID              |

## 11. Constraints

- Every `.vue` file ≤ 400 lines (lint-enforced)
- Line length = 80 (Prettier)
- No model SDK imports in `apps/web/` (lint + grep test)
- Types from `@mise/contract` only — never hand-mirror
- Tier comes from the server (bootstrap call) — never client-asserted
- Translate toggle disabled + refusal when !translate_allowed — no API call
- Webhooks disabled/internal-only until DECISIONS 19
- All review/resolution writes use Idempotency-Key
- SFC order: `<script setup>` → `<template>` → `<style>`
- PascalCase component filenames; `useX.ts` composables
- `ref`/`computed` over `reactive` for primitives
- Pinia only for auth + notifications (everything else is local)
