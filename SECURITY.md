# Security Model

Digital Will is designed with a **zero-trust** philosophy toward the host system and database administrator.

## Threat Model

### Protected Assets
- Will content (encrypted payloads)
- Action configurations (SMTP credentials, webhook URLs, script commands)
- Passphrases (never stored)
- Audit log integrity

### Adversaries Considered
- Database administrator with full SQL access
- Host compromise (root access)
- Network attacker (mitigated by loopback-only API)
- Malicious scheduled job or backup process

## Cryptographic Design

### Key Hierarchy
1. **User Passphrase** (never persisted)
2. **KEK** (Key Encryption Key) — derived via PBKDF2-SHA256
   - Minimum 600,000 iterations (NIST SP 800-132)
   - Calibrated to target ≥500ms derivation time
   - Stored only in `memguard.Enclave`
3. **DEK** (Data Encryption Key) — 256-bit random per will
   - Encrypted under KEK using AES-256-GCM
   - Never written to disk in plaintext

### Encryption
- **Algorithm**: AES-256-GCM (authenticated encryption)
- **Nonce**: 12-byte random per encryption (never reused)
- **AAD**: `will_id|crypto_meta_id` for payload binding
- **Memory Safety**: All intermediate keys wiped with `memguard.WipeBytes`

### Audit Log Integrity
Every audit record includes a SHA-256 hash chain:

```
checksum = SHA256(
    previous_checksum ||
    record_id ||
    event_type ||
    actor ||
    will_id ||
    timestamp ||
    metadata_json
)
```

Any modification to `actor`, `will_id`, or `event_type` will break the chain.

## Runtime Protections

### API Security
- **Loopback Only**: Rejects all non-127.0.0.1 connections
- **Token Auth**: SHA-256 hashed tokens stored in database
- **Constant-Time Comparison**: `subtle.ConstantTimeCompare`
- **Rate Limiting**: 60 requests/minute per token
- **Security Headers**: CSP, X-Frame-Options, Referrer-Policy

### Scheduler Hardening
- Bounded worker pool prevents resource exhaustion
- CAS (Compare-And-Set) prevents duplicate triggers
- Stale execution recovery (10-minute timeout)
- No secret material passed to notification templates

### Database
- `PRAGMA foreign_keys = ON`
- WAL mode with `synchronous = NORMAL`
- Single-writer connection (`SetMaxOpenConns(1)`)
- Parameterized queries only

## Operational Security Recommendations

1. **Run the daemon as a dedicated low-privilege user**
2. **Store the database and backups on encrypted volumes** (LUKS, FileVault, BitLocker)
3. **Never expose the API port publicly**
4. **Rotate the master passphrase periodically** using `crypto rotate`
5. **Regularly verify the audit chain**:
   ```go
   brokenSeq, brokenID, err := auditSvc.VerifyChain()
   ```
6. **Keep backups encrypted** and test restore procedures
7. **Monitor the health endpoint** for scheduler stalls or crypto lock state

## What Is NOT Protected

- Memory dumps while the daemon is running (KEK lives in enclave)
- Physical access to a running machine with the daemon unlocked
- Compromise of the machine while the crypto engine is initialized

## Reporting Security Issues

Please report vulnerabilities privately by opening an issue with the `security` label or contacting the maintainer directly. Do not disclose details publicly until a fix is released.

---

**Current Security Posture**: Strong. All known classes of runtime decryption and tampering attacks have been mitigated in the final implementation.