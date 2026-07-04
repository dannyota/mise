<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Deploy

mise runs **single-tenant** — one instance in each bank's own GCP project. This guide covers
the reference deployment on GKE and the local dev stack.

## Prerequisites

- A GCP project with billing enabled
- `gcloud` CLI authenticated with sufficient IAM
- Podman (local dev) or a GKE cluster (production)
- Go 1.26+, pnpm 11+, Node 24 LTS

## Local development (recommended start)

The local stack uses Podman Compose with `VERTEX=fake` — no GCP calls, no cost.

```bash
# Clone and enter the repo
git clone https://github.com/dannyota/mise.git
cd mise

# Start the local stack (AlloyDB Omni + Temporal + workers)
podman compose up -d

# Run migrations
go run ./cmd/migrate up

# Start the serving process
go run ./cmd/serving

# In another terminal — start the reasoning endpoint
cd apps/reasoning && pnpm install && pnpm dev
```

The local stack provisions:

- **AlloyDB Omni** container (1 vCPU, schema-per-corpus + `graph` schema, ScaNN, RLS)
- **Temporal** server + Go workers (ingest + detect workflows)
- All Vertex seams faked (`VERTEX=fake`) — embed, judge, ground, rank return deterministic results

See [LOCAL-DEV](/engineering/LOCAL-DEV.md) for the full local-dev guide.

## Production deployment (GKE)

The reference deployment is a single GKE cluster with no HA (scale-down to zero when idle).

### 1. Provision infrastructure

```bash
# Create the GKE cluster (Autopilot recommended)
gcloud container clusters create-auto mise \
  --region=<region> --project=<project-id>

# Enable required APIs
gcloud services enable \
  alloydb.googleapis.com \
  aiplatform.googleapis.com \
  documentai.googleapis.com \
  discoveryengine.googleapis.com \
  secretmanager.googleapis.com
```

### 2. Deploy AlloyDB Omni

AlloyDB Omni runs as a StatefulSet in the cluster (not managed AlloyDB — keeps everything
inside the bank's perimeter with no Google-managed control plane access to data).

```bash
kubectl apply -f deploy/alloydb/
```

### 3. Deploy mise services

```bash
# Build containers (Podman/Buildah)
podman build -t mise-serving -f deploy/containers/serving.Containerfile .
podman build -t mise-worker -f deploy/containers/worker.Containerfile .
podman build -t mise-reasoning -f deploy/containers/reasoning.Containerfile .

# Push to Artifact Registry and deploy
kubectl apply -f deploy/k8s/
```

### 4. Configure secrets

```bash
# AlloyDB credentials for the serving/worker deployments
gcloud secrets create mise-alloydb-password --data-file=./db-pass.txt

# The Vertex AI connection uses Workload Identity (ADC) — no secret needed
```

### 5. Verify

```bash
# Health check
curl https://<mise-endpoint>/healthz

# Run the eval harness against the local stack
go run ./cmd/eval -golden eval/golden-vn.json -corpora vn-reg -min-recall 0.7
```

## Environment variables

| Variable                                                                  | Default                                      | Purpose                                                       |
| ------------------------------------------------------------------------- | -------------------------------------------- | ------------------------------------------------------------- |
| `VERTEX`                                                                  | `fake`                                       | `fake` = deterministic fakes (CI/local); `real` = live Vertex |
| `ALLOYDB_HOST` / `ALLOYDB_USER` / `ALLOYDB_PASSWORD` / `ALLOYDB_DATABASE` | (unset)                                      | AlloyDB connection; serving stays healthz-only without a host |
| `MISE_DB_ROLE`                                                            | `mise_public`                                | RLS role the serving process reads as                         |
| `TEMPORAL_HOST` / `TEMPORAL_NAMESPACE` / `TEMPORAL_TASK_QUEUE`            | `localhost:7233` / `default` / `mise-ingest` | Temporal wiring for the worker                                |
| `GCP_PROJECT` / `GCP_REGION`                                              | (required real)                              | Vertex project/region when `VERTEX=real`                      |
| `DOCAI_PROCESSOR_ID` / `DOCAI_LOCATION`                                   | (required real)                              | Document AI parser seam                                       |
| `JUDGE_MODEL` / `JUDGE_ESCALATION_MODEL`                                  | Gemini defaults                              | Judge model + escalation tier                                 |
| `GCS_BUCKET` / `BLOB_DIR`                                                 | local dir                                    | Raw-file blob store (GCS in prod, dir locally)                |
| `LIBRARY_ROOT`                                                            | (unset)                                      | Internal-corpora drop folder; unset = internal ingest off     |
| `SERVING_PORT`                                                            | `8080`                                       | Serving HTTP port                                             |

## What's next

- [Ingest your first corpus](first-corpus.md) — start with public law, no internal docs needed.
- [DEPLOYMENT](/engineering/DEPLOYMENT.md) — full runtime topology reference.
- [DELIVERY-MODEL](/engineering/DELIVERY-MODEL.md) — tenancy, upstream/adopter split.
