# Digital Will remediation plan

Proposed only; no actions or checks were run. See [review](REVIEW.md).

## Phased roadmap

| Phase | Focus | Entry criteria | Exit criteria |
| --- | --- | --- | --- |
| 1 | Baseline and claim scoping | Owner provides isolated environment and synthetic data | Config errors are reproducible, documentation boundaries are narrowed, and runtime-vs-host guarantees are written down |
| 2 | Legal/identity/authority boundary definition | Phase 1 complete | Product/legal positioning, role matrix, and destination-verification requirements are approved or the repo is explicitly limited to non-legal personal use |
| 3 | Trigger and audit invariants | Phase 2 complete and action policy approved | Restart, retry, idempotency and audit-checkpoint semantics are specified and tested |
| 4 | Meaningful automation | Phase 3 complete | Migration-backed negative tests and dependency-integrity gates fail correctly on broken behavior |
| 5 | Deployment and recovery observation | Phase 4 complete and owner approves recovery objectives | Synthetic staging, backup restore and rollback evidence are observed |
| 6 | Operator UX and independent acceptance | Phase 5 complete | Operators can safely understand deadlines/overrides and an independent reviewer can reproduce the critical checks |

### DW-T1 — Baseline and threat model (DW-1, DW-2)
Prerequisite: owner supplies isolated environment and synthetic data. Observed paths: `SECURITY.md`, `cmd/dw/main.go`, `internal/config/`.
1. Inspect complete configuration loading and reproduce errors without real wills.
2. Propagate configuration errors and test missing, invalid and inaccessible configuration paths.
3. Specify host-compromise, at-rest, unlocked-process and operator boundaries; correct overbroad documentation.
Deliverable: reproducible baseline and threat model. Verification: independent reviewer confirms a malformed configuration yields a clear error and checks claims against the defined attacker.
- [ ] Configuration failures are surfaced without unintended actions.
- [ ] Runtime host-compromise limitations are explicit.

### DW-T2 — Bound legal, identity and destination claims (DW-6, DW-7)
Prerequisite: DW-T1. Observed paths: `README.md`, `SECURITY.md`, `cmd/dw/main.go`, `internal/api/server.go`; proposed paths: operator/legal docs and destination-onboarding policy.
1. Inventory every place the product claims `will`, `owner`, auto-trigger, executor-like authority or external action control, then classify each as software record, technical authentication, or asserted legal effect.
2. Decide and document whether the product is only an encrypted dead-man-switch/instruction vault or is intended for any jurisdiction-specific estate workflow; if the latter, name the jurisdictions, required human approvals and non-software formalities instead of implying software sufficiency.
3. Define owner/executor/beneficiary authority separately from passphrase or token possession, including who may unlock the daemon, issue/revoke tokens, register action destinations and approve post-trigger execution.
4. Define destination verification for each supported action class: email recipient control, webhook/domain ownership, and script-host authorization, plus re-verification and recovery rules when domains/accounts change hands.
Deliverable: approved claim-boundary memo and authority matrix. Verification: independent reviewer checks the docs/specs and confirms no sampled interface or operator runbook equates passphrase/token possession with legal entitlement; sample onboarding artifacts show required destination/domain checks for each action type.
- [ ] Software record versus legally effective will language is explicit.
- [ ] Authentication is separated from owner/executor authority.
- [ ] Destination/domain verification requirements exist for email, webhook and script actions.

### DW-T3 — Protect trigger and audit invariants (DW-3, DW-4)
Prerequisite: DW-T2 and owner-approved action policies. Observed paths: `internal/scheduler/scheduler.go`, `internal/action/service.go`, `internal/audit/audit.go`, `internal/api/server.go`.
1. Trace check-in, deadline, claim, retry, cancellation and completion across daemon restarts.
2. Define at-least-once delivery and idempotency boundaries, secret handling, webhook destination controls and script authorization.
3. Establish independently protected audit checkpoints or narrow the integrity claim; add negative tests for rewrite and truncation.
Deliverable: documented, enforced state and audit boundaries. Verification: simulate check-in races, duplicate delivery, clock changes, restart and tampered audit data with synthetic actions.
- [ ] Duplicate and missed-trigger handling is explicit and tested.
- [ ] Audit guarantees match the actual checkpoint threat model.

### DW-T4 — Automate meaningful checks (DW-2, DW-3, DW-5, DW-7)
Prerequisite: DW-T3. Observed paths: `go.mod`, `internal/*/*_test.go`, `internal/storage/migrations/`; proposed CI workflow.
1. Generate and review module checksums under an approved dependency policy and capture clean-build results.
2. Add migration-backed integration tests for authentication, action execution, failure recovery, encrypted payload tampering and any implemented destination-registration policy.
3. Add a reproducible required CI gate without weakening existing tests.
Deliverable: reproducible security and lifecycle suite. Verification: deliberately break a deadline, authorization condition or destination-verification precondition in isolation and confirm a negative test fails.
- [ ] Tests use real migrations and independent expected behavior.
- [ ] Module integrity and test failure propagation are recorded.

### DW-T5 — Observe deployment and recovery (preventive)
Prerequisite: DW-T4; owner approves monitoring, backup and recovery objectives. Observed paths: `internal/storage/backup.go`, `docs/OPERATIONS.md`, `internal/health/health.go`.
1. Define encrypted backup/key separation, alerting and safe behavior during host or network outages.
2. Stage the daemon with synthetic wills and non-delivering action sinks; exercise restart, missed check-in and cancellation.
3. Restore an actual encrypted backup in isolation and demonstrate rollback to a verified artifact.
Deliverable: observed recovery and operating runbook. Verification: independently inspect restored wills and scheduler state; a successful backup exit alone is insufficient.
- [ ] Restore and rollback meet approved objectives.
- [ ] Monitoring and action delivery failure paths are observed.

### DW-T6 — Operator UX and independent acceptance (preventive)
Prerequisite: DW-T5. Observed paths: `cmd/dw/main.go`, `docs/OPERATIONS.md`.
1. Test terminal prompts, deadline presentation and cancellation warnings with representative operators.
2. Verify errors do not expose secrets, irreversible actions require the approved confirmation policy, and authority handoff instructions do not imply legal certification.
3. Independently rerun boundary, tamper, destination-verification and restore checks against the candidate revision.
Deliverable: acceptance record and residual risks. Verification: observe a synthetic full lifecycle including a denied or cancelled action.
- [ ] Operators can understand deadlines and recover from failures.
- [ ] Owner and independent reviewer accept the candidate and limitations.

## Task dependency map

| Task | Depends on | Unlocks |
| --- | --- | --- |
| DW-T1 | Owner provides isolated environment | DW-T2 |
| DW-T2 | DW-T1 | DW-T3 |
| DW-T3 | DW-T2 | DW-T4 |
| DW-T4 | DW-T3 | DW-T5 |
| DW-T5 | DW-T4 | DW-T6 |
| DW-T6 | DW-T5 | Production acceptance decision |

## Production acceptance checklist

- [ ] README/ops copy no longer implies that this software alone creates a legally effective will unless owner-supplied jurisdictional evidence exists.
- [ ] Owner/executor/beneficiary authority matrix is approved and distinct from token/passphrase possession.
- [ ] Email, webhook and script destination registration includes defined ownership/domain verification and re-verification rules.
- [ ] Scheduler, retry, cancellation, duplicate-trigger and audit-checkpoint behavior is observed with synthetic data.
- [ ] Backup restore and rollback are independently observed against an approved recovery objective.
- [ ] Negative tests fail on deliberately broken config, authorization or trigger behavior.

## Changes requiring owner approval

- Declaring the repo a legally operative estate product in any jurisdiction.
- Choosing who may unlock, check in, revoke tokens or approve post-trigger actions.
- Approving external destination classes, domain/account verification evidence and recovery rules.
- Setting cancellation windows, dispute handling and irreversible-action policy.
- Setting recovery objectives, backup custody and release criteria.

## Release and rollback policy

Owner approval is required for threat model, delivery channels, action authority, credentials, legal scope and recovery objectives. Stop promotion if trigger, security or restore evidence is missing. Roll back software to a verified artifact; already delivered messages or executed scripts cannot be assumed reversible.
