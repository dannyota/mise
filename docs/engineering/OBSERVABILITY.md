<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Observability

How mise is **traced, measured, logged, and alerted** across its four services, and the
**SLOs** that back the read-path "milliseconds" claim. This doc owns _telemetry + SLOs_;
the data/AI **audit** trail (a compliance record, not telemetry) stays in
[DATA-GOVERNANCE.md](../design/DATA-GOVERNANCE.md) §6 +
[AI-GOVERNANCE.md](../design/AI-GOVERNANCE.md) §9.

See also:

- [ARCHITECTURE.md](../design/ARCHITECTURE.md) (services)
- [CODE_STYLE_GO.md](./CODE_STYLE_GO.md) (slog)
- [CODE_STYLE_TS.md](./CODE_STYLE_TS.md) (pino)
- [CI-CD.md](./CI-CD.md)
- [DATA-GOVERNANCE.md](../design/DATA-GOVERNANCE.md) §6

---

## 1. Audit vs telemetry (don't conflate)

|          | **Audit** (compliance record)             | **Telemetry** (operational)        |
| -------- | ----------------------------------------- | ---------------------------------- |
| Question | "who saw/changed what, with which model"  | "is it up, fast, erroring"         |
| Store    | DB tables, immutable, retained per policy | metrics/traces/logs backend, TTL'd |
| Owner    | DATA-GOVERNANCE §6 · AI-GOVERNANCE §9     | **this doc**                       |

They share a correlation id (`run_id` on write, `request_id` on read) so an operational
trace can point at the audit row — but they are separate systems.

---

## 2. Tracing — one request, three hops

The hard requirement: a Q&A request crosses **Web → reasoning (SSE) → serving (MCP) →
AlloyDB**, and a trace must stitch all of it.

- **OpenTelemetry** everywhere; **W3C `traceparent`** propagated across every hop,
  **including the MCP call** from reasoning → serving (inject into MCP request metadata).
- Go: `otel` SDK + `otelsql`/`pgx` instrumentation (DB spans), `otelhttp`.
- TS: `@opentelemetry/*` auto-instrumentation for Hono + the Agent SDK tool calls (one
  span per MCP tool call + per model turn).
- The browser starts the trace (web vitals + the SSE request span) so latency is
  measured end-to-end, not just server-side.
- Export via **OTLP** to one collector (Tempo/Jaeger or Cloud Trace).

---

## 3. Metrics

RED for services, USE for the box; plus mise-specific signals.

| Area                | Metrics                                                                                                                               |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| **Serving (read)**  | request rate · error rate · **latency p50/p95/p99** (the SLO, §4) · ScaNN vs FTS time · RRF merge time · cache hit-rate (query-embed) |
| **Reasoning**       | turns/request · tool calls/request · model latency (Haiku/Sonnet) · abstain rate · refusal/iteration-cap hits · tokens (cost signal)  |
| **Worker (ingest)** | Temporal workflow success/fail · activity retries · docs/chunks ingested · judge candidates proposed · grounding pass/fail rate       |
| **AlloyDB**         | CPU/mem (vs the default 2-vCPU sizing) · connections · index-build time · cold-start time (scale-down)                                |
| **KEDA/autoscaler** | replica count · scale-to-zero events · cold-start latency                                                                             |

Prometheus scrape (or Managed Service for Prometheus); the worker exposes Temporal's own
metrics.

---

## 4. SLOs (and the budget they protect)

The architecture asserts **evidence-only reads in milliseconds** — make it a measured
target, not a claim:

| Service                       | SLO                                                                  | Notes                                                       |
| ----------------------------- | -------------------------------------------------------------------- | ----------------------------------------------------------- |
| **Serving read (cache hit)**  | p95 **< 150 ms**                                                     | pure AlloyDB retrieve + RLS + joins                         |
| **Serving read (cache miss)** | p95 **< 500 ms**                                                     | adds one Vertex query-embed hop                             |
| **Reasoning Q&A first token** | p95 **< 3 s**                                                        | retrieve + first model token (SSE)                          |
| **Ingest freshness**          | new internal-source doc (web crawl / SMB / Graph) visible **< 24 h** | batch, scales from zero                                     |
| **Availability**              | best-effort (**no HA**, single replicas — DECISIONS 9)               | scale-down may add cold-start; pre-warm before busy windows |

Cold starts (KEDA scale-to-zero, AlloyDB idle-stop) are an explicit budget line — they
trade availability for cost (DECISIONS 9/15) and must be visible on the dashboard.

---

## 5. Logging

- **Structured JSON**, no PII / no raw query text (queries can carry sensitive terms —
  store a hash, matching DATA-GOVERNANCE §6). Go = `log/slog`; TS = `pino`.
- Every line carries `service · request_id/run_id · trace_id · caller_tier`.
- Logs correlate to traces by `trace_id`; **don't** duplicate the audit log here.

---

## 6. Alerting

- **SLO burn-rate** alerts on the read latency + Q&A first-token SLOs (§4).
- **Ingest health:** Temporal workflow failures, grounding-pass-rate drop (a model/quality
  regression signal), internal-source crawl stalled > freshness SLO.
- **Cost/quota:** Vertex/Doc AI quota nearing limit; reasoning token spend anomaly.
- Route to the on-call channel (the **operational** path — distinct from the user-facing
  _finding_ notifications, which are a shipped product feature: UI-DESIGN §5,
  DATA-GOVERNANCE §8).

---

## 7. Observability Defaults

- Use **Cloud Operations** (Trace/Monitoring/Logging) in the reference GCP deployment. A
  self-hosted Grafana + Tempo + Prometheus + Loki stack is an adopter portability variant.
- Trace sampling is deploy config: always keep errors and slow traces; sample successful read-path
  traces at a low rate to control cost.
