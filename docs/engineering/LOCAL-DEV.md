<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Local Development

How to run the **whole stack on a laptop**, deliberately **separated from the GKE
deployment** ([DEPLOYMENT.md](./DEPLOYMENT.md)): same code, local infra, **Podman** for
containers, and a `VERTEX=real|fake` toggle for the managed AI. This doc owns _local dev_.

See also:

- [DEPLOYMENT.md](./DEPLOYMENT.md) (the production counterpart)
- [ARCHITECTURE.md](../design/ARCHITECTURE.md) §9 (stack)
- [TOOLCHAIN.md](./TOOLCHAIN.md) (versions)
- [TESTING.md](./TESTING.md)
- [CI-CD.md](./CI-CD.md)

---

## 1. Principle — local ≠ deploy, on purpose

The full stack runs locally **before** any deployment. Stateful infra runs in **Podman**
containers on the dev machine; the managed Vertex AI APIs are either called from localhost
(ADC) or stubbed. **No Kubernetes locally** — local uses `podman compose`; GKE/KEDA/Helm are
deployment-only ([DEPLOYMENT.md](./DEPLOYMENT.md)). The seam that makes both work is the
**Vertex Go interface** (§4).

---

## 2. The local stack (Podman)

| Component                                                   | Local                                                                                               |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| AlloyDB (Omni)                                              | Podman container `google/alloydbomni` (ScaNN + pgvector + `ai.hybrid_search` offline; free for dev) |
| Temporal                                                    | `temporal server start-dev`, or `podman compose`                                                    |
| Go workers / serving / MCP                                  | run directly (`go run ./cmd/...`) or via `podman compose`                                           |
| Web UI (Vue)                                                | `pnpm dev` (Vite)                                                                                   |
| Reasoning endpoint (TS)                                     | `pnpm dev` (tsx + Hono); Claude via Vertex (ADC), or a stubbed model offline                        |
| Internal sources                                            | **folder + manifest** loader (the `ingest.Source` fake); real connectors only to test them (§3)     |
| Vertex APIs (embed · Gemini · Ranking · Grounding · Doc AI) | call real Vertex from localhost (ADC), or fake/fallback offline                                     |

Containers run **rootless** under Podman — the same `Containerfile`s CI builds with
Buildah (CI-CD §3), so "works on my laptop" matches the image that ships.

---

## 3. Internal source connectors, locally

Default to the **folder + manifest** loader — drop sample docs in a directory with a
side-car manifest (validity · owner · tier); no network, no creds. Exercise the real
connectors (DATA-MODEL §1) only when working on them:

- **SharePoint/Graph** — needs an Azure AD app; point at a test site.
- **SharePoint web crawl** — sign in with a test **AD account**; crawl a test library.
- **SMB share** — mount a local SMB server (a Podman `samba` container works) and walk it.

All three emit the same records as the folder loader, so feature work downstream of Discover
never needs them.

---

## 4. Two modes & the Vertex seam

**Mode A — real Vertex from laptop (default, high fidelity):** local infra + ADC auth to
real Vertex APIs. Needs a dev GCP project; tiny cost at dev volume; gives real
embeddings/judgments. Use for feature work and quality evaluation.

**Mode B — fully offline (CI):** each Vertex call swapped for a fake/fallback behind its Go
interface. Fast and free, but **fake vectors ⇒ cross-corpus mapping is not semantically
meaningful** — validates plumbing only (the unit-test path, TESTING §2).

**Design rule:** every Vertex touchpoint sits behind a Go interface (the banhmi/laksa engine
already does this for the embedder, `pkg/rag/embed`). Add the same seam for parser, judge,
and grounding. Local config wires the fake/fallback; deploy wires Vertex; a `VERTEX=real|fake`
toggle selects per environment.

> Note: the embedder call site is in-app Go, not AlloyDB's in-DB `google_ml.embedding()` —
> DECISIONS 14 (locked).

---

## 5. Run law ingest locally

The M1a public-law pipeline end to end, `VERTEX=fake`: vn-reg/my-reg sources are **public**
government sites that need no credentials, so — unlike the internal connectors (§3) — there is no
"fake source" to swap in here; the fully offline/no-network path is
`pkg/pipeline/e2e_test.go`'s in-memory fixture source, exercised by `go test -tags integration`,
not this CLI walkthrough.

1. `podman compose up -d` — starts AlloyDB Omni + Temporal (UI on `localhost:8088`), runs the
   `migrate` job, then `serving` and `worker` (both default to `VERTEX=fake`, compose.yaml).
2. Start a run:

   ```bash
   temporal workflow start --task-queue mise-ingest --type IngestCorpusWorkflow \
     --input '{"Corpus":"vn-reg"}'
   ```

   The worker's crawlers hit the live public sources (vbpl/vanban/congbao/sbv_hanoi) for real —
   `VERTEX=fake` only swaps the embed/parse seams — so expect real documents with fake
   (token-bag) embeddings.

3. Watch it in the Temporal UI, or read `ingest.run` / `ingest.doc_ledger` directly against
   AlloyDB.
4. Check what landed:

   ```bash
   go run ./cmd/eval -golden deploy/eval/golden-vn.json -corpora vn-reg
   ```

DEPLOYMENT.md §6 has the same commands for the GKE reference deployment, plus the `VERTEX=real`
prerequisites.
