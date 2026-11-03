# Digital Will remediation plan

Proposed only; no actions or checks were run. See [review](REVIEW.md). Dependencies: `DW-T1 → DW-T2 → DW-T3 → DW-T4 → DW-T5`.

### DW-T1 — Baseline and threat model (DW-1, DW-2)
Prerequisite: owner supplies isolated environment and synthetic data. Observed paths: `SECURITY.md`, `cmd/dw/main.go`, `internal/config/`.
1. Inspect complete configuration loading and reproduce errors without real wills.
2. Propagate configuration errors and test missing, invalid and inaccessible configuration paths.
3. Specify host-compromise, at-rest, unlocked-process and operator boundaries; correct overbroad documentation.
Deliverable: reproducible baseline and threat model. Verification: independent reviewer confirms a malformed configuration yields a clear error and checks claims against the defined attacker.
- [ ] Configuration failures are surfaced without unintended actions.
- [ ] Runtime host-compromise limitations are explicit.

### DW-T2 — Protect trigger and audit invariants (DW-3, DW-4)
Prerequisite: DW-T1 and owner-approved action policies. Observed paths: `internal/scheduler/scheduler.go`, `internal/action/service.go`, `internal/audit/audit.go`, `internal/api/server.go`.
1. Trace check-in, deadline, claim, retry, cancellation and completion across daemon restarts.
2. Define at-least-once delivery and idempotency boundaries, secret handling, webhook destination controls and script authorization.
3. Establish independently protected audit checkpoints or narrow the integrity claim; add negative tests for rewrite and truncation.
Deliverable: documented, enforced state and audit boundaries. Verification: simulate check-in races, duplicate delivery, clock changes, restart and tampered audit data with synthetic actions.
- [ ] Duplicate and missed-trigger handling is explicit and tested.
- [ ] Audit guarantees match the actual checkpoint threat model.

### DW-T3 — Automate meaningful checks (DW-2, DW-3, DW-5)
Prerequisite: DW-T2. Observed paths: `go.mod`, `internal/*/*_test.go`, `internal/storage/migrations/`; proposed CI workflow.
1. Generate and review module checksums under an approved dependency policy and capture clean-build results.
2. Add migration-backed integration tests for authentication, action execution, failure recovery and encrypted payload tampering.
3. Add a reproducible required CI gate without weakening existing tests.
Deliverable: reproducible security and lifecycle suite. Verification: deliberately break a deadline or authorization condition in isolation and confirm a negative test fails.
- [ ] Tests use real migrations and independent expected behavior.
- [ ] Module integrity and test failure propagation are recorded.

### DW-T4 — Observe deployment and recovery (preventive)
Prerequisite: DW-T3; owner approves monitoring, backup and recovery objectives. Observed paths: `internal/storage/backup.go`, `docs/OPERATIONS.md`, `internal/health/health.go`.
1. Define encrypted backup/key separation, alerting and safe behavior during host or network outages.
2. Stage the daemon with synthetic wills and non-delivering action sinks; exercise restart, missed check-in and cancellation.
3. Restore an actual encrypted backup in isolation and demonstrate rollback to a verified artifact.
Deliverable: observed recovery and operating runbook. Verification: independently inspect restored wills and scheduler state; a successful backup exit alone is insufficient.
- [ ] Restore and rollback meet approved objectives.
- [ ] Monitoring and action delivery failure paths are observed.

### DW-T5 — Operator UX and independent acceptance (preventive)
Prerequisite: DW-T4. Observed paths: `cmd/dw/main.go`, `docs/OPERATIONS.md`.
1. Test terminal prompts, deadline presentation and cancellation warnings with representative operators.
2. Verify errors do not expose secrets and that irreversible actions require the approved confirmation policy.
3. Independently rerun boundary, tamper and restore checks against the candidate revision.
Deliverable: acceptance record and residual risks. Verification: observe a synthetic full lifecycle including a denied or cancelled action.
- [ ] Operators can understand deadlines and recover from failures.
- [ ] Owner and independent reviewer accept the candidate and limitations.

## Gate and rollback

Owner approval is required for threat model, delivery channels, action authority, credentials, legal scope and recovery objectives. Stop promotion if trigger, security or restore evidence is missing. Roll back software to a verified artifact; already delivered messages or executed scripts cannot be assumed reversible.
