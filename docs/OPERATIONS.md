# Operations Guide

## Starting the Daemon

### Manual Start

```bash
./dist/digitalwilld
```

### Recommended: Systemd (Linux)

Create `/etc/systemd/system/digitalwilld.service`:

```ini
[Unit]
Description=Digital Will Dead Man's Switch
After=network.target

[Service]
Type=simple
User=digitalwill
Group=digitalwill
ExecStart=/usr/local/bin/digitalwilld
Restart=always
RestartSec=10
Environment="HOME=/home/digitalwill"

[Install]
WantedBy=multi-user.target
```

Then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now digitalwilld
sudo journalctl -u digitalwilld -f
```

### macOS (launchd)

Create `~/Library/LaunchAgents/com.digitalwill.daemon.plist` and load it.

## Daily Operations

### Check Health

```bash
curl -s http://127.0.0.1:8472/api/v1/health | jq
```

Expected output when healthy:

```json
{
  "status": "ok",
  "scheduler": { "status": "running" },
  "crypto": { "initialized": true },
  "db": { "reachable": true },
  "version": "1.0.0"
}
```

### Create API Token

```bash
curl -X POST http://127.0.0.1:8472/api/v1/tokens \
  -H "Content-Type: application/json" \
  -d '{"label": "backup-script"}'
```

Save the returned token securely. It is shown **only once**.

### Manual Check-in (via API)

```bash
curl -X POST http://127.0.0.1:8472/api/v1/wills/<will-id>/checkin \
  -H "Authorization: Bearer <token>"
```

### View Recent Audit Events

```bash
curl -s http://127.0.0.1:8472/api/v1/audit | jq
```

## Backup & Restore

### Create Encrypted Backup

```bash
./dist/dw backup create /secure/location/backup-2026-06-13.sqlite --passphrase
```

Or via API (future enhancement).

### Restore

```bash
./dist/dw backup restore /secure/location/backup-2026-06-13.sqlite.enc
```

**Important**: The daemon must be stopped before restoring.

## Monitoring & Alerting

Recommended metrics to monitor:

- Scheduler status (`stalled` is critical)
- Crypto initialized state (`false` after restart requires unlock)
- Database reachability
- Number of `ACTIVE` wills vs `TRIGGERED`
- Failed action executions (look for `EXHAUSTED` status)

## Log Rotation

The daemon logs to stdout. Use your process supervisor (systemd, supervisord) to handle rotation.

Example journald configuration:

```ini
[Service]
StandardOutput=journal
StandardError=journal
```

## Graceful Shutdown

The daemon handles `SIGINT` / `SIGTERM` cleanly:
- Stops accepting new HTTP requests
- Waits for in-flight actions to complete (up to 10s)
- Closes database connections
- Emits `daemon_stop` audit event

## Upgrading

1. Stop the daemon
2. Replace binary
3. Start the daemon (migrations run automatically)
4. Re-unlock with passphrase if needed

Database schema is forward-compatible within major versions.

## Troubleshooting

### "crypto not initialized"

You must call `/unlock` after every daemon restart.

### Scheduler shows "stalled"

Check for:
- Database lock contention
- Extremely long-running actions
- System clock skew

### Actions stuck in QUEUED

Verify the scheduler is running and crypto is unlocked.

### Decryption failed during will creation

Ensure you are using the correct passphrase that was used when the `crypto_meta` row was created.

## Security Hardening Checklist

- [ ] Run as dedicated non-root user
- [ ] Database directory has `0700` permissions
- [ ] Backups stored on encrypted volume
- [ ] API token rotation policy in place
- [ ] Regular audit chain verification scheduled
- [ ] Health checks integrated into monitoring system
- [ ] Firewall blocks all inbound traffic except loopback to port 8472

---

**Last Updated**: 2026-06-13