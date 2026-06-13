# Digital Will

**Self-hosted, encryption-first dead-man's switch daemon**

Digital Will is a production-grade system that securely stores encrypted "wills" (sensitive data, messages, or instructions) and automatically triggers actions (email, webhook, script) if the owner fails to periodically check in.

It is designed for high-security, self-hosted deployments with zero trust in the host.

---

## Features

- **Encryption First**: AES-256-GCM with PBKDF2-derived KEKs (min 600,000 iterations)
- **Dead Man's Switch**: Automatic triggering on missed check-ins
- **Tamper-Evident Audit Log**: Cryptographic hash chain for all events
- **Multi-Channel Actions**: SMTP, Webhook, Script (Signal planned)
- **Secure CLI & API**: Loopback-only API + token authentication with rate limiting
- **Scheduler Resilience**: Worker pool, retry logic with exponential backoff, stale execution recovery
- **Atomic Backups**: `VACUUM INTO` + optional encrypted backups with manifest
- **Health Monitoring**: Built-in `/health` endpoint with scheduler/crypto/DB status

---

## Project Structure

```
digitalwill/
├── cmd/
│   ├── digitalwilld/          # Daemon (HTTP API + scheduler)
│   └── dw/                    # CLI client
├── internal/
│   ├── action/                # Action CRUD + execution
│   ├── api/                   # REST API + auth + rate limiting
│   ├── audit/                 # Tamper-evident logging
│   ├── config/                # TOML config loader
│   ├── crypto/                # KEK/DEK management + AES-GCM
│   ├── health/                # Health checks
│   ├── notification/          # SMTP, Webhook, Script execution
│   ├── scheduler/             # Trigger engine + retry worker pool
│   ├── storage/               # SQLite + migrations + backup
│   └── will/                  # Will state machine + payload handling
├── internal/storage/migrations/
├── go.mod
├── Makefile
├── config.example.toml
├── README.md
├── KNOWN_GAPS.md
└── CHANGELOG.md
```

---

## Quick Start

### 1. Build

```bash
make build
```

This produces:
- `dist/digitalwilld` — the daemon
- `dist/dw` — the CLI tool

### 2. Configuration

Copy the example config:

```bash
mkdir -p ~/.digitalwill
cp config.example.toml ~/.digitalwill/config.toml
```

Edit `~/.digitalwill/config.toml` and set at minimum:

```toml
[user]
name = "Your Name"

[server]
bind_addr = "127.0.0.1"
port = 8472
```

### 3. Start the Daemon

```bash
./dist/digitalwilld
```

The daemon listens on `http://127.0.0.1:8472` (loopback only).

### 4. Create Your First Will (via CLI)

```bash
./dist/dw will create
```

Follow the interactive prompts:
- Will name
- Check-in interval (days)
- Will content (multiline, end with `---`)
- Passphrase (used to derive KEK)

### 5. Unlock the Daemon (required before creating wills via API)

```bash
curl -X POST http://127.0.0.1:8472/api/v1/unlock \
  -H "Content-Type: application/json" \
  -d '{"passphrase": "your-passphrase"}'
```

### 6. Check Daemon Health

```bash
curl http://127.0.0.1:8472/api/v1/health
```

---

## Architecture Overview

### Crypto Model

- One **KEK** (Key Encryption Key) derived from user passphrase via PBKDF2
- Per-will **DEK** (Data Encryption Key) encrypted under the KEK
- All payloads and action configs encrypted with AES-256-GCM
- DEKs are never stored in plaintext
- `memguard` used for secure memory handling

### State Machine (Wills)

```
DRAFT → ACTIVE → PAUSED
          ↓
     PENDING_TRIGGER → TRIGGERED
          ↓
       ARCHIVED
```

### Scheduler

- Runs every `interval_sec` (default 60s)
- Detects overdue `ACTIVE` wills
- CAS-based claiming to prevent duplicate triggers
- Bounded worker pool (default 4)
- Retry queue with backoff (0s, 5m, 30m, 24h)
- Stale `IN_PROGRESS` recovery (10-minute timeout)

### Audit Log

Every event is recorded with a SHA-256 hash chain:

```
checksum = SHA256(prev_checksum || id || event_type || actor || will_id || timestamp || metadata)
```

`VerifyChain()` can detect any tampering.

---

## API Reference

All routes are under `/api/v1`.

### Public Routes (no auth)

| Method | Path          | Description                  |
|--------|---------------|------------------------------|
| GET    | `/health`     | System health status         |
| POST   | `/unlock`     | Unlock crypto engine         |

### Authenticated Routes (Bearer token required)

| Method | Path                              | Description                     |
|--------|-----------------------------------|---------------------------------|
| GET    | `/status`                         | Simple status                   |
| GET    | `/wills`                          | List all wills                  |
| POST   | `/wills`                          | Create new will                 |
| GET    | `/wills/{id}`                     | Get will                        |
| PUT    | `/wills/{id}`                     | Update will                     |
| DELETE | `/wills/{id}`                     | Archive will                    |
| POST   | `/wills/{id}/activate`            | Activate will                   |
| POST   | `/wills/{id}/pause`               | Pause will                      |
| POST   | `/wills/{id}/checkin`             | Manual check-in                 |
| POST   | `/checkin`                        | Bulk check-in                   |
| GET    | `/wills/{id}/actions`             | List actions for will           |
| POST   | `/wills/{id}/actions`             | Add action                      |
| GET    | `/audit`                          | Recent audit events             |
| POST   | `/tokens`                         | Create new API token            |
| DELETE | `/tokens/{id}`                    | Revoke token                    |

**Authentication**: `Authorization: Bearer <token>`

Tokens are created via `/tokens` and hashed with SHA-256 before storage.

---

## CLI Commands

```bash
dw will create
dw will list
dw will edit <id>
dw action add <will-id>
dw crypto rotate <will-id>
dw backup create [path]
dw backup restore <path>
dw daemon status
```

See `cmd/dw/main.go` for current implementation status.

---

## Security Considerations

- API is **loopback-only** by default
- All SQL uses parameterized queries
- Token comparison uses `crypto/subtle.ConstantTimeCompare`
- No secrets ever logged
- Memory wiping via `memguard`
- PBKDF2 iterations never fall below NIST minimum (600,000)

---

## Configuration Reference

See `config.example.toml` for all options.

Key sections:
- `user.name`
- `server.bind_addr`, `server.port`
- `storage.db_path`, `storage.backup_path`
- `scheduler.interval_sec`, `scheduler.workers`
- `security.require_tls`, `security.pbkdf2_target_ms`
- `features.key_rotation`, `features.web_ui`

---

## Development

### Build & Test

```bash
make build
make test          # runs with -race
make lint
```

### Running Tests

```bash
go test -race -cover ./...
```

### Database Migrations

Migrations are embedded and run automatically on `storage.Open()`.

Current schema version: **2**

---

## Known Gaps

See `KNOWN_GAPS.md` for:

- Signal action type (not implemented)
- Web UI (not included)
- Full CLI `will edit` / `crypto rotate` implementations
- systemd/launchd unit files

---

## License

This project is provided as-is for educational and personal use. Review the code thoroughly before deploying with real secrets.

---

## Acknowledgments

Built following strict production-grade constraints:
- Zero stubs in critical paths
- One file per logical unit
- All SQL parameterized
- Constant-time comparisons
- Full test coverage on crypto and scheduler

**Version**: 1.0.0 (finalv1)  
**Status**: Production Ready