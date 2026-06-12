

```markdown
---
title: "Digital Will — Production Engineering Specification"
version: "2.0.0"
status: "APPROVED — Single Source of Truth (SSOT)"
owner: "TBD / DRI: <assign before first production release>"
security_reviewer: "TBD"
last_reviewed: "2026-06-12"
compatibility_policy: "Semantic Versioning 2.0.0 — CLI/config/API/DB schema are 
                        backward-compatible within a MAJOR version"
requirement_language: "RFC 2119 — MUST/MUST NOT/SHOULD/SHOULD NOT/MAY"
---

# Digital Will — Production Engineering Specification

## Changelog

| Version | Date       | Author | Summary                                      |
|---------|------------|--------|----------------------------------------------|
| 1.0.0   | 2024-xx-xx | TBD    | Initial design doc                           |
| 2.0.0   | 2026-06-12 | TBD    | Full rewrite — SSOT upgrade, SLO/ops layer,  |
|         |            |        | migration contract, incident response, ADRs,  |
|         |            |        | RFC 2119 requirement language, DR section     |

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Document Governance](#2-document-governance)
3. [Goals, Non-Goals, and Constraints](#3-goals-non-goals-and-constraints)
4. [System Context and Operating Model](#4-system-context-and-operating-model)
5. [Architecture](#5-architecture)
6. [Module Specification](#6-module-specification)
7. [Data Model and Schema (Canonical)](#7-data-model-and-schema-canonical)
8. [API and Interface Specification](#8-api-and-interface-specification)
9. [CLI Specification](#9-cli-specification)
10. [Cryptography and Key Management](#10-cryptography-and-key-management)
11. [Security Model](#11-security-model)
12. [State Machine: Will Lifecycle](#12-state-machine-will-lifecycle)
13. [Scheduler and Trigger Engine](#13-scheduler-and-trigger-engine)
14. [Notification and Action Execution](#14-notification-and-action-execution)
15. [Observability, Logging, and Health](#15-observability-logging-and-health)
16. [Operational Health Objectives (OHOs)](#16-operational-health-objectives-ohos)
17. [Testing Strategy](#17-testing-strategy)
18. [Build, Packaging, and Release](#18-build-packaging-and-release)
19. [Upgrade and Migration Contract](#19-upgrade-and-migration-contract)
20. [Rollout and Phased Delivery Plan](#20-rollout-and-phased-delivery-plan)
21. [Disaster Recovery, Backup, and Restore](#21-disaster-recovery-backup-and-restore)
22. [Incident Response and Vulnerability Handling](#22-incident-response-and-vulnerability-handling)
23. [Dependency Registry and Failure Modes](#23-dependency-registry-and-failure-modes)
24. [Platform Support Matrix](#24-platform-support-matrix)
25. [Architectural Decision Records (ADRs)](#25-architectural-decision-records-adrs)
26. [Open Questions and Future Work](#26-open-questions-and-future-work)
27. [Glossary](#27-glossary)

---

## 1. Executive Summary

**Digital Will** is a self-hosted, encryption-first, dead-man's-switch daemon that allows a
user to pre-compose a set of encrypted "will" documents (messages, files, credentials,
instructions) and have them automatically delivered to designated recipients if the user
fails to check in within a configurable time window.

The system operates entirely on-device: no cloud services, no third-party key escrow,
no network dependency for storage. It is designed for individuals who want cryptographic
guarantees over their own posthumous or emergency communications without trusting any
external party with their data or timing logic.

This document is the **single source of truth** for building, reviewing, auditing, operating,
and evolving the Digital Will system. All implementation decisions MUST be traceable to a
section in this document or to an ADR recorded in Section 25.

---

## 2. Document Governance

### 2.1 Roles and Responsibilities

| Role                    | Responsibility                                              | Assigned To |
|-------------------------|-------------------------------------------------------------|-------------|
| **DRI (Owner)**         | Final decision authority on spec changes; signs off PRs     | TBD         |
| **Security Reviewer**   | Reviews all changes to Sections 10, 11, 22                  | TBD         |
| **Release Manager**     | Owns Section 18–20; tags releases; manages migration runs   | TBD         |
| **On-call / Maintainer**| Owns incident response (Section 22); monitors health        | TBD         |

### 2.2 Document Lifecycle

- **Draft** → author opens PR against `main`; at least one reviewer MUST approve.
- **Approved (SSOT)** → merged to `main`; becomes the authoritative reference.
- **Deprecated section** → marked with `> ⚠️ DEPRECATED as of vX.Y.Z — see Section N`
  before removal in the next MAJOR version.

### 2.3 SSOT Rules

- Every canonical definition (schema, API contract, config key, error code) MUST appear in
  exactly one section of this document.
- If implementation diverges from this doc, the doc is authoritative — open a bug against
  the implementation, not the doc, unless a spec change is formally approved.
- Cross-references between sections MUST use section anchors (e.g., `Section 7.3`),
  not prose descriptions.

### 2.4 Compatibility Policy

This project follows **Semantic Versioning 2.0.0**:

| Change type                              | Version bump |
|------------------------------------------|-------------|
| Breaking CLI flag / API / config / schema| MAJOR       |
| New backward-compatible feature          | MINOR       |
| Bug fix, security patch                  | PATCH       |

The MAJOR version in this document's front matter MUST match the `version` field in
`config.go` and the `user_version` SQLite PRAGMA.

---

## 3. Goals, Non-Goals, and Constraints

### 3.1 Goals

- **G1.** Deliver a single statically-linked binary daemon (`digitalwilld`) for Linux,
  macOS, and (stretch) Windows.
- **G2.** Store all sensitive data encrypted at rest using a user-supplied passphrase with
  no plaintext ever written to disk.
- **G3.** Trigger will delivery automatically if the user fails to check in within a
  configurable dead-man window.
- **G4.** Support multiple independent wills, each with configurable recipients,
  action types, and trigger windows.
- **G5.** Provide a localhost-only web UI and a CLI; no public network exposure by default.
- **G6.** Integrate natively with systemd (Linux) and launchd (macOS) for daemon lifecycle.
- **G7.** Produce a tamper-evident audit log for every state transition and action execution.
- **G8.** Remain fully functional with zero internet connectivity (except for outbound
  delivery actions the user configures).

### 3.2 Non-Goals

- **NG1.** Cloud sync, remote storage, or any third-party key escrow.
- **NG2.** Multi-user / multi-machine shared state.
- **NG3.** A mobile native app (web UI is mobile-browser accessible on localhost only).
- **NG4.** Legal enforceability as a formal last will and testament — this is a
  communications delivery tool, not a legal document.
- **NG5.** Real-time collaboration or recipient acknowledgment tracking.

### 3.3 Hard Constraints

| ID   | Constraint                                                                       |
|------|----------------------------------------------------------------------------------|
| C1   | SQLite MUST be the sole runtime storage dependency — no Postgres, Redis, etc.    |
| C2   | The user passphrase MUST NEVER be stored or logged in any form.                  |
| C3   | All will content MUST be encrypted before being written to disk.                 |
| C4   | The web UI MUST bind to `127.0.0.1` only and MUST require an auth token.         |
| C5   | The daemon MUST run with the minimum OS privileges required (see Section 11.4).  |
| C6   | No telemetry, analytics, or outbound connections except user-configured actions. |
| C7   | Forward-only DB migrations; downgrade MUST trigger a documented restore procedure|
| C8   | All action executions MUST be idempotent and persisted before attempt.           |

---

## 4. System Context and Operating Model

### 4.1 Operating Model

Digital Will is a **single-user, single-machine** system. The threat model, architecture,
and operational posture are designed accordingly:

```
┌─────────────────────────────────────────────────────────────┐
│                     USER'S MACHINE                          │
│                                                             │
│   ┌──────────┐    ┌──────────────┐    ┌─────────────────┐  │
│   │  CLI     │    │  Web UI      │    │  systemd/       │  │
│   │ (dw CLI) │    │ (localhost   │    │  launchd        │  │
│   └────┬─────┘    │  :8472)      │    │  (watchdog)     │  │
│        │          └──────┬───────┘    └────────┬────────┘  │
│        └──────────────────┼─────────────────────┘          │
│                           │ HTTP/Unix socket                 │
│                    ┌──────▼───────────────────────┐         │
│                    │       digitalwilld            │         │
│                    │                               │         │
│                    │  ┌──────────┐ ┌───────────┐  │         │
│                    │  │Scheduler │ │ Crypto Eng│  │         │
│                    │  └──────────┘ └───────────┘  │         │
│                    │  ┌──────────┐ ┌───────────┐  │         │
│                    │  │ Action   │ │ Check-in  │  │         │
│                    │  │ Executor │ │ Handler   │  │         │
│                    │  └──────────┘ └───────────┘  │         │
│                    │  ┌──────────────────────────┐ │         │
│                    │  │  SQLite DB (encrypted)   │ │         │
│                    │  └──────────────────────────┘ │         │
│                    └──────────────────────────────-┘         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
              │ outbound only, user-configured actions
              ▼
    ┌─────────────────────────┐
    │  External Delivery:     │
    │  SMTP, Webhook, Signal, │
    │  custom script          │
    └─────────────────────────┘
```

### 4.2 Trust Boundaries

| Zone             | Members                                     | Trust Level        |
|------------------|---------------------------------------------|--------------------|
| **Trusted**      | `digitalwilld` process, SQLite DB on disk   | Full               |
| **Semi-trusted** | Localhost CLI, localhost web UI (authed)    | User-level (authed)|
| **Untrusted**    | Network interfaces, other local processes,  | Zero trust         |
|                  | SMTP relays, webhook endpoints              |                    |

---

## 5. Architecture

### 5.1 Layer Map

```
┌──────────────────────────────────────────────────────────┐
│  INTERFACE LAYER                                          │
│  CLI (cobra)          Web UI (HTTP/HTML/HTMX)            │
└────────────────────────────┬─────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────┐
│  API LAYER                                                │
│  REST handlers (/api/v1/*)   Auth middleware              │
│  Request validation          Error normalization          │
└────────────────────────────┬─────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────┐
│  CORE SERVICE LAYER                                       │
│  WillService     CheckInService    SchedulerService       │
│  ActionService   NotificationSvc   AuditService           │
└────────────────────────────┬─────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────┐
│  CRYPTO ENGINE                                            │
│  KEK/DEK management   AES-GCM encrypt/decrypt             │
│  PBKDF2 key derivation   Key zeroing   Rotation           │
└────────────────────────────┬─────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────┐
│  STORAGE LAYER                                            │
│  SQLite (WAL mode)   Migration runner   Backup manager    │
└──────────────────────────────────────────────────────────┘
```

### 5.2 Critical Path: Will Trigger Flow

```
TICK (every SchedulerInterval, default 60s)
│
├─► Query all ACTIVE wills WHERE next_check_in_deadline <= now()
│         │
│         ├─ No overdue wills → log heartbeat, sleep
│         │
│         └─ Overdue will found:
│               │
│               ├─► Set will.status = PENDING_TRIGGER (atomic CAS)
│               ├─► Write audit_log: "trigger_initiated"
│               ├─► Load & decrypt will content (DEK from crypto engine)
│               ├─► For each action in will.actions (ordered):
│               │     ├─► Persist action_execution row (status=QUEUED)
│               │     ├─► Execute action (SMTP / webhook / script)
│               │     ├─► On success: mark COMPLETED, write audit_log
│               │     └─► On failure: mark FAILED, retry up to MaxRetries
│               │           └─► After MaxRetries: mark EXHAUSTED, alert user
│               └─► Set will.status = TRIGGERED, write audit_log: "triggered"
```

### 5.3 Critical Path: Check-In Flow

```
USER CHECK-IN (CLI or web UI)
│
├─► Validate auth token (MUST match token in DB, not expired)
├─► Load will(s) by ID or "all active"
├─► Compute new next_check_in_deadline = now() + will.check_in_interval
├─► UPDATE will SET last_check_in = now(), 
│                   next_check_in_deadline = <computed>
│                   WHERE status = ACTIVE AND id = ?
├─► Write audit_log: "check_in" with timestamp
└─► Return success; zero passphrase from memory
```

---

## 6. Module Specification

| Module              | Package path            | Responsibility                                           | Owner |
|---------------------|-------------------------|----------------------------------------------------------|-------|
| `scheduler`         | `internal/scheduler`    | Tick loop, deadline evaluation, trigger dispatch         | TBD   |
| `will`              | `internal/will`         | Will CRUD, lifecycle transitions, validation             | TBD   |
| `action`            | `internal/action`       | Action registry, executor, retry logic, idempotency      | TBD   |
| `checkin`           | `internal/checkin`      | Check-in handler, deadline extension, token validation   | TBD   |
| `crypto`            | `internal/crypto`       | KEK/DEK, AES-GCM, PBKDF2, zeroing, rotation             | TBD   |
| `notification`      | `internal/notification` | SMTP, webhook, signal-cli, script dispatch               | TBD   |
| `audit`             | `internal/audit`        | Structured audit log writes, tamper-detection            | TBD   |
| `storage`           | `internal/storage`      | SQLite connection pool, WAL, migrations, backup          | TBD   |
| `api`               | `internal/api`          | HTTP handlers, auth middleware, request/response models  | TBD   |
| `config`            | `internal/config`       | Config file load, validation, env override               | TBD   |
| `health`            | `internal/health`       | Health endpoint, scheduler liveness probe, OHO checks   | TBD   |
| `cli`               | `cmd/dw`                | Cobra command tree, CLI UX                              | TBD   |

### 6.1 Module Isolation Rule

Modules MUST NOT import each other in a cycle. The dependency graph MUST be a DAG:
`cli/api → services → crypto/audit/storage`. The `crypto` package MUST NOT import
any service package.

---

## 7. Data Model and Schema (Canonical)

> **SSOT Rule:** This section is the canonical schema definition. Migration files MUST
> match this exactly. If a migration diverges, the migration is wrong.

### 7.1 Schema Version

```sql
PRAGMA user_version = 2; -- MUST match app MAJOR version
```

### 7.2 Table: `wills`

```sql
CREATE TABLE IF NOT EXISTS wills (
    id                      TEXT PRIMARY KEY,          -- UUIDv4
    name                    TEXT NOT NULL,
    status                  TEXT NOT NULL              -- ENUM: see Section 12
                            CHECK(status IN (
                              'DRAFT','ACTIVE','PAUSED',
                              'PENDING_TRIGGER','TRIGGERED','ARCHIVED'
                            )),
    check_in_interval_sec   INTEGER NOT NULL,          -- e.g. 604800 = 7 days
    last_check_in           INTEGER,                   -- Unix epoch, nullable
    next_check_in_deadline  INTEGER,                   -- Unix epoch, nullable
    max_retries             INTEGER NOT NULL DEFAULT 3,
    created_at              INTEGER NOT NULL,          -- Unix epoch
    updated_at              INTEGER NOT NULL,          -- Unix epoch
    encrypted_payload       BLOB,                      -- AES-GCM ciphertext
    crypto_meta_id          TEXT NOT NULL REFERENCES crypto_meta(id),
    CONSTRAINT valid_interval CHECK(check_in_interval_sec > 0)
);

CREATE INDEX idx_wills_status_deadline
    ON wills(status, next_check_in_deadline)
    WHERE status = 'ACTIVE';
```

### 7.3 Table: `actions`

```sql
CREATE TABLE IF NOT EXISTS actions (
    id          TEXT PRIMARY KEY,                      -- UUIDv4
    will_id     TEXT NOT NULL REFERENCES wills(id) ON DELETE CASCADE,
    type        TEXT NOT NULL                          -- ENUM:
                CHECK(type IN (
                  'SMTP','WEBHOOK','SIGNAL','SCRIPT'
                )),
    position    INTEGER NOT NULL,                      -- execution order
    config      BLOB NOT NULL,                         -- AES-GCM encrypted JSON
    crypto_meta_id TEXT NOT NULL REFERENCES crypto_meta(id),
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE(will_id, position)
);
```

### 7.4 Table: `action_executions`

```sql
-- One row per execution attempt per action per trigger event.
-- This table is the idempotency and retry ledger.
CREATE TABLE IF NOT EXISTS action_executions (
    id              TEXT PRIMARY KEY,                  -- UUIDv4
    action_id       TEXT NOT NULL REFERENCES actions(id),
    will_id         TEXT NOT NULL REFERENCES wills(id),
    trigger_event_id TEXT NOT NULL,                   -- groups all actions from one trigger
    attempt         INTEGER NOT NULL DEFAULT 1,
    status          TEXT NOT NULL
                    CHECK(status IN (
                      'QUEUED','IN_PROGRESS','COMPLETED','FAILED','EXHAUSTED'
                    )),
    error_message   TEXT,                              -- last error, redacted of secrets
    started_at      INTEGER,
    completed_at    INTEGER,
    created_at      INTEGER NOT NULL
);

CREATE INDEX idx_action_exec_status
    ON action_executions(will_id, status, created_at);
```

### 7.5 Table: `audit_log`

```sql
-- Append-only. Rows MUST NEVER be updated or deleted by application code.
CREATE TABLE IF NOT EXISTS audit_log (
    id          TEXT PRIMARY KEY,                      -- UUIDv4
    timestamp   INTEGER NOT NULL,                      -- Unix epoch (milliseconds)
    event_type  TEXT NOT NULL,                         -- see Section 7.5.1
    will_id     TEXT,                                  -- nullable for system events
    actor       TEXT NOT NULL,                         -- "daemon","cli","webui","system"
    metadata    TEXT,                                  -- JSON blob, secrets NEVER included
    checksum    TEXT NOT NULL                          -- SHA-256(prev_row_id || event_type
                                                       --   || timestamp || metadata)
);

CREATE INDEX idx_audit_will ON audit_log(will_id, timestamp);
CREATE INDEX idx_audit_time ON audit_log(timestamp);
```

#### 7.5.1 Canonical Audit Event Types

| Event Type              | Emitted When                                          |
|-------------------------|-------------------------------------------------------|
| `will_created`          | New will persisted (DRAFT)                            |
| `will_activated`        | Will transitioned to ACTIVE                           |
| `will_paused`           | Will transitioned to PAUSED                           |
| `will_archived`         | Will transitioned to ARCHIVED                         |
| `check_in`              | Successful check-in, deadline extended                |
| `check_in_failed`       | Auth failure on check-in attempt                      |
| `trigger_initiated`     | Scheduler detected overdue will, CAS succeeded        |
| `action_queued`         | action_execution row written (QUEUED)                 |
| `action_completed`      | Action delivered successfully                         |
| `action_failed`         | Action attempt failed (retry pending)                 |
| `action_exhausted`      | Max retries exceeded                                  |
| `triggered`             | All actions processed, will marked TRIGGERED          |
| `key_rotation`          | DEK rotated for a will                                |
| `daemon_start`          | Daemon process started                                |
| `daemon_stop`           | Daemon process stopped (graceful)                     |
| `migration_run`         | DB migration applied                                  |
| `backup_created`        | Manual or scheduled backup completed                  |
| `config_changed`        | Config file reloaded (path, changed keys — no values) |

### 7.6 Table: `crypto_meta`

```sql
CREATE TABLE IF NOT EXISTS crypto_meta (
    id              TEXT PRIMARY KEY,                  -- UUIDv4
    kek_id          TEXT NOT NULL,                     -- identifies KEK version
    dek_ciphertext  BLOB NOT NULL,                     -- DEK encrypted under KEK
    dek_nonce       BLOB NOT NULL,                     -- 12-byte AES-GCM nonce for DEK
    pbkdf2_salt     BLOB NOT NULL,                     -- 32-byte salt
    pbkdf2_iters    INTEGER NOT NULL,                  -- min 600000 (NIST SP 800-132)
    created_at      INTEGER NOT NULL,
    rotated_at      INTEGER                            -- set on key rotation
);
```

### 7.7 Table: `tokens`

```sql
CREATE TABLE IF NOT EXISTS tokens (
    id          TEXT PRIMARY KEY,                      -- UUIDv4
    token_hash  TEXT NOT NULL UNIQUE,                  -- SHA-256 of raw token (raw never stored)
    label       TEXT,
    expires_at  INTEGER,                               -- nullable = non-expiring
    created_at  INTEGER NOT NULL,
    last_used   INTEGER
);
```

### 7.8 Table: `schema_migrations`

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  INTEGER NOT NULL,
    checksum    TEXT NOT NULL                          -- SHA-256 of migration SQL file
);
```

---

## 8. API and Interface Specification

### 8.1 API Versioning Policy

- All endpoints are namespaced under `/api/v1/`.
- `/api/v1/` is **stable within MAJOR version 2**. Breaking changes require a new major
  version (`/api/v2/`) and a minimum 90-day deprecation window for `/api/v1/`.
- Clients MUST send `Content-Type: application/json`.
- All responses MUST include `Content-Type: application/json` and a `X-Request-ID` header.

### 8.2 Authentication

Every request to `/api/v1/*` MUST include:
```
Authorization: Bearer <raw_token>
```
The daemon validates by hashing the raw token (SHA-256) and comparing against `tokens.token_hash`.
Constant-time comparison MUST be used to prevent timing attacks.

### 8.3 Error Response Model (Canonical)

```json
{
  "error": {
    "code": "WILL_NOT_FOUND",
    "message": "No will found with the given ID.",
    "request_id": "uuid"
  }
}
```

#### 8.3.1 Canonical Error Codes

| HTTP Status | Code                    | Meaning                                 |
|-------------|-------------------------|-----------------------------------------|
| 400         | `VALIDATION_ERROR`      | Request body or params failed validation|
| 401         | `UNAUTHORIZED`          | Missing or invalid token                |
| 403         | `FORBIDDEN`             | Token valid but action not permitted    |
| 404         | `WILL_NOT_FOUND`        | Will ID not found                       |
| 404         | `ACTION_NOT_FOUND`      | Action ID not found                     |
| 409         | `INVALID_TRANSITION`    | Will state machine rejects transition   |
| 429         | `RATE_LIMITED`          | Too many check-in attempts              |
| 500         | `INTERNAL_ERROR`        | Unhandled server error                  |
| 503         | `CRYPTO_UNAVAILABLE`    | Crypto engine not initialized           |

### 8.4 Endpoint Catalogue

#### Wills

| Method | Path                        | Description                                  |
|--------|-----------------------------|----------------------------------------------|
| GET    | `/api/v1/wills`             | List all wills (status, metadata — no content)|
| POST   | `/api/v1/wills`             | Create a new will (DRAFT)                    |
| GET    | `/api/v1/wills/:id`         | Get will metadata                            |
| PUT    | `/api/v1/wills/:id`         | Update will (DRAFT or PAUSED only)           |
| DELETE | `/api/v1/wills/:id`         | Archive will (soft delete — status=ARCHIVED) |
| POST   | `/api/v1/wills/:id/activate`| Transition DRAFT/PAUSED → ACTIVE             |
| POST   | `/api/v1/wills/:id/pause`   | Transition ACTIVE → PAUSED                   |

#### Check-In

| Method | Path                             | Description                                  |
|--------|----------------------------------|----------------------------------------------|
| POST   | `/api/v1/checkin`                | Check in (all active wills)                  |
| POST   | `/api/v1/wills/:id/checkin`      | Check in a specific will                     |

#### Actions

| Method | Path                                      | Description                       |
|--------|-------------------------------------------|-----------------------------------|
| GET    | `/api/v1/wills/:id/actions`               | List actions for a will           |
| POST   | `/api/v1/wills/:id/actions`               | Add action to will                |
| PUT    | `/api/v1/wills/:id/actions/:aid`          | Update action                     |
| DELETE | `/api/v1/wills/:id/actions/:aid`          | Remove action                     |

#### Audit Log

| Method | Path                             | Description                                  |
|--------|----------------------------------|----------------------------------------------|
| GET    | `/api/v1/audit`                  | Query audit log (filter by will, time range) |
| GET    | `/api/v1/audit/:id`              | Get single audit event                       |

#### System

| Method | Path            | Description                                       |
|--------|-----------------|---------------------------------------------------|
| GET    | `/api/v1/health`| Health check (no auth required — see Section 15.3)|
| GET    | `/api/v1/status`| Daemon status, scheduler state, crypto status     |
| POST   | `/api/v1/tokens`| Create new auth token (requires existing token)   |
| DELETE |`/api/v1/tokens/:id`| Revoke token                                 |

---

## 9. CLI Specification

### 9.1 Compatibility Promise

CLI flags and subcommands are **stable within MAJOR version 2**. Flag renames MUST go
through a MINOR version deprecation cycle (old flag kept, emits deprecation warning)
before removal in the next MAJOR version.

### 9.2 Command Tree

```
dw
├── daemon
│   ├── start           Start the daemon (foreground unless --detach)
│   ├── stop            Graceful stop (SIGTERM)
│   ├── status          Print scheduler state, will count, last check-in times
│   └── reload          Reload config without restart
│
├── will
│   ├── list            List all wills with status and deadline
│   ├── create          Interactively create a new will
│   ├── show <id>       Show will metadata and actions
│   ├── edit <id>       Edit will (opens $EDITOR on decrypted JSON, re-encrypts)
│   ├── activate <id>   Activate a DRAFT or PAUSED will
│   ├── pause <id>      Pause an ACTIVE will
│   └── archive <id>    Archive a will
│
├── checkin             Check in (all active wills); --id for specific will
│
├── action
│   ├── list <will-id>  List actions for a will
│   ├── add <will-id>   Add action interactively
│   └── remove <aid>    Remove an action
│
├── crypto
│   ├── rotate <id>     Rotate DEK for a will (requires passphrase)
│   └── verify <id>     Verify ciphertext integrity (HMAC check)
│
├── backup
│   ├── create          Create encrypted backup of DB + crypto meta
│   └── restore <file>  Restore from backup (see Section 21)
│
├── audit
│   ├── list            Query audit log (--will, --since, --until, --limit)
│   └── verify          Verify audit log chain checksums
│
└── token
    ├── create          Create new API/web UI token
    └── revoke <id>     Revoke token
```

### 9.3 Global Flags

| Flag              | Default               | Description                               |
|-------------------|-----------------------|-------------------------------------------|
| `--config`        | `~/.digitalwill/config.toml` | Config file path                   |
| `--db`            | `~/.digitalwill/db.sqlite`   | SQLite DB path                     |
| `--log-level`     | `info`                | `debug`, `info`, `warn`, `error`          |
| `--log-format`    | `json`                | `json`, `text`                            |
| `--socket`        | `~/.digitalwill/daemon.sock` | Unix socket for daemon IPC         |

---

## 10. Cryptography and Key Management

### 10.1 Key Hierarchy

```
User Passphrase (never stored)
        │
        ▼  PBKDF2-SHA256 (≥600,000 iterations, 32-byte salt)
        │
   Key Encryption Key (KEK) — 256-bit, in-memory only, zeroed on shutdown
        │
        ▼  AES-256-GCM (encrypt DEK)
        │
   Data Encryption Key (DEK) ciphertext — stored in crypto_meta
        │
        ▼  AES-256-GCM (encrypt will content and action configs)
        │
   Encrypted Payload — stored in wills.encrypted_payload / actions.config
```

### 10.2 PBKDF2 Parameters

| Parameter      | Value               | Reference              |
|----------------|---------------------|------------------------|
| Hash function  | SHA-256             | FIPS 180-4             |
| Min iterations | 600,000             | NIST SP 800-132        |
| Salt length    | 32 bytes (random)   | NIST SP 800-132        |
| Output length  | 32 bytes (256-bit)  | —                      |

The iteration count MUST be calibrated at startup to target ≥500ms on the host machine.
The resulting count MUST be stored in `crypto_meta.pbkdf2_iters` and used for that
record's future decryption.

### 10.3 AES-256-GCM Parameters

- Nonce: 12 bytes, randomly generated per encryption operation (MUST NEVER be reused).
- Tag: 16 bytes (standard GCM).
- Associated Data (AAD): `will_id || crypto_meta_id` (provides binding, prevents
  ciphertext from being moved between will records).

### 10.4 Memory Zeroing

- The raw passphrase, KEK, and decrypted DEK MUST be zeroed from memory immediately
  after use using a memory-safe zeroing function (not a compiler-optimizable memset).
- In Go: use `github.com/awnumar/memguard` or equivalent that prevents compiler
  optimization from eliding the zero.

### 10.5 Key Rotation Procedure

1. User invokes `dw crypto rotate <will-id>`.
2. Daemon prompts for current passphrase → derives current KEK → decrypts DEK.
3. Daemon generates a new DEK (32 random bytes).
4. Decrypts all will payload and action configs under old DEK.
5. Re-encrypts all under new DEK with new nonces.
6. Generates new `crypto_meta` row (new salt, new nonce, new kek_id); sets
   `old_meta.rotated_at = now()`.
7. Updates all relevant `wills` and `actions` rows atomically in a single transaction.
8. Writes `audit_log: "key_rotation"`.
9. Zeros all intermediate key material.
10. The old `crypto_meta` row is retained for audit purposes and MUST NOT be deleted.

### 10.6 Passphrase Change

Passphrase change is a full key rotation: new PBKDF2 salt + iterations → new KEK → 
new DEK (step 3 above uses the new passphrase to wrap the same re-derived or new DEK).

---

## 11. Security Model

### 11.1 Threat Model — DREAD-Scored Threats

| # | Threat                                       | D | R | E | A | D | Score | Mitigation            |
|---|----------------------------------------------|---|---|---|---|---|-------|-----------------------|
| T1| Disk theft — attacker reads SQLite file      | 5 | 4 | 3 | 5 | 3 | 20    | AES-256-GCM at rest   |
| T2| Memory scraping — attacker dumps process RAM | 4 | 2 | 2 | 4 | 3 | 15    | memguard, key zeroing |
| T3| Local port scan — finds web UI on localhost  | 3 | 3 | 4 | 3 | 3 | 16    | Token auth, lo-only   |
| T4| Token theft — attacker steals auth token     | 4 | 2 | 3 | 4 | 3 | 16    | Token expiry, revoke  |
| T5| SMTP relay intercept — will content in transit|4 | 2 | 2 | 4 | 3 | 15    | TLS enforced on SMTP  |
| T6| Log scraping — secrets leak into logs        | 5 | 2 | 2 | 5 | 2 | 16    | Denylist (Sec 15.2)   |
| T7| Action replay — duplicate delivery           | 3 | 2 | 2 | 3 | 2 | 12    | Idempotency key (C8)  |
| T8| Scheduler clock manipulation                 | 3 | 1 | 2 | 4 | 2 | 12    | Monotonic + wall clock|
| T9| SQL injection via API inputs                 | 5 | 2 | 3 | 5 | 2 | 17    | Parameterized queries |
| T10| Audit log tampering                         | 4 | 2 | 2 | 4 | 2 | 14    | Chain checksum verify |

*DREAD scale: D=Damage, R=Reproducibility, E=Exploitability, A=Affected users, D=Discoverability. Each 1–5.*

### 11.2 Security Requirements (MUST)

- Queries MUST use parameterized statements; no string concatenation in SQL.
- Auth token comparison MUST use constant-time comparison (`subtle.ConstantTimeCompare`).
- Web UI MUST enforce `Content-Security-Policy: default-src 'self'` and `X-Frame-Options: DENY`.
- SMTP delivery MUST use TLS (STARTTLS or direct TLS); plaintext SMTP MUST be rejected
  unless explicitly overridden by the user with an acknowledged risk prompt.
- The daemon MUST reject any request arriving on a non-loopback interface.
- Audit log rows MUST NEVER be modified or deleted by application code.

### 11.3 Security Recommendations (SHOULD)

- SHOULD rotate auth tokens every 90 days.
- SHOULD use an expiring token for web UI sessions (default: 24h).
- SHOULD alert the user (local notification or log `WARN`) if `audit_log` chain
  checksum verification fails.

### 11.4 OS Privilege Hardening

#### Linux (systemd unit)

```ini
[Service]
User=digitalwill
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=yes
PrivateDevices=yes
ReadWritePaths=/var/lib/digitalwill
CapabilityBoundingSet=
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM
```

#### macOS (launchd plist)

```xml
<key>UserName</key><string>_digitalwill</string>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
```

The daemon MUST NOT run as root. A dedicated low-privilege system user MUST be created
by the installer.

---

## 12. State Machine: Will Lifecycle

```
         ┌─────────┐
         │  DRAFT  │◄──────────────────┐
         └────┬────┘                   │
              │ activate               │ (edit returns to DRAFT)
              ▼                        │
         ┌─────────┐                   │
    ┌───►│  ACTIVE │                   │
    │    └────┬────┘                   │
    │         │ pause                  │
    │    ┌────▼────┐                   │
    └────│  PAUSED │───────────────────┘
         └────┬────┘
              │ (scheduler trigger or manual test)
              ▼
    ┌──────────────────┐
    │  PENDING_TRIGGER │  (atomic CAS; only scheduler may set this)
    └────────┬─────────┘
             │ actions complete
             ▼
        ┌──────────┐
        │ TRIGGERED│
        └────┬─────┘
             │ archive (manual or auto)
             ▼
        ┌──────────┐
        │ ARCHIVED │ (terminal)
        └──────────┘
```

### 12.1 Transition Rules

| From               | To                  | Allowed Actor       | Condition                          |
|--------------------|---------------------|---------------------|------------------------------------|
| DRAFT              | ACTIVE              | User (CLI/UI)       | Will has ≥1 action configured      |
| ACTIVE             | PAUSED              | User (CLI/UI)       | Any time                           |
| PAUSED             | ACTIVE              | User (CLI/UI)       | Any time                           |
| ACTIVE             | PENDING_TRIGGER     | Scheduler only      | next_check_in_deadline ≤ now()     |
| PENDING_TRIGGER    | TRIGGERED           | Scheduler only      | All actions COMPLETED or EXHAUSTED |
| TRIGGERED/ARCHIVED | any                 | —                   | MUST NOT transition (terminal+1)   |
| Any                | ARCHIVED            | User (CLI/UI)       | Any non-terminal state             |

---

## 13. Scheduler and Trigger Engine

### 13.1 Tick Loop Specification

```
MUST run every SchedulerInterval seconds (default: 60, min: 10, max: 3600).
MUST use both wall clock and monotonic clock to detect system suspend/resume.
On resume from suspend: MUST run an immediate evaluation pass.
MUST write a heartbeat to the health state store every tick (Section 15.3).
MUST NOT execute actions synchronously in the tick goroutine — 
  dispatch to a bounded worker pool (default: 4 workers, configurable).
```

### 13.2 Idempotency Contract

- Before any action attempt, the executor MUST verify that an `action_execution` row
  exists with `status = QUEUED`. If the row is missing, the action MUST NOT be executed.
- If a row exists with `status = COMPLETED`, the executor MUST skip (already delivered).
- Transition from `QUEUED → IN_PROGRESS` MUST use a compare-and-set UPDATE:
  ```sql
  UPDATE action_executions SET status = 'IN_PROGRESS', started_at = ?
  WHERE id = ? AND status = 'QUEUED';
  ```
  If 0 rows affected, another worker owns this execution; skip silently.

### 13.3 Retry Policy

| Attempt | Delay before retry  |
|---------|---------------------|
| 1       | Immediate           |
| 2       | 5 minutes           |
| 3       | 30 minutes          |
| N > max | Mark EXHAUSTED, log WARN, local alert to user |

Retry schedule MUST be persisted in `action_executions.started_at` (next attempt time)
so the scheduler can correctly schedule retries across daemon restarts.

---

## 14. Notification and Action Execution

### 14.1 Action Types

#### SMTP

```json
{
  "type": "SMTP",
  "to": ["recipient@example.com"],
  "subject": "Message from {user_name}",
  "body_template": "...",
  "smtp_host": "smtp.example.com",
  "smtp_port": 587,
  "smtp_user": "...",
  "smtp_password": "...",    // stored encrypted (action.config is encrypted blob)
  "tls": "starttls"          // "starttls" | "tls" | "none" (none requires user ack)
}
```

#### Webhook

```json
{
  "type": "WEBHOOK",
  "url": "https://...",
  "method": "POST",
  "headers": { "X-Secret": "..." },
  "body_template": "...",
  "tls_verify": true
}
```

#### Script

```json
{
  "type": "SCRIPT",
  "path": "/usr/local/bin/my-delivery.sh",
  "args": ["--will-id", "{will_id}"],
  "timeout_sec": 30,
  "env": { "DW_WILL_NAME": "{will_name}" }
}
```

### 14.2 Template Variables

All `*_template` fields support:

| Variable          | Value                      |
|-------------------|----------------------------|
| `{user_name}`     | Config `user.name`         |
| `{will_name}`     | `wills.name`               |
| `{will_id}`       | `wills.id`                 |
| `{triggered_at}`  | ISO 8601 timestamp         |
| `{last_check_in}` | ISO 8601 or "never"        |

---

## 15. Observability, Logging, and Health

### 15.1 Structured Log Format

All log output MUST be structured JSON (default) with the following base fields:

```json
{
  "ts":      "2026-06-12T10:00:00.000Z",
  "level":   "info",
  "msg":     "trigger_initiated",
  "will_id": "uuid",
  "actor":   "scheduler",
  "module":  "scheduler",
  "request_id": "uuid or empty"
}
```

### 15.2 Fields That MUST NEVER Appear in Logs

| Field                        | Reason                            |
|------------------------------|-----------------------------------|
| `passphrase`                 | User secret                       |
| `kek`, `dek`                 | Key material                      |
| `token` (raw)                | Auth credential                   |
| `smtp_password`              | Credential                        |
| `encrypted_payload` (raw)    | Ciphertext blob — no value in logs|
| Will content (decrypted)     | Sensitive personal data           |
| `action.config` (decrypted)  | May contain credentials           |

### 15.3 Health Endpoint

`GET /api/v1/health` — no authentication required — returns:

```json
{
  "status":          "ok",            // "ok" | "degraded" | "critical"
  "scheduler": {
    "last_tick":     "2026-06-12T09:59:00Z",
    "status":        "running"        // "running" | "stalled" | "stopped"
  },
  "crypto": {
    "initialized":   true
  },
  "db": {
    "reachable":     true,
    "schema_version": 2
  },
  "version":         "2.0.0"
}
```

The health endpoint MUST return `200` only if all subsystems are `ok`. Degraded/critical
states MUST return `503`.

### 15.4 Local Alert Channels

When action exhaustion or scheduler stall occurs, the daemon MUST emit an alert via
at least one of (in order of preference, configurable):

1. Desktop notification (`notify-send` on Linux, `osascript` on macOS)
2. Write a marker file to a configurable path (for external monitoring)
3. Log at `ERROR` level (always done regardless of the above)

---

## 16. Operational Health Objectives (OHOs)

> These are not SLAs to external customers. They are operational health invariants that
> the daemon MUST maintain and that operators/users can verify.

| OHO ID | Invariant                                                         | Measurement                                   | Alert Threshold       |
|--------|-------------------------------------------------------------------|-----------------------------------------------|-----------------------|
| OHO-1  | Scheduler MUST tick at least once per `2 × SchedulerInterval`    | `now() - health.last_tick`                    | > 2× interval → STALL |
| OHO-2  | No ACTIVE will's deadline MUST be missed by > 5 minutes          | `now() - next_check_in_deadline` for ACTIVE   | > 300s → log ERROR    |
| OHO-3  | Action MUST NOT remain `IN_PROGRESS` > 10 minutes                | `now() - started_at` for IN_PROGRESS rows     | > 600s → reset + retry|
| OHO-4  | Audit log chain checksum MUST be valid                            | `dw audit verify`                             | Any failure → CRITICAL|
| OHO-5  | DB MUST be reachable within 1 second                              | Health check query latency                    | > 1000ms → DEGRADED   |

---

## 17. Testing Strategy

### 17.1 Testing Pyramid

```
         ▲  E2E / integration tests
         │  (full daemon lifecycle, real SQLite, real SMTP mock)
         │
         │  Component / service tests
         │  (scheduler logic, crypto engine, state machine)
         │
         │  Unit tests
         ▼  (each function, table-driven, no I/O)
```

### 17.2 Unit Test Requirements

- `crypto` package: MUST have 100% test coverage of encrypt/decrypt/rotate/zero paths.
- `scheduler` package: MUST have table-driven tests for all state machine transitions
  (valid and invalid).
- `action_executions` idempotency: MUST have unit tests for concurrent CAS behavior
  (use in-memory SQLite).
- All error paths MUST be tested explicitly (not just happy path).

### 17.3 Integration Test Requirements

- Start real daemon on ephemeral port with in-memory SQLite.
- Exercise: create will → activate → simulate missed deadline → verify trigger → 
  verify audit log → verify action execution record.
- SMTP delivery: use a mock SMTP server (e.g., `github.com/emersion/go-smtp` in test mode).
- Test key rotation end-to-end: encrypt, rotate, verify old ciphertext unreachable,
  decrypt with new key.

### 17.4 Security Test Requirements

- MUST run `gosec` on every CI build; zero `HIGH` findings allowed.
- MUST run `govulncheck` on every CI build; zero unpatched vulnerabilities in direct deps.
- MUST include a test that verifies the web UI rejects requests without a valid token
  (401) and rejects requests from non-loopback (403).
- MUST include a test that verifies audit log rows cannot be modified (attempt UPDATE via
  raw DB handle and verify trigger/constraint fires).

### 17.5 CI Pipeline Requirements

```
lint (golangci-lint) → unit tests → integration tests → security scans → build matrix
```

All gates MUST pass before merge to `main`.

---

## 18. Build, Packaging, and Release

### 18.1 Build

```makefile
build:
    CGO_ENABLED=1 go build \
      -ldflags="-X main.version=$(VERSION) -X main.commit=$(GIT_SHA) -s -w" \
      -o dist/digitalwilld ./cmd/digitalwilld
    CGO_ENABLED=1 go build \
      -ldflags="-X main.version=$(VERSION) -X main.commit=$(GIT_SHA) -s -w" \
      -o dist/dw ./cmd/dw
```

> Note: `CGO_ENABLED=1` is required for the SQLite driver. Cross-compilation targets
> MUST use appropriate CGO cross-compile toolchains.

### 18.2 Release Checklist

Before tagging a release:

- [ ] All CI gates pass on `main`
- [ ] `CHANGELOG.md` updated with all changes since last release
- [ ] DB schema version (`PRAGMA user_version`) incremented if schema changed
- [ ] Migration file added and tested (Section 19)
- [ ] Platform support matrix verified (Section 24)
- [ ] Security scan clean (`gosec`, `govulncheck`)
- [ ] If MAJOR version: deprecation period for previous API version started
- [ ] Release notes include explicit "upgrade instructions" link (Section 19)
- [ ] Binary checksums (SHA-256) published alongside release artifacts

### 18.3 Artifact Naming

```
digitalwill-v{VERSION}-{OS}-{ARCH}.tar.gz
  └── digitalwilld  (daemon)
  └── dw            (CLI)
  └── install.sh
  └── SHA256SUMS
```

---

## 19. Upgrade and Migration Contract

### 19.1 Forward-Only Migration Policy

- DB migrations are **forward-only**. There is no automated downgrade path.
- Downgrading a MAJOR version MUST be done via the backup/restore procedure (Section 21).
- Each migration file MUST be named `{version}_{description}.sql` and MUST be
  idempotent (safe to run twice without error).
- The migration runner MUST verify `schema_migrations.checksum` matches the file
  on disk before considering a migration "already applied." If the checksum diverges,
  the daemon MUST refuse to start and log a `CRITICAL` error.

### 19.2 Migration Runner Behavior

```
On daemon start:
  1. Open DB; check PRAGMA user_version
  2. Compare to embedded migration list
  3. If behind: run pending migrations in version order, inside a transaction
  4. On any migration error: ROLLBACK, log CRITICAL, refuse to start
  5. On success: update PRAGMA user_version, insert schema_migrations row
  6. Emit audit_log: "migration_run" for each applied migration
```

### 19.3 Pre-Upgrade User Instructions

Before upgrading across a MINOR or MAJOR version:

1. Run `dw backup create` → confirm backup file created and checksum noted.
2. Stop the daemon: `dw daemon stop`.
3. Replace binaries.
4. Start daemon: migrations run automatically.
5. Run `dw audit verify` to confirm audit log chain intact.
6. Run `dw daemon status` to confirm scheduler running.

### 19.4 Config Compatibility

- Config keys deprecated in MINOR version: MUST emit a `WARN` log and continue to work.
- Config keys removed in MAJOR version: MUST emit `ERROR` and refuse to start,
  with a message pointing to the migration guide.

---

## 20. Rollout and Phased Delivery Plan

### 20.1 Milestones

| Phase | Milestone                                      | Exit Criteria                                          |
|-------|------------------------------------------------|--------------------------------------------------------|
| **0** | Foundation: storage, crypto, config, daemon    | Unit tests pass; `dw daemon start` works; DB created   |
| **1** | Will CRUD + state machine + CLI                | Create/activate/pause/archive via CLI; audit log works |
| **2** | Check-in handler + scheduler + trigger engine  | End-to-end trigger test passes; idempotency verified   |
| **3** | Action executors: SMTP, Webhook, Script        | Each action type delivers in integration test          |
| **4** | Web UI + token auth + localhost enforcement    | All API endpoints work; security tests pass            |
| **5** | Key rotation + backup/restore                  | Rotation test passes; backup → restore verified        |
| **6** | systemd + launchd integration + installer      | Install script works on Ubuntu 22.04 + macOS 14        |
| **7** | Hardening + gosec + full CI pipeline           | Zero HIGH gosec findings; all CI gates green           |
| **8** | v2.0.0 release                                 | Release checklist (Section 18.2) complete              |

### 20.2 Feature Flags (Runtime)

A lightweight feature flag mechanism MUST be implemented in the `config` package
to allow shipping Phase 5–6 features behind a flag during development:

```toml
[features]
key_rotation = true       # default: true in Phase 5+
web_ui       = true       # default: true in Phase 4+
launchd      = false      # default: false until Phase 6 macOS support complete
```

Flags MUST be evaluated at startup and logged (key name + value, not secrets).

---

## 21. Disaster Recovery, Backup, and Restore

### 21.1 Backup Scope

A Digital Will backup MUST include:

| Item                        | Notes                                          |
|-----------------------------|------------------------------------------------|
| `db.sqlite`                 | Full DB including wills, actions, audit log    |
| `crypto_meta` entries       | Embedded in DB — backed up with DB             |
| `config.toml`               | Paths, intervals, user settings                |

The backup file itself MUST be AES-256-GCM encrypted using a backup passphrase
(MAY be the same as the main passphrase, but SHOULD be noted separately).

### 21.2 Backup File Format

```
digitalwill-backup-{timestamp}.dwbak
  Header (JSON, plaintext):
    { "version": "2.0.0", "created_at": "...", "db_checksum": "sha256:...", 
      "config_checksum": "sha256:..." }
  AES-GCM encrypted body:
    [ db.sqlite bytes | config.toml bytes ]
```

### 21.3 Restore Procedure

```
dw backup restore <backup-file>
  1. Prompt for backup passphrase; derive KEK; decrypt body
  2. Verify SHA-256 checksums of extracted db.sqlite and config.toml
  3. Stop daemon if running
  4. Write files to target paths (prompt before overwrite)
  5. Run migration runner (Section 19.2) against restored DB
  6. Start daemon
  7. Emit audit_log: "backup_restored"
```

### 21.4 Backup Schedule Recommendation (User-Facing Guidance)

The daemon SHOULD remind the user to create a backup:

- After each key rotation
- If no backup has been created in > 30 days (configurable)
- After any MAJOR version upgrade

### 21.5 RPO and RTO Targets (Single-User Local System)

| Target | Value                                                         |
|--------|---------------------------------------------------------------|
| RPO    | Last manual backup (automated backup is a stretch goal)       |
| RTO    | < 10 minutes from `dw backup restore` on same hardware        |

### 21.6 Annual Restore Drill (Recommendation)

Users SHOULD verify their backup is restorable at least annually by running
`dw backup restore --dry-run <backup-file>`, which decrypts, verifies checksums,
and reports success without writing any files.

---

## 22. Incident Response and Vulnerability Handling

### 22.1 Vulnerability Disclosure

- Report vulnerabilities via: [GitHub Security Advisory] or email: `security@<project-domain>` (TBD).
- Expected response SLA: acknowledge within 72 hours; patch within 14 days for CRITICAL,
  30 days for HIGH, 90 days for MEDIUM/LOW.
- Embargoed vulnerabilities will be disclosed publicly after patch release.

### 22.2 Severity Levels

| Severity | Examples                                              | Response Target |
|----------|-------------------------------------------------------|-----------------|
| CRITICAL | Passphrase/key leakage; remote code execution         | Patch in 14 days|
| HIGH     | Auth bypass; plaintext will content in logs           | Patch in 30 days|
| MEDIUM   | Audit log manipulation; token brute-force             | Patch in 90 days|
| LOW      | Information disclosure (version, timing)              | Next release    |

### 22.3 Key Compromise Procedure

If the user suspects their passphrase or key material has been compromised:

1. Immediately run `dw daemon stop`.
2. Run `dw backup create` (with a new backup passphrase stored offline).
3. Run `dw crypto rotate <will-id>` for all wills with a new strong passphrase.
4. Revoke all tokens: `dw token revoke --all`.
5. Review `dw audit list --since <suspected-compromise-date>` for unauthorized actions.
6. If `action_exhausted` events are present: determine whether will content was delivered
   unexpectedly and contact intended recipients if needed.

### 22.4 Scheduler Stall Response

If OHO-1 fires (scheduler stall detected):

1. Check `dw daemon status` → if "stalled": `dw daemon stop && dw daemon start`.
2. Check system logs for OOM killer activity (`dmesg | grep -i kill`).
3. Run `dw audit list --since <last-known-tick>` to verify no deliveries were missed.
4. If missed deliveries suspected: manually inspect `action_executions` table.

---

## 23. Dependency Registry and Failure Modes

### 23.1 Go Dependencies (Direct)

| Package                          | Purpose                        | License  |
|----------------------------------|--------------------------------|----------|
| `modernc.org/sqlite`             | SQLite driver (pure Go option) | MIT      |
| `github.com/mattn/go-sqlite3`    | SQLite driver (CGO option)     | MIT      |
| `github.com/spf13/cobra`         | CLI framework                  | Apache-2 |
| `github.com/spf13/viper`         | Config management              | Apache-2 |
| `golang.org/x/crypto`            | PBKDF2, AES-GCM                | BSD      |
| `github.com/awnumar/memguard`    | Secure memory zeroing          | Apache-2 |
| `github.com/google/uuid`         | UUID generation                | BSD      |
| `go.uber.org/zap`                | Structured logging             | MIT      |

### 23.2 External Runtime Dependencies and Failure Modes

| Dependency         | Failure Mode                         | User Impact                | Daemon Behavior                         | Degraded Mode OK? |
|--------------------|--------------------------------------|----------------------------|-----------------------------------------|-------------------|
| SMTP relay         | Connection refused / timeout         | Action not delivered       | Retry per Section 13.3; EXHAUSTED alert | Yes (retry)       |
| Webhook endpoint   | 5xx / timeout                        | Action not delivered       | Retry per Section 13.3; EXHAUSTED alert | Yes (retry)       |
| Script             | Non-zero exit / timeout              | Action not delivered       | Mark FAILED; retry; EXHAUSTED alert     | Yes (retry)       |
| OS filesystem      | Disk full                            | DB write fails; daemon halt| Log CRITICAL; stop scheduler            | No                |
| OS clock           | Clock skew (NTP jump)                | Trigger timing affected    | Use monotonic clock; log WARN on jump   | Yes               |
| systemd/launchd    | Watchdog not available               | Daemon not auto-restarted  | Log WARN on startup; no crash           | Yes               |

---

## 24. Platform Support Matrix

| Platform         | Version            | Arch           | Tier  | Notes                              |
|------------------|--------------------|----------------|-------|------------------------------------|
| Linux            | Ubuntu 22.04+      | amd64, arm64   | Tier 1| systemd integration fully tested   |
| Linux            | Debian 11+         | amd64          | Tier 1| —                                  |
| Linux            | Fedora 38+         | amd64          | Tier 2| Community-tested                   |
| macOS            | 13 Ventura +       | amd64, arm64   | Tier 1| launchd integration tested         |
| macOS            | 12 Monterey        | amd64          | Tier 2| Best-effort                        |
| Windows          | 11 / Server 2022   | amd64          | Tier 3| Stretch goal; no service integration|

**Tier definitions:**
- **Tier 1:** Full CI coverage; blocking for release.
- **Tier 2:** Best-effort CI; non-blocking for release; maintained by community.
- **Tier 3:** No CI; community contributions welcome; no release guarantee.

---

## 25. Architectural Decision Records (ADRs)

### ADR-001: SQLite as the sole storage backend

- **Status:** Accepted
- **Date:** 2024-xx-xx
- **Context:** Need embedded, offline, single-file storage with no external service deps.
- **Decision:** Use SQLite in WAL mode.
- **Consequences:** Excellent for single-user local use. Not suitable for concurrent
  multi-process writes. Multi-machine sync is explicitly a non-goal (NG2).

---

### ADR-002: AES-256-GCM + PBKDF2 over alternative encryption schemes

- **Status:** Accepted
- **Date:** 2024-xx-xx
- **Context:** Need authenticated encryption; passphrase-based key derivation; no HSM.
- **Decision:** AES-256-GCM for data encryption; PBKDF2-SHA256 for key derivation.
- **Rejected alternatives:**
  - *ChaCha20-Poly1305:* Equally valid; deferred to avoid introducing `x/crypto/chacha20poly1305`
    dependency split. Can be revisited.
  - *Argon2id:* Stronger KDF; deferred due to less mature FIPS guidance. Revisit in v3.
- **Consequences:** NIST-standard, well-audited. Iteration count must be calibrated per device.

---

### ADR-003: Localhost-only web UI (no remote access)

- **Status:** Accepted
- **Date:** 2024-xx-xx
- **Context:** Remote access dramatically expands the attack surface and requires TLS cert management.
- **Decision:** Bind only to `127.0.0.1`. No built-in TLS, SSH tunnel, or ngrok support.
- **Consequences:** Users who want remote access must set up their own reverse proxy +
  TLS. This is intentional — it forces a conscious security decision by the user.

---

### ADR-004: Forward-only migrations; no auto-downgrade

- **Status:** Accepted
- **Date:** 2026-06-12
- **Context:** Auto-downgrade migrations are risky and rarely well-tested; backup/restore
  is safer for the rare case of needing to go back.
- **Decision:** Migrations are forward-only. Downgrade is done via backup restore.
- **Consequences:** Users must back up before upgrading MAJOR versions. See Section 21.

---

### ADR-005: No built-in telemetry or analytics

- **Status:** Accepted
- **Date:** 2024-xx-xx
- **Context:** This is a personal privacy tool. Any telemetry would undermine user trust.
- **Decision:** Zero telemetry. No crash reporting, no usage analytics.
- **Consequences:** Bug reports depend entirely on user-submitted logs (which exclude
  secrets per Section 15.2).

---

## 26. Open Questions and Future Work

| # | Question / Future Work                                      | Priority | Target Version |
|---|-------------------------------------------------------------|----------|----------------|
| 1 | Argon2id as KDF replacement (stronger than PBKDF2)          | Medium   | v3.0.0         |
| 2 | ChaCha20-Poly1305 as alternative cipher                     | Low      | v3.0.0         |
| 3 | Automated scheduled backups (daemon-managed)                | High     | v2.1.0         |
| 4 | Recipient acknowledgment tracking (confirm delivery receipt)| Low      | TBD            |
| 5 | Windows Service integration (Tier 3 → Tier 2)               | Medium   | v2.2.0         |
| 6 | `dw backup restore --dry-run` annual drill command          | High     | v2.1.0         |
| 7 | Signal-cli action type (requires Signal account)            | Medium   | v2.1.0         |
| 8 | Audit log export (JSON / CSV)                               | Low      | v2.1.0         |
| 9 | Passphrase strength meter in CLI/UI                         | Low      | v2.1.0         |
| 10| Formal security audit / pen test                            | High     | Before v2.0.0  |

---

## 27. Glossary

| Term                   | Definition                                                              |
|------------------------|-------------------------------------------------------------------------|
| **Dead-man's switch**  | A mechanism that triggers automatically if a user fails to act          |
| **DEK**                | Data Encryption Key — encrypts will content; itself encrypted under KEK |
| **DRI**                | Directly Responsible Individual — single accountable owner              |
| **KEK**                | Key Encryption Key — derived from user passphrase via PBKDF2            |
| **OHO**                | Operational Health Objective — internal health invariant                |
| **ADR**                | Architectural Decision Record — documents why a decision was made       |
| **CAS**                | Compare-and-Set — atomic conditional update to prevent race conditions  |
| **WAL**                | Write-Ahead Logging — SQLite mode for better concurrent read performance |
| **SSOT**               | Single Source of Truth — this document is the SSOT for all design       |
| **RFC 2119**           | IETF standard defining MUST/SHOULD/MAY requirement levels               |
| **PBKDF2**             | Password-Based Key Derivation Function 2 — NIST SP 800-132             |
| **AES-GCM**            | Advanced Encryption Standard — Galois/Counter Mode (authenticated)      |
| **Idempotency key**    | A persisted token ensuring an operation executes at most once           |
| **Trigger event**      | The scheduler event that initiates will delivery                        |
| **Action execution**   | One attempt by one action to deliver content                            |
```

---

### What this doc adds over the original (and why)

| Addition | Section | Why it matters |
|---|---|---|
| YAML front matter + status + changelog | §2 | Makes it a governed SSOT, not just a blueprint |
| Explicit DRI/owner/reviewer table | §2.1 | Accountability before production |
| Semantic versioning + compatibility policy | §2.4 | Users know what breaks when |
| Canonical SSOT rules (one definition, one place) | §2.3 | Prevents spec drift |
| C4-style system context diagram | §4 | Architecture communication for reviewers |
| RFC 2119 requirement language throughout | Throughout | Distinguishes invariants from suggestions |
| Formal error model + error code table | §8.3 | API consumers know what to expect |
| API versioning + deprecation policy | §8.1 | Upgrade safety |
| CLI compatibility promise | §9.1 | Users don't get surprised by breaking changes |
| DREAD threat scoring table | §11.1 | Prioritised, auditable security posture |
| Operational Health Objectives (OHOs) | §16 | Measurable daemon invariants |
| Scheduler stall detection + local alerts | §13, §15.4 | Operational observability |
| Forward-only migration contract | §19 | Safe upgrades, no silent data loss |
| Pre-upgrade checklist | §19.3 | Prevents foot-guns |
| Full backup/restore spec + DR targets | §21 | A production spec without DR is incomplete |
| Incident response + key compromise procedure | §22 | Teams need a playbook |
| Dependency failure mode table | §23.2 | Failure-mode thinking baked in |
| Restore drill recommendation | §21.6 | Backups that are never tested aren't backups |
| ADRs for all major decisions | §25 | "Why" survives longer than "what" |
| Open questions + future work table | §26 | Honest about what's unresolved |
