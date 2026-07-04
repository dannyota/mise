<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Operations

Day-to-day operation of a mise deployment — monitoring, backup, upgrades, and scaling.

## Monitoring

mise exports metrics and traces via OpenTelemetry:

- **Metrics** — request latency (p50/p95/p99), ingest throughput, detector run counts,
  embedding cache hit rate, Temporal workflow health.
- **Traces** — distributed traces across serving → reasoning → Vertex calls.
- **Logs** — structured JSON (Go `slog`), correlated with trace IDs.

Ship to your existing observability stack (Cloud Monitoring, Grafana, Datadog — any OTLP
receiver). See [OBSERVABILITY](/engineering/OBSERVABILITY.md) for SLOs and alerting guidance.

## Backup & recovery

- **AlloyDB Omni** — standard PostgreSQL backup (`pg_dump` / WAL archiving). The reference
  deployment uses a CronJob for nightly `pg_dump` to GCS.
- **Temporal** — persistence is in the shared AlloyDB instance; backed up with it.
- **No external state** — mise is stateless beyond the database. Lose a pod, not data.

Recovery: restore the database, redeploy containers. Ingest is idempotent — re-running it
after a restore is safe (dedup keys prevent duplicates).

## Upgrades

mise follows semver. Upgrade path:

1. Pull the new tag from source.
2. Run `go run ./cmd/migrate up` — migrations are forward-only (goose).
3. Rebuild and redeploy containers.
4. Verify with the eval harness.

No breaking schema changes without a major version bump. Temporal workflows in flight will
drain on the old worker before new workers pick up new work.

## Scaling

The reference deployment is single-replica, no HA (cost-appropriate for the typical audit
team size). If you need more:

- **Serving** — horizontal scale (stateless); add replicas behind a load balancer.
- **Workers** — scale Temporal worker count for ingest throughput.
- **AlloyDB** — increase vCPU (2 → 4 → 8) for heavier concurrent search loads.
- **Reasoning** — scale the TS service replicas; each is independent.

KEDA + cluster autoscaler handle scale-down to zero when idle (ingest workers, reasoning).

## Ingest scheduling

Ingest runs on-demand or on a cron schedule (Temporal cron workflows):

```bash
# Trigger a one-off ingest
curl -X POST http://localhost:8080/api/v1/ingest -d '{"corpus": "vn-reg"}'

# Cron ingest is configured in deploy/temporal/schedules.yaml
```

Public law corpora: weekly (regulations change slowly).
Internal corpora: daily or on SharePoint webhook (detect doc updates).

## Security operations

- **Secrets** are in GCP Secret Manager — never in env vars or config files.
- **Container scanning** — Trivy runs in CI; images are signed with cosign.
- **Dependency scanning** — govulncheck (Go), osv-scanner (npm) in CI.
- **Access** — Workload Identity for GCP services; no long-lived keys.

See [THREAT-MODEL](/engineering/THREAT-MODEL.md) for the full security posture.
