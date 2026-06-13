# Changelog

All notable changes to Digital Will are documented here.

## [1.0.0] - 2026-06-13

### Added
- Complete production-grade implementation from scratch
- AES-256-GCM encryption with PBKDF2 KEK derivation (min 600k iterations)
- Tamper-evident audit log with full-field SHA-256 hash chain
- Dead-man's switch scheduler with CAS claiming and retry logic
- SMTP, Webhook, and Script action types with template rendering
- Loopback-only REST API with Bearer token auth + rate limiting
- Interactive CLI (`dw`) for will creation and daemon status
- Atomic `VACUUM INTO` backups with optional encryption
- Health check endpoint with scheduler/crypto/DB status
- Full test suites for crypto, will state machine, and scheduler
- Embedded SQLite migrations (v1 + v2)

### Security
- All SQL uses parameterized queries
- Constant-time token comparison (`subtle.ConstantTimeCompare`)
- No secrets ever logged
- `memguard` for secure key handling
- Memory wiping after key derivation

### Fixed (from earlier iterations)
- CryptoMeta full row loading before DEK decryption
- Scheduler CAS now supports both QUEUED and FAILED states
- Audit `VerifyChain()` now matches `Log()` hashing logic
- Text templates instead of HTML templates for notifications
- Config validation always runs

## [0.9.0] - Pre-release

- Initial architecture and core services
- Basic scheduler and API skeleton

[1.0.0]: https://github.com/knarayanareddy/DIGITAL-WILL/releases/tag/v1.0.0