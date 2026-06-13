PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL,
    checksum   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS crypto_meta (
    id             TEXT PRIMARY KEY,
    kek_id         TEXT NOT NULL,
    dek_ciphertext BLOB NOT NULL,
    dek_nonce      BLOB NOT NULL,
    pbkdf2_salt    BLOB NOT NULL,
    pbkdf2_iters   INTEGER NOT NULL,
    created_at     INTEGER NOT NULL,
    rotated_at     INTEGER
);

CREATE TABLE IF NOT EXISTS wills (
    id                     TEXT PRIMARY KEY,
    name                   TEXT NOT NULL,
    status                 TEXT NOT NULL
        CHECK(status IN (
            'DRAFT','ACTIVE','PAUSED',
            'PENDING_TRIGGER','TRIGGERED','ARCHIVED'
        )),
    check_in_interval_sec  INTEGER NOT NULL,
    last_check_in          INTEGER,
    next_check_in_deadline INTEGER,
    max_retries            INTEGER NOT NULL DEFAULT 3,
    created_at             INTEGER NOT NULL,
    updated_at             INTEGER NOT NULL,
    encrypted_payload      BLOB,
    crypto_meta_id         TEXT NOT NULL REFERENCES crypto_meta(id),
    CONSTRAINT valid_interval CHECK(check_in_interval_sec > 0)
);

CREATE INDEX IF NOT EXISTS idx_wills_status_deadline
ON wills(status, next_check_in_deadline)
WHERE status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS actions (
    id             TEXT PRIMARY KEY,
    will_id        TEXT NOT NULL REFERENCES wills(id) ON DELETE CASCADE,
    type           TEXT NOT NULL
        CHECK(type IN ('SMTP','WEBHOOK','SIGNAL','SCRIPT')),
    position       INTEGER NOT NULL,
    config         BLOB NOT NULL,
    crypto_meta_id TEXT NOT NULL REFERENCES crypto_meta(id),
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE(will_id, position)
);

CREATE TABLE IF NOT EXISTS action_executions (
    id               TEXT PRIMARY KEY,
    action_id        TEXT NOT NULL REFERENCES actions(id),
    will_id          TEXT NOT NULL REFERENCES wills(id),
    trigger_event_id TEXT NOT NULL,
    attempt          INTEGER NOT NULL DEFAULT 1,
    status           TEXT NOT NULL
        CHECK(status IN (
            'QUEUED','IN_PROGRESS','COMPLETED','FAILED','EXHAUSTED'
        )),
    error_message    TEXT,
    started_at       INTEGER,
    completed_at     INTEGER,
    next_retry_at    INTEGER,
    created_at       INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_action_exec_status
ON action_executions(will_id, status, created_at);

CREATE INDEX IF NOT EXISTS idx_action_exec_retry
ON action_executions(status, next_retry_at)
WHERE status = 'FAILED';

CREATE TABLE IF NOT EXISTS audit_log (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    id         TEXT NOT NULL UNIQUE,
    timestamp  INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    will_id    TEXT,
    actor      TEXT NOT NULL,
    metadata   TEXT,
    checksum   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_will ON audit_log(will_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_log(timestamp);

CREATE TABLE IF NOT EXISTS tokens (
    id         TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    label      TEXT,
    expires_at INTEGER,
    created_at INTEGER NOT NULL,
    last_used  INTEGER
);