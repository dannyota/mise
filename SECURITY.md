# Security

mise is design-stage (no application code yet); this states the **security posture** the
design commits to and **how to report a vulnerability**. The **design-level threat model**
(assets, trust boundaries, STRIDE, residual risks) is
[THREAT-MODEL.md](docs/engineering/THREAT-MODEL.md); penetration testing and per-service
STRIDE refinement are deferred to the build phase.

## 🔒 Reporting a vulnerability

Email **<danh.software@outlook.com>** with details and reproduction steps. Please do **not**
open public issues for security reports (Issues/Discussions are not monitored —
[CONTRIBUTING.md](CONTRIBUTING.md)). You'll get an acknowledgement; coordinated disclosure is
appreciated. mise is provided under **AGPL-3.0 without warranty** (see [LICENSE](LICENSE)).

## 🧭 Posture (where each control is specified)

| Concern                      | Control                                                                                                                                        | Owning doc                                                                                              |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| **Tenancy / deployment**     | **single-tenant** — one instance per enterprise, in the bank's own GCP project, bank-operated; **open source, self-hosted** from public source | [DELIVERY-MODEL](docs/engineering/DELIVERY-MODEL.md) · [DECISIONS](docs/project/DECISIONS.md) 9         |
| **Access / confidentiality** | per-corpus access tiers, **RLS at the database** (not just UI); cross-corpus joins can't leak rows the caller can't see                        | [DATA-GOVERNANCE](docs/design/DATA-GOVERNANCE.md) §2                                                    |
| **AI / agent boundary**      | serve agent is **read-only, permission-gated, tier-scoped**; evidence is data, not instructions (prompt-injection safe); no fs/bash            | [AI-GOVERNANCE](docs/design/AI-GOVERNANCE.md) §5/§8                                                     |
| **Evidence-only invariant**  | models propose, grounding gates, humans attest — no model asserts compliance                                                                   | [AI-GOVERNANCE](docs/design/AI-GOVERNANCE.md) §1                                                        |
| **Supply chain**             | SAST · SCA · secrets · container scan; SBOM + cosign signing; signed-image admission                                                           | [CI-CD](docs/engineering/CI-CD.md) §3/§4                                                                |
| **Secrets**                  | Secret Manager, WIF (keyless) CI, git-ignored creds + gitleaks                                                                                 | [CI-CD](docs/engineering/CI-CD.md) §5 · [DATA-GOVERNANCE](docs/design/DATA-GOVERNANCE.md) §7            |
| **Audit trail**              | every read, attestation, and model turn is logged and reconstructable                                                                          | [DATA-GOVERNANCE](docs/design/DATA-GOVERNANCE.md) §6 · [AI-GOVERNANCE](docs/design/AI-GOVERNANCE.md) §9 |
| **Data retention / PDPD**    | per-layer retention; PII minimized to role+dept; erasure via the `org_role` resolver                                                           | [DATA-GOVERNANCE](docs/design/DATA-GOVERNANCE.md) §9                                                    |
| **Residency (open)**         | Decree 53/2022 + Decree 13/2023 — region-pinning and cross-border processing                                                                   | [DECISIONS](docs/project/DECISIONS.md) 17                                                               |
| **Dependency licensing**     | permissive-only shipped deps, enforced by a CI license gate                                                                                    | [LICENSES](docs/engineering/LICENSES.md)                                                                |
| **Threat model**             | assets · trust boundaries · STRIDE · residual risks                                                                                            | [THREAT-MODEL](docs/engineering/THREAT-MODEL.md)                                                        |
