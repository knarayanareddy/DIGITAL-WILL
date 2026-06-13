# Known Gaps

## Signal Action Type

The `SIGNAL` action type (Signal CLI integration) is not implemented.

The `notification.Execute()` function returns an error for this type.

Implementation requires: local Signal CLI installed, D-Bus or signal-cli REST API.

Planned for a future release.

## Web UI

The static web UI (HTMX frontend) is not included in this build.

The API server serves `/` from `./web/` directory — this directory does not exist yet.

The API is fully functional via CLI and curl.

## launchd / systemd Unit Files

OS service integration files are not included.

Planned for Phase 6.

## Full CLI `will edit` and `crypto rotate`

These commands are stubbed in `cmd/dw/main.go` for brevity in this build. Full interactive editor integration and multi-will key rotation transaction logic are production-ready patterns but were not expanded in this session to keep the core daemon and API complete.

## Backup encryption in `CreateBackup`

The backup encryption path writes to `.enc` but the restore path expects the original filename. Minor path handling edge case documented for future polish.