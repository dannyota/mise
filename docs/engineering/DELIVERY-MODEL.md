<!--
SPDX-License-Identifier: AGPL-3.0-only
Copyright (C) 2026 Danny Ota
-->

# Mise — Delivery & Operating Model

mise is **open source (AGPL-3.0) on GitHub**, deployed as a **single-tenant instance per
enterprise**: the adopter (a bank, or an integrator it engages) **builds from the public
source and self-hosts** in **its own GCP project**, run by **its own ops team**. There is **no
managed service and no proprietary vendor in the software path** — the source is auditable, and
the adopter owns the build, the deployment, and all data. This doc owns the _delivery model_;
runtime mechanics are [DEPLOYMENT.md](./DEPLOYMENT.md), cost per instance is
[COST.md](../project/COST.md), the threat view is [THREAT-MODEL.md](./THREAT-MODEL.md), and
licensing is [LICENSE](../../LICENSE) / [README](../../README.md).

> **Open core, startup door left open.** mise ships under **AGPL-3.0** and the author is the
> **sole copyright holder** (no external contributions merged —
> [CONTRIBUTING](../../CONTRIBUTING.md)), so a future **commercial / managed distribution**
> (signed releases, support, or a hosted offering) is possible under the existing
> **dual-license** — without re-architecting anything below.

See also:

- [DEPLOYMENT.md](./DEPLOYMENT.md) (GKE runtime)
- [CI-CD.md](./CI-CD.md) (build · sign · SBOM)
- [LICENSES.md](./LICENSES.md) (dependency licenses)
- [ARCHITECTURE.md](../design/ARCHITECTURE.md)
- [DECISIONS.md](../project/DECISIONS.md) 9

---

## 1. Tenancy — single-tenant, one instance per enterprise

mise is **not** a multi-tenant SaaS. Each bank runs its **own dedicated instance**, deployed
**into the bank's own GCP project**, inside the bank's security perimeter. Consequences:

- **No shared control plane** and **no cross-customer blast radius** — one bank's data,
  models, and outages never touch another's; there is nothing central to operate or attack.
- **Access tiers are intra-org** (group vs local within the one bank), enforced by RLS — never
  a cross-tenant boundary (DATA-GOVERNANCE §2).
- **The bank owns the cloud account**, so it owns the **Vertex contract, IAM, region, data
  terms, and keys (CMEK)** — decisions 10/17 lock the reference "yes" to bank-owned Vertex for
  internal control text; region is deploy config.
- The trade-off is **per-enterprise build + deploy** effort (no fleet-wide push) and a
  **build/dependency supply-chain** trust boundary (THREAT-MODEL §3, B7).

## 2. Who does what — upstream (the project) vs adopter (the operator)

| Concern              | Upstream (project / author)                                                                                      | Adopter (the bank)                                                          |
| -------------------- | ---------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| **Source**           | maintains the public **AGPL** repo; reproducible **build + sign + SBOM** recipes (CI-CD §3); security advisories | reviews/audits the source; tracks releases                                  |
| **Build**            | **source only — no published binaries** (reproducible build recipes, CI-CD §3)                                   | **builds from source and signs with its own keys** for admission control    |
| **Runtime**          | —                                                                                                                | runs GKE, AlloyDB, workers; scaling, backup/DR (DEPLOYMENT)                 |
| **Cloud + identity** | —                                                                                                                | owns the GCP project, IAM/ADC, **Vertex enablement**, networking, VPC-SC    |
| **Data**             | **no access, none held**                                                                                         | owns all corpora, graph, audit, backups; sole data controller               |
| **Secrets**          | —                                                                                                                | provisions + rotates AD/SMB/DB creds in Secret Manager (DATA-GOVERNANCE §7) |
| **Config**           | ships defaults + corpus descriptors                                                                              | sets connectors, tiers, thresholds                                          |

Upstream distributes **source only** — the adopter produces **every running binary**. There is
no **standing upstream access** to a running instance or its data: the relationship is
**source + dependencies (a supply chain)**, not a managed service.

## 3. Per-enterprise onboarding (build + install)

1. **Obtain + verify the source** (public repo) and review the AGPL terms (LICENSE).
2. **Build + scan + sign** images via the project's CI recipes (CI-CD §3/§4); push to the
   bank's registry under the **bank's** signature.
3. **Provision** the GCP project: GKE, AlloyDB Omni, GCS, Secret Manager; enable the bank's
   **Vertex** APIs + region (DECISIONS 17).
4. **Deploy** (signed-image admission); run the migration Job (`cmd/migrate`).
5. **Configure** source connectors (SharePoint web crawl / SMB — DATA-MODEL §1), access tiers,
   and the metadata config; load secrets.
6. **Seed + ingest** the corpora; confirm the eval baseline (TESTING §5, DECISIONS 18).
7. **Bring-up checks:** RLS tier isolation, OIDC, signed-image admission, backups.

## 4. Releases & upgrades

- **Source-tagged releases.** Upstream tags **source** versions (no prebuilt binaries); the
  adopter **builds, generates the SBOM/provenance, signs, and deploys** on its own schedule
  (CI-CD §3) — **no auto-update, no upstream push.**
- **Migrations** run as a pre-deploy Job; **single replicas** make an upgrade a brief restart
  (no HA — DECISIONS 9), scheduled in a maintenance window with pre-warm (DEPLOYMENT §2).
- **Security advisories** flow via the repo; dependency CVEs are caught by the adopter's CI
  scanners (CI-CD §4). **Rollback** = redeploy the previous signed image + restore if needed
  (DEPLOYMENT §3); the irreplaceable graph/attestations are backed up first.

## 5. Data ownership & boundaries

- **The bank is the sole data controller.** All corpora, the graph, attestations, and the
  audit trail live in the bank's project; upstream neither stores nor accesses them.
- **The only data egress** is to the **bank's own GCP Vertex** (allowed by DECISIONS 10/17;
  AI-GOVERNANCE §7) and to **internal webhook receivers** (reference + tier only —
  DATA-GOVERNANCE §8). Both stay within the bank's control.
- **Region** is the bank's GCP region choice (DECISIONS 17); retention/erasure is
  DATA-GOVERNANCE §9.

## 6. Licensing & the startup option

- **AGPL-3.0 today.** Upstream publishes **source only** (no prebuilt binaries) — anyone may
  self-host and modify under AGPL (network use ⇒ source availability); the dependency tree is
  permissive-only and gate-enforced (LICENSES.md).
- **Sole copyright holder.** Because no external contributions are merged
  ([CONTRIBUTING](../../CONTRIBUTING.md)), the author may later offer a **commercial license,
  signed distribution, or managed service** without re-architecting — the self-host model above
  simply becomes one of two delivery options. Commercial inquiries: README / CONTRIBUTING.
