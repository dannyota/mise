# Mise — UI Design

The mise Web UI — a **Vue 3.5 SPA** (Vite). This
file owns the screens, stack, and design principles; detailed visual design (layouts,
components, interactions, wireframes) is iterated here.

See also: [ARCHITECTURE.md](./ARCHITECTURE.md) (system design) ·
[AI-GOVERNANCE.md](./AI-GOVERNANCE.md) (the agent the Q&A chat talks to) ·
[DATA-GOVERNANCE.md](./DATA-GOVERNANCE.md) (access tiers the screens respect) ·
[DATA-MODEL.md](./DATA-MODEL.md).

---

## 1. 🛠️ Stack

**Vite + Vue 3.5 + TypeScript** · **Vue Flow + elkjs** (Graph Explorer — layered
auto-layout of the SOP→Policy→Group→law DAG) · **TanStack Vue Table** (Review Workbench
/ Findings) · **reka-ui + CVA + Tailwind 4** (shadcn-vue components) · **mermaid**
(chains/diagrams) · lucide icons · vue-router · @vueuse/core. **SSE** for the Q&A chat.
SPA behind **OIDC** — no SSR.

The browser **never calls a model.** The Audit Q&A chat streams from the backend
**reasoning endpoint** (Claude Agent SDK) over SSE; dashboards/graph/review hit the Go
serving API over REST. (See [AI-GOVERNANCE.md](./AI-GOVERNANCE.md) §5.)

---

## 2. 🖥️ Screens

| Screen               | Purpose                                                                                                                                                                                                                                                                                         |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Dashboards**       | coverage %, open conflicts, staleness alerts, review-queue depth, per-corpus ingest status                                                                                                                                                                                                      |
| **Graph Explorer**   | visualize SOP→Policy→Group→law chain (Vue Flow + elkjs); click node → edges + verbatim evidence; filter by corpus/tier                                                                                                                                                                          |
| **Review Workbench** | candidate edges with **confidence + grounding score** + both verbatim texts side-by-side + AI rationale → **promote / reject / relink** (correct target or edge_type); a **relink re-triggers detection** and recomputes dependent findings; filter/sort by confidence (human-in-the-loop core) |
| **Findings**         | gaps / conflicts (with contradiction citations) / staleness — each opens a **resolution**                                                                                                                                                                                                       |
| **Resolution**       | the finding-resolution board: each finding → **disposition** (Map · Document · Accept · Escalate) → owner (role+dept) → status (Open · In progress · In review · Closed); progress · overdue · group into action plans; **closure is evidence-verified** by re-detection                        |
| **Audit Q&A chat**   | ask a question → grounded answer + citation chain + "evidence checked" badge; streamed from the reasoning endpoint                                                                                                                                                                              |
| **Change Timeline**  | regulatory "what changed": amendments over a period → the downstream policies/SOPs each makes stale — an **impact view** over the validity/amendment model (DATA-MODEL §3/§6)                                                                                                                   |
| **Notifications**    | in-app inbox of finding events (new conflict · staleness · overdue resolution); per-user read/unread, deep-links to the finding. Also pushed by **email + webhook** (§5)                                                                                                                        |
| **Reports**          | generate a **Coverage report** (per-regulation control chain, gaps flagged) and export the **Findings register** (Excel) — §5                                                                                                                                                                   |
| **Corpus Admin**     | register a corpus/scope, configure source, trigger ingest, view Temporal workflow status                                                                                                                                                                                                        |

---

## 3. ⚖️ Finding resolution (closing the loop)

mise **detects** findings (gap · conflict · staleness) and **tracks how each is
resolved** — it does **not** remediate. A human chooses a **disposition**; mise records
it, routes it to the durable owner, and **verifies closure by re-detection**. mise
never asserts the bank is compliant — only that a _finding_ no longer fires.

| Disposition  | When                                                 | Action                                      | Closes when                                                   |
| ------------ | ---------------------------------------------------- | ------------------------------------------- | ------------------------------------------------------------- |
| **Map**      | coverage exists, just unlinked                       | promote / relink an edge (Review Workbench) | a grounded, human-attested edge exists → finding stops firing |
| **Document** | the paperwork is incomplete                          | draft/update the policy/SOP → re-ingest     | re-detection confirms it's gone                               |
| **Accept**   | a human risk-accepts the gap                         | record rationale + owner                    | logged (no re-detection)                                      |
| **Escalate** | a real control gap; the fix happens **outside** mise | record owner + due + link out               | a document change flows back and clears it                    |

- **Evidence-verified closure** (the differentiator): an item closes because **detection
  re-runs and the finding is gone** — not on a "done" click. `Accept` closes with a
  logged rationale; `Escalate` stays visible until a document change flows back.
- **Owner = durable `(department, role)`** — accountability survives staff turnover;
  overdue routes to the role, not a departed person.
- **AI assist (advisory):** the reasoning agent may _suggest_ a disposition or draft text
  ("POL-001 could add a clause covering RMiT 10.x — here's the law"), but a **human
  decides and acts** ([AI-GOVERNANCE.md](./AI-GOVERNANCE.md) §5).

---

## 4. 🧭 Design principles

- **Evidence-first:** every claim links verbatim source + citation; nothing is asserted
  without a traceable span.
- **Confidence visible:** candidate edges always show judge confidence + grounding score;
  the review queue is sortable/filterable by it.
- **Human-in-the-loop is the spine:** the Review Workbench is the feedback core — every
  promote/reject/relink is audited, re-triggers detection for affected nodes, and the
  human-attested edges become the eval golden set
  ([DATA-GOVERNANCE.md](./DATA-GOVERNANCE.md) §5).
- **Access-tiered:** screens only surface what the caller's tier permits — enforced
  server-side (RLS), the UI filter is convenience, never the boundary.
- **Cross-lingual evidence:** corpora span vi/en/ms; the crown-jewel mapping links across
  languages. Evidence is shown **verbatim, side-by-side** (e.g. VN law beside the EN/MS
  policy that satisfies it) with a per-pane **"translate" toggle** that calls machine
  translation **on demand** — the verbatim source stays authoritative; translation is a
  reading aid, never the cited text. **Confidential-tier text is only translated subject to
  the AI gate** (§5 / AI-GOVERNANCE §7).

---

## 5. 🔔 Notifications, reporting & change tracking

- **Notifications — three channels.** A finding event (new conflict · staleness from an
  amendment · overdue resolution) fans out to: an **in-app** inbox (read/unread, deep-link),
  **email** (immediate for high-severity, else digest), and a **webhook** (a generic
  HTTP+HMAC callback the bank wires into whatever internal system it uses). **Webhook
  payloads carry a reference + tier, not confidential content** — the receiver fetches the
  finding under auth (DATA-GOVERNANCE §8).
- **Reporting/export.** First outputs: a **Coverage report** (pick a regulation → the
  SOP→Policy→Group→law chain with gaps flagged) and the **Findings register** as **Excel**
  (kind · severity · owner role+dept · status · due). Generated server-side from evidence —
  cited, never asserted (an auditor evidence-pack PDF is deferred).
- **Change tracking.** The Change Timeline screen reads the existing amendment/validity
  model — no new schema, an impact query (DATA-MODEL §3/§6) — and is the primary source of
  staleness notifications.
- **On-demand translation.** The "translate" toggle (§4) calls the **Google Cloud
  Translation API** per request; results are a display aid, cached by source-hash, and
  **gated for confidential tiers** (AI-GOVERNANCE §7).

---

## 6. 🗺️ To design (next)

Wireframes, component inventory (shadcn-vue), the Graph Explorer interaction model
(node/edge detail panel, chain highlighting), the Review Workbench layout (side-by-side
verbatim + rationale + actions), and the chat transcript with inline citations.
