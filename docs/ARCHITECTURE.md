# Architecture Deep Dive

This document describes the internal design and data flow of Digital Will.

## High-Level Components

```
┌─────────────────┐
│   CLI (dw)      │
└────────┬────────┘
         │
┌────────▼────────┐     ┌──────────────────────┐
│  HTTP API       │────▶│  Scheduler           │
│  (loopback)     │     │  - Tick loop         │
└────────┬────────┘     │  - Worker pool       │
         │              │  - Retry engine      │
┌────────▼────────┐     └──────────┬───────────┘
│  Will Service   │                │
│  Action Service │◀───────────────┘
│  Crypto Engine  │
└────────┬────────┘
         │
┌────────▼────────┐
│  SQLite (WAL)   │
│  - wills        │
│  - actions      │
│  - audit_log    │
│  - crypto_meta  │
└─────────────────┘
```

## Core Data Flow

### 1. Will Creation

1. User provides passphrase via CLI or `/unlock`
2. `crypto.Engine.Initialize()` derives KEK via PBKDF2
3. `crypto.DecryptDEK()` opens the per-will DEK
4. `will.Service.Create()` encrypts payload with DEK + AAD
5. Record stored with `encrypted_payload` and `crypto_meta_id`

### 2. Check-in

- `POST /wills/{id}/checkin` or CLI
- Updates `last_check_in` and `next_check_in_deadline`
- Only allowed when status = `ACTIVE`
- Emits `check_in` audit event

### 3. Trigger Path (Scheduler)

1. `Scheduler.tick()` queries `GetOverdue()`
2. CAS `TransitionToPending()` (only one goroutine succeeds)
3. `processWill()` creates `action_executions` in `QUEUED` state
4. Dispatches to bounded worker pool
5. `retryAction()`:
   - Loads full `CryptoMeta`
   - Decrypts DEK
   - Decrypts will payload + action config
   - CAS claim (`QUEUED` → `IN_PROGRESS`)
   - Executes via `notification.Service`
   - On failure: `FailExecution()` with retry schedule

### 4. Notification Execution

- **SMTP**: Supports `none` / `starttls` / `tls` with strict TLS enforcement option
- **Webhook**: 30s timeout, optional `InsecureSkipVerify`
- **Script**: Isolated environment variables, timeout, output capture

## Concurrency Model

- **Database**: Single writer connection (`SetMaxOpenConns(1)`)
- **Scheduler Workers**: Bounded semaphore (`workerSem`)
- **Crypto Engine**: `sync.RWMutex` protected
- **API Rate Limiters**: Per-token `sync.Mutex` protected map with lazy cleanup

## State Machines

### Will Status

| From                | To                  | Allowed By     |
|---------------------|---------------------|----------------|
| DRAFT               | ACTIVE              | User           |
| ACTIVE              | PAUSED              | User           |
| PAUSED              | ACTIVE              | User           |
| ACTIVE              | PENDING_TRIGGER     | Scheduler only |
| PENDING_TRIGGER     | TRIGGERED           | Scheduler only |
| Any non-terminal    | ARCHIVED            | User           |

### Action Execution Status

`QUEUED` → `IN_PROGRESS` → `COMPLETED`
                ↓
             `FAILED` → retry or `EXHAUSTED`

## Security Boundaries

- **Crypto boundary**: All encryption/decryption happens inside `crypto.Engine`
- **Auth boundary**: `authMiddleware` runs before any will/action handlers
- **Network boundary**: `loopbackOnly` middleware
- **Memory boundary**: `memguard.Enclave` + explicit wiping

## Error Handling Philosophy

- Never return detailed decryption errors to clients (`ErrDecryptFailed` is opaque)
- Audit events are best-effort but never silent on critical paths
- Scheduler failures are logged but do not crash the daemon

## Future Extension Points

- Pluggable notification drivers (Signal, Matrix, etc.)
- Multi-user support (separate KEKs per user)
- Distributed scheduler (multiple nodes with leader election)
- Web UI (HTMX + static assets)

---

This architecture prioritizes **simplicity**, **auditability**, and **cryptographic correctness** over distributed systems complexity.