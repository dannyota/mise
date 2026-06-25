# Mise — Delivery Risk Register

Execution / delivery risks for building mise — what could stall, rework, or derail the phased
build, with mitigations and the decision/configuration path each risk traces to. This is the
**delivery** view; it is
**not** the security threat model ([THREAT-MODEL](../engineering/THREAT-MODEL.md), STRIDE +
trust boundaries) nor the design-decision log ([DECISIONS](./DECISIONS.md)) — it **points** to
both and tracks the schedule/quality exposure they create.

See also:

- [PLAN](./PLAN.md) (the build plan)
- [ROADMAP](./ROADMAP.md) (milestones + critical path)
- [DECISIONS](./DECISIONS.md) (locked and open design basis)
- [THREAT-MODEL](../engineering/THREAT-MODEL.md) (security residual risks)
- [DELIVERY-MODEL](../engineering/DELIVERY-MODEL.md) (who owns the confidentiality call)
- [COST](./COST.md)
- [plan/](./plan/README.md) (per-milestone risks)

---

## 1. Posture & method

- **Build shape (frames every risk).** AI coding agents implement; a **solo, part-time human**
  directs and reviews. Phases run **strictly sequential** (PLAN). So the scarce resource is
  **human review/decision time**, not code throughput — the register weights that accordingly.
- **Scoring.** **Likelihood** and **Impact** each `H` / `M` / `L`. **Impact** = effect on the
  build (rework, schedule slip, or a blocked phase), not production severity. **Exposure** =
  the headline of likelihood × impact.
- **Ownership.** Each high risk names its owning decision, implementation gate, or adopter input
  so the mitigation is concrete. The register is reviewed **at each phase boundary** (§4).

---

## 2. The register

Ordered by exposure. "Bites" names the phase(s) where the risk first lands.

| ID  | Risk                                                                                                                                                                                                        | Bites          | Likelihood | Impact | Traces to    | Mitigation                                                                                                                                     |
| --- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------- | ---------- | ------ | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| R1  | **Managed-AI policy drift.** DECISIONS 10/17 allow confidential internal control text on the bank's **own** Vertex; a stricter adopter policy could later require self-hosted models for confidential tiers | P1·P3·P4·P5·P6 | M          | M      | —            | every Vertex call sits behind a **config seam** so a stricter policy is an endpoint swap to the self-hosted variant (AI-GOV §7), not a rebuild |
| R2  | **RLS correctness** — one policy bug collapses tier isolation across the cross-corpus join surface                                                                                                          | P1·P2·P3·P4·P5 | M          | H      | —            | **mandatory** cross-tier-deny RLS tests on real AlloyDB (TESTING §2); edge inherits the stricter tier; Heavy review on every tier task         |
| R3  | **Crawler MFA / Conditional-Access** — headless AD login blocked; needs an approved scoped service account                                                                                                  | P1             | H          | M      | DECISIONS 13 | build the connector against a **non-bank SharePoint test site** until adopter access is available; IP-allowlisted read-only account            |
| R4  | **Solo-reviewer bottleneck** — Heavy-review tasks (RLS, grounding, AI-gov, migrations) concentrate on one part-time human                                                                                   | all            | H          | M      | —            | keep every task to **one reviewable PR**; split when too big; sequence Heavy tasks so they don't bunch (ROADMAP)                               |
| R5  | **Mapping-quality cold start** — the cross-corpus `satisfies` metric is unmeasured at design time                                                                                                           | P3·P4          | M          | M      | DECISIONS 18 | seed a curated VN/Malay golden set; treat the first number as a **baseline, not a gate**; floors are provisional (TESTING §5)                  |
| R6  | **Doc-control / metadata parsing brittleness** — ragged internal templates break signer/owner + edge extraction                                                                                             | P1·P2          | H          | M      | —            | drive parsing off the per-source `metadata_config`; treat an absent header as "no edge / fall back", never a guess (DATA-MODEL §2)             |
| R7  | **Judge precision / threshold tuning** — wrong thresholds flood the review queue or suppress real mappings                                                                                                  | P3             | M          | M      | DECISIONS 11 | thresholds **logged + versioned**, tuned against the golden set before exit; confidence + grounding both gate candidacy                        |
| R8  | **Contract drift** — serving / reasoning / web diverge if types are hand-mirrored                                                                                                                           | P2·P4·P5       | M          | M      | —            | **one generated contract** (`packages/contract` from Go — API-CONTRACT §5); the contract test fails the build on un-regenerated drift          |
| R9  | **Confused-deputy / tier propagation** — the reasoning endpoint reads under the service's tier, not the user's                                                                                              | P4             | M          | H      | —            | call MCP **as the user** (identity forwarded); prove with an RLS-propagation test; the boundary is the DB, not the agent (API-CONTRACT §1)     |
| R10 | **Engine generalization leak** — the single-jurisdiction assumption is wired deeper than the descriptor                                                                                                     | P0             | M          | M      | —            | isolate to the **corpus registry + config seam** early; a registry test asserts all 5 descriptors resolve + share one embed space              |
| R11 | **Prompt injection via evidence** — ingested text carries adversarial instructions                                                                                                                          | P4·P6          | M          | M      | —            | agent is **read-only, permission-gated, tier-scoped** (AI-GOV §5/§8) — blast radius capped to the caller's tier; assert in an injection test   |
| R12 | **One-embedding-space drift** — a new corpus/caption embedded in a different model/dim fragments the space                                                                                                  | P6             | L          | H      | DECISIONS 1  | registry **fails closed** on a mismatched `embed{}` at registration; native image vectors barred here (DEC 1 LOCKED)                           |
| R13 | **Cost expectation drift** — an adopter's runtime shape differs from the reference 2-vCPU/no-HA profile                                                                                                     | all (run)      | M          | L      | —            | COST owns reference numbers; adopters can resize node pools, DB vCPU, and worker concurrency in their own project (DECISIONS 12/15/16)         |
| R14 | **Availability (no HA by design)** — single replicas + scale-to-zero cold starts                                                                                                                            | all (run)      | M          | L      | DECISIONS 9  | accepted trade for an internal tool; cold-start SLO budget + pre-warm (OBSERVABILITY §4); HA priced if it becomes critical (COST §6)           |
| R15 | **Webhook SSRF policy gap** — webhook endpoint validation left vague until the UI/notification surface ships                                                                                                | P5             | M          | M      | DECISIONS 19 | lock allowlist/URL-validation behavior before webhook delivery is enabled; negative tests for private-address and DNS-rebinding cases          |
| R16 | **Embedding call-site rework** — in-DB embedding couples the app to DB extension/IAM behavior, while in-app embedding keeps Mode B simpler                                                                  | P0·P1          | M          | L      | DECISIONS 14 | keep the embedder interface as the boundary; close DEC 14 after checking IAM, latency, portability, backfill use, and offline testability      |

---

## 3. Open Review And Integration View

Several risks share a root implementation-review decision, integration input, or calibration task.

| Dependency                                                     | Risk | Needed by                     | If delayed                                                                                   |
| -------------------------------------------------------------- | ---- | ----------------------------- | -------------------------------------------------------------------------------------------- |
| **DECISIONS 14** (embedding call site)                         | R16  | **M0/M1** embed path          | use the interface seam and fake while the implementation review closes                       |
| **DECISIONS 13** (adopter source-access config)                | R3   | **M1** real-source validation | internal-corpus ingest runs on fixtures/test site until adopter access exists                |
| **DECISIONS 18** (eval golden-set bootstrap)                   | R5   | **M3** eval baseline          | mapping precision/recall has no first baseline                                               |
| **DECISIONS 11** (judge + serve threshold / escalation tuning) | R7   | **M3/M4** tuning              | queue noise or suppressed mappings; model id is a config seam, so this is tuning, not rework |
| **DECISIONS 19** (webhook egress policy)                       | R15  | **M5** webhook delivery       | webhooks stay disabled or internal-only until SSRF controls are specified                    |
| **DECISIONS 1** (embedding @1536-d)                            | R12  | **M6** registry enforcement   | a non-conforming corpus fragments the vector space — enforced fail-closed                    |

> **R1 status.** DECISIONS 10/17 close the reference path: confidential internal control text may
> use the bank's **own** GCP Vertex. The residual is adopter policy drift, handled by the same
> config seam and self-host swap (AI-GOV §7).

---

## 4. Watchlist & review cadence

- **At every phase boundary:** re-score this register; a phase does not exit with an unmitigated
  `H`/`H` risk in its scope (per-phase **Phase risks** sections).
- **R1 / R2 / R9 are standing** — model-egress controls and tier isolation are re-checked on
  **every** phase that adds a Vertex call or a new read surface, not once.
- **R4 (reviewer bottleneck) is structural** — the antidote is task hygiene (one PR = one review)
  enforced in the work-breakdowns, not a one-time fix.
- **New risks** are added here as phases surface them; the per-phase files point back to this
  register so nothing is tracked in two places.
