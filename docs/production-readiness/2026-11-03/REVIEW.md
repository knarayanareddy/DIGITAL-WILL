# Digital Will review

## Scope and verdict

Go daemon/CLI implementation at `69c0f12c13857027985a7d2e956245e79ab304cb`. Snapshot samples README, security documentation, `go.mod`, daemon entry point, the first 220 lines each of CLI and API, and crypto/scheduler/will tests. Scheduler, action, storage and crypto implementations were not sampled. No build, test, notification, backup or restore was run. Claims of production readiness are not established. [Plan](REMEDIATION_PLAN.md).

## Architecture

The daemon wires storage, crypto, audit, wills, actions, notifications and scheduling ([cmd/digitalwilld/main.go:48-63](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/cmd/digitalwilld/main.go#L48-L63)). The CLI separately opens the database in its creation path ([cmd/dw/main.go:55-68](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/cmd/dw/main.go#L55-L68)). README describes timed email, webhook and script actions and encrypted wills ([README.md:3-20](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/README.md#L3-L20)); actual delivery and recovery semantics remain unverified.

## Findings

- **DW-1 — High, confirmed claim limitation:** `SECURITY.md` includes `Host compromise (root access)` among adversaries ([lines 3-16](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/SECURITY.md#L3-L16)). Encryption at rest does not by itself protect an unlocked running process from a sufficiently privileged host attacker. Define exact at-rest versus runtime guarantees and narrow `zero trust in the host` claims ([README.md:5-7](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/README.md#L5-L7)).
- **DW-2 — Medium, confirmed static:** the create command ignores `config.Load`'s error before accessing the returned config ([cmd/dw/main.go:55-60](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/cmd/dw/main.go#L55-L60)); malformed or missing configuration can be obscured and may cause a failure depending on the returned value. Propagate and test the error.
- **DW-3 — High, unverified risk:** scheduler, retries and recovery are advertised ([README.md:14-19](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/README.md#L14-L19)), but side-effect execution and crash recovery were not inspected. A missed or duplicated trigger can have irreversible consequences. Test clock boundaries, check-in races, restart and idempotency before real use.
- **DW-4 — Medium, confirmed design limitation:** the documented audit chain is unkeyed SHA-256 over record fields ([SECURITY.md:37-52](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/SECURITY.md#L37-L52)). Without a separately protected trusted checkpoint, an attacker able to rewrite the database can recompute a chain or remove its tail. Actual storage implementation was not inspected; constrain the claim and establish independent anchoring.
- **DW-5 — Medium, confirmed static:** the complete tree contains `go.mod` but no `go.sum`. The sample lists versioned modules ([go.mod:3-14](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/go.mod#L3-L14)); verify reproducible module integrity and commit an appropriate checksum policy.

## Five disciplines

- **Correctness:** the state-machine test exercises transition calls ([service_test.go:27-49](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/internal/will/service_test.go#L27-L49)); actual scheduler-to-action behavior, races and delivery are unverified.
- **Testing:** crypto, scheduler and will test files exist. Scheduler tests create a minimal in-memory schema ([scheduler_test.go:16-22](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/internal/scheduler/scheduler_test.go#L16-L22)), not proof of production migrations or restart behavior. No tests were run.
- **DevOps:** backup and health modules exist in the tree; real encrypted backup, restore, monitoring, release and host outage behavior are unknown. A self-hosted dead-man switch requires an independently monitored deployment.
- **OWASP:** API uses loopback middleware in sampled setup ([server.go:59-65](https://github.com/knarayanareddy/DIGITAL-WILL/blob/69c0f12c13857027985a7d2e956245e79ab304cb/internal/api/server.go#L59-L65)); authentication/authorization coverage, webhook SSRF, script injection, secret handling and data protection require tracing complete routes and action execution.
- **UI/UX:** CLI, not an observed web UI. Verify clear check-in deadlines, irreversible action warnings, accessible prompts and failure states with operators.

## Owner decisions and limitations

Owner must define host threat model, executor authority, notification channels, cancellation windows, credential custody, legal use, backup location and recovery objectives. Do not entrust real wills or irreversible actions until independent security and recovery tests pass. Snapshot ranges: README 1-292, SECURITY 1-100, go.mod 1-15, daemon 1-109, CLI/API 1-220 of 284/503, and complete sampled crypto/scheduler/will tests. No external account, delivery, dependency audit, clock-failure test or restore was performed.
