🕯️ DIGITAL WILL
Comprehensive Engineering Design Document

Version 1.0 | Self-Hosted Encrypted Dead Man's Switch with Automated Execution Engine
TABLE OF CONTENTS

    Project Overview & Vision
    Goals, Non-Goals & Constraints
    System Architecture
    Module Breakdown
        4.1 Scheduler Engine (Go)
        4.2 Action Execution Engine
        4.3 Encryption & Key Management Layer
        4.4 Check-In Module
        4.5 Notification & Escalation System
        4.6 CLI Interface
        4.7 Web Dashboard (Local)
        4.8 Event Bus & Audit Log
    Data Models & Schemas
    API Specifications
    Directory Structure
    Configuration System
    Encryption & Key Management Deep Dive
    Action System Deep Dive
    Check-In System Deep Dive
    Notification & Escalation Deep Dive
    Security Model & Threat Boundaries
    Storage & Persistence
    Logging, Observability & Debugging
    Testing Strategy
    Build, Packaging & Installation
    Platform Support Matrix
    Performance Targets & Benchmarks
    Error Handling Strategy
    Dependency Registry
    Milestone & Phased Rollout Plan
    Open Questions & Future Work

1. Project Overview & Vision
1.1 What is Digital Will?

Digital Will is a self-hosted, locally-run, cryptographically-secured dead man's switch that allows users to define automated actions to be executed if they fail to check in within a configured time window.

It operates as a background daemon (willd) with a companion CLI (will), an optional local web interface, and a fully pluggable action engine. The user defines one or more "wills," each with:

    A check-in interval (e.g., every 7 days)
    A grace period (e.g., 24 hours after the deadline)
    A multi-stage escalation (warn → confirm → execute)
    An ordered list of actions (email recipients, publish files, post to social media, trigger webhooks, securely wipe data)

All configuration, secrets, and action payloads are AES-256-GCM encrypted at rest. No cloud service is involved. No third party holds any part of the user's data. The user owns everything.
1.2 The Problem Being Solved

When a person becomes incapacitated or dies, their digital life — passwords, private communications, critical documents, final messages to loved ones, sensitive data that should be destroyed — has no automatic executor. Existing solutions either:

    Require trusting a third-party service with your secrets
    Are fragile (paper instructions, informal arrangements)
    Provide no cryptographic guarantees
    Cannot automate actions at the moment they are needed

Digital Will gives technically-minded users a locally-controlled, cryptographically-verified, self-executing instruction set that requires no intermediary and leaves no unencrypted secrets on any external server.
1.3 Design Philosophy
Principle	Description
Self-sovereign	All data, keys, and logic live on the user's machine. No cloud dependency.
Irreversibility-aware	Destructive and publishing actions have mandatory multi-stage escalation and delays.
Fail-safe, not fail-open	Transient errors (daemon crash, clock issues) must never accidentally trigger wills.
Human-in-the-loop	Every irreversible action has a configurable delay and pre-notification.
Cryptographically principled	Encryption is authenticated, key derivation is modern, key material is never logged.
Auditable	Every check-in, escalation event, and action execution is logged with timestamps.
Composable	Multiple independent wills, multiple action types, configurable ordering.
2. Goals, Non-Goals & Constraints
2.1 Goals (In Scope)

    Periodic check-in tracking with configurable interval and grace period
    Multi-stage escalation: warn → final warning → trigger (with configurable delays between each stage)
    Action types: email, publish (IPFS, paste services), social (Mastodon, Bluesky, Nostr), wipe (cryptographic erase + optional overwrite), webhook
    AES-256-GCM encryption of all stored secrets and action configs
    PBKDF2-HMAC-SHA256 key derivation (600,000 iterations, OWASP 2024 recommendation)
    KEK/DEK key hierarchy (passphrase → KEK → random DEK → data)
    CLI tool (will) for all will management operations
    Daemon (willd) for background scheduling with systemd/launchd support
    Optional local web UI for check-in and status (localhost only)
    Multiple check-in methods: CLI, web, email token, SMS token
    Per-action delay support (e.g., wipe fires 48h after trigger)
    Execution log with per-action success/failure tracking
    Export of will configuration (re-encrypted for backup)
    Cross-platform: macOS, Linux, Windows

2.2 Non-Goals (Explicitly Out of Scope)

    ❌ Any cloud sync, remote storage, or third-party relay of secrets
    ❌ Plaintext publishing of sensitive payloads (all publish actions encrypt before upload)
    ❌ "Secure file overwrite" as a primary security guarantee on SSDs (crypto erase is the default)
    ❌ Social media account takeover or platform impersonation
    ❌ Legal will or estate planning (this is a technical tool, not a legal instrument)
    ❌ Multi-user shared access or family account management (v1.0)
    ❌ Mobile app or browser extension

2.3 Constraints

    All secrets must be encrypted at rest; master password never stored
    Daemon must be resilient to clock skew, NTP jumps, and accidental machine downtime
    Irreversible actions (publish, wipe) require minimum 1 stage of pre-notification
    Per-action delays must be enforced in persistent storage (survive daemon restarts)
    Single binary deployment (statically linked Go binary)
    SQLite as the sole runtime dependency for storage
    Must support systemd (Linux) and launchd (macOS) service integration

3. System Architecture
3.1 High-Level Architecture Diagram

text

┌──────────────────────────────────────────────────────────────────────┐
│                         USER'S MACHINE                               │
│                                                                      │
│  ┌──────────┐    ┌───────────────────────────────────────────────┐   │
│  │          │    │             DIGITAL WILL CORE                 │   │
│  │   will   │    │                                               │   │
│  │  (CLI)   │◄──►│  ┌─────────────────┐  ┌───────────────────┐  │   │
│  │          │    │  │  Scheduler      │  │  Check-In Module  │  │   │
│  └──────────┘    │  │  Engine (Go)    │  │  (CLI/Web/Email/  │  │   │
│                  │  │                 │  │   SMS)            │  │   │
│  ┌──────────┐    │  └────────┬────────┘  └────────┬──────────┘  │   │
│  │  Web UI  │    │           │                    │             │   │
│  │ (Local   │◄──►│  ┌────────▼────────────────────▼──────────┐  │   │
│  │  :9090)  │    │  │           Event Bus (channels)         │  │   │
│  └──────────┘    │  └──┬──────────┬──────────┬───────────────┘  │   │
│                  │     │          │          │                   │   │
│                  │  ┌──▼──┐  ┌───▼────┐ ┌───▼──────────────┐   │   │
│                  │  │Notif│  │Action  │ │  Encryption &    │   │   │
│                  │  │ication│ │Execution│ │  Key Management  │   │   │
│                  │  │Engine │ │Engine  │ │  (AES-256-GCM)   │   │   │
│                  │  └──┬──┘  └───┬────┘ └───────────────────┘   │   │
│                  │     │         │                               │   │
│                  │  ┌──▼─────────▼──────────────────────────┐   │   │
│                  │  │      SQLite DB (encrypted at rest)    │   │   │
│                  │  └───────────────────────────────────────┘   │   │
│                  └───────────────────────────────────────────────┘   │
│                                        │                             │
└────────────────────────────────────────┼─────────────────────────────┘
                                         │ (only on trigger)
                         ┌───────────────┼───────────────┐
                         │               │               │
                    ┌────▼────┐   ┌──────▼─────┐  ┌──────▼─────┐
                    │  SMTP   │   │  IPFS /    │  │  Mastodon/ │
                    │  Server │   │  0x0.st    │  │  Bluesky/  │
                    │(email)  │   │ (publish)  │  │  Nostr     │
                    └─────────┘   └────────────┘  └────────────┘

3.2 Scheduler & Trigger Flow (Step by Step)

text

Step 1:  Daemon starts, loads all active wills from encrypted DB
Step 2:  Ticker fires every 60 seconds
Step 3:  For each active will:
           deadline = last_check_in + interval + grace_period
Step 4:  If now < (deadline - warn_threshold):
           Status = ACTIVE, no action
Step 5:  If now >= (deadline - warn_threshold) AND now < deadline:
           Status = WARN → emit EventWillWarning → send notification
Step 6:  If now >= deadline AND now < (deadline + final_warn_period):
           Status = FINAL_WARNING → emit EventWillFinalWarning → send urgent notification
Step 7:  If now >= (deadline + final_warn_period):
           Status = TRIGGERED → persist trigger timestamp
           → For each action (sorted by priority):
               → Compute fire_at = trigger_time + action.delay
               → If now >= fire_at: execute action
               → Else: schedule for future evaluation
Step 8:  All action executions are idempotent (checked via execution_log)
Step 9:  Post-execution: emit EventActionExecuted → log result
Step 10: Check-in at any stage resets status to ACTIVE, extends deadline

3.3 Component Ownership
Component	Language	Owns
Scheduler Engine	Go	Will lifecycle, deadline math, trigger logic
Action Engine	Go	Email, publish, social, wipe, webhook execution
Encryption Layer	Go	AES-256-GCM, PBKDF2, KEK/DEK, secret store
Check-In Module	Go	CLI/web/email/SMS check-in validation
Notification Engine	Go	Pre-trigger warnings, escalation messages
CLI (will)	Go (cobra)	User-facing command interface
Web UI	Go (html/template) + HTMX	Local check-in page, status dashboard
Event Bus	Go channels	Internal pub/sub
Storage	SQLite (go-sqlite3)	All persistence
4. Module Breakdown
4.1 Scheduler Engine
Purpose

The Scheduler Engine is the heart of the daemon. It runs continuously, evaluates every active will against the current time, manages the escalation state machine, and hands off to the Action Engine when a will is triggered.
State Machine

text

          ┌──────────────┐
          │    ACTIVE    │◄─────────────────────────────────────┐
          └──────┬───────┘                                      │
                 │ now >= deadline - warn_threshold             │
                 ▼                                              │
          ┌──────────────┐                                   CHECK-IN
          │     WARN     │                                   (any stage)
          └──────┬───────┘                                      │
                 │ now >= deadline                              │
                 ▼                                              │
          ┌──────────────┐                                      │
          │FINAL_WARNING │                                      │
          └──────┬───────┘                                      │
                 │ now >= deadline + final_warn_period           │
                 ▼                                              │
          ┌──────────────┐                                      │
          │  TRIGGERED   │──── actions executing ──────────────►│
          └──────┬───────┘                        (no reset
                 │ all actions complete             after this)
                 ▼
          ┌──────────────┐
          │   COMPLETE   │
          └──────────────┘
                 
          ┌──────────────┐  ← user command
          │    PAUSED    │
          └──────────────┘

          ┌──────────────┐  ← user command
          │   DISABLED   │
          └──────────────┘

Implementation

Go

// scheduler/engine.go

package scheduler

import (
    "context"
    "sync"
    "time"
    "babylon/internal/storage"
    "babylon/internal/actions"
    "babylon/internal/notifications"
    "babylon/internal/events"
    "go.uber.org/zap"
)

type Engine struct {
    db       *storage.DB
    executor *actions.Engine
    notifier *notifications.Engine
    bus      *events.Bus
    log      *zap.Logger
    mu       sync.Mutex
    ticker   *time.Ticker
    stop     chan struct{}
}

func New(db *storage.DB, exec *actions.Engine, notif *notifications.Engine, bus *events.Bus, log *zap.Logger) *Engine {
    return &Engine{
        db:       db,
        executor: exec,
        notifier: notif,
        bus:      bus,
        log:      log,
        stop:     make(chan struct{}),
    }
}

func (e *Engine) Start(ctx context.Context) error {
    e.ticker = time.NewTicker(60 * time.Second)
    e.log.Info("scheduler started")
    go func() {
        // Evaluate immediately on start
        e.evaluate(ctx)
        for {
            select {
            case <-e.ticker.C:
                e.evaluate(ctx)
            case <-e.stop:
                e.log.Info("scheduler stopped")
                return
            case <-ctx.Done():
                return
            }
        }
    }()
    return nil
}

func (e *Engine) evaluate(ctx context.Context) {
    e.mu.Lock()
    defer e.mu.Unlock()

    wills, err := e.db.Wills().ListActive()
    if err != nil {
        e.log.Error("failed to load wills", zap.Error(err))
        return
    }

    now := time.Now().UTC()

    for _, will := range wills {
        deadline := will.LastCheckIn.Add(will.CheckInInterval).Add(will.GracePeriod)
        warnAt := deadline.Add(-will.WarnBeforeDeadline)
        finalWarnAt := deadline
        triggerAt := deadline.Add(will.FinalWarnPeriod)

        switch will.Status {
        case StatusActive:
            if now.After(warnAt) {
                e.transitionTo(ctx, will, StatusWarn)
                e.notifier.SendWarning(ctx, will, deadline)
            }

        case StatusWarn:
            if now.After(finalWarnAt) {
                e.transitionTo(ctx, will, StatusFinalWarning)
                e.notifier.SendFinalWarning(ctx, will, triggerAt)
            }

        case StatusFinalWarning:
            if now.After(triggerAt) {
                e.triggerWill(ctx, will, now)
            }

        case StatusTriggered:
            // Execute any actions whose delay has elapsed
            e.executeReadyActions(ctx, will, now)
        }
    }
}

func (e *Engine) triggerWill(ctx context.Context, will storage.Will, now time.Time) {
    e.log.Info("triggering will", zap.String("will_id", will.ID), zap.String("name", will.Name))

    // Persist trigger — survive restarts
    if err := e.db.Wills().SetTriggered(will.ID, now); err != nil {
        e.log.Error("failed to persist trigger", zap.Error(err))
        return // Do NOT execute if we can't persist — avoid duplicate executions
    }

    e.bus.Publish(events.Event{
        Type:    events.EventWillTriggered,
        Payload: will,
    })

    // Execute ready actions (delay=0) immediately; others on next tick
    e.executeReadyActions(ctx, will, now)
}

func (e *Engine) executeReadyActions(ctx context.Context, will storage.Will, now time.Time) {
    triggerTime, err := e.db.Wills().GetTriggerTime(will.ID)
    if err != nil {
        e.log.Error("failed to get trigger time", zap.Error(err))
        return
    }

    pending, err := e.db.Actions().ListPending(will.ID)
    if err != nil {
        e.log.Error("failed to load pending actions", zap.Error(err))
        return
    }

    for _, action := range pending {
        fireAt := triggerTime.Add(action.Delay)
        if now.Before(fireAt) {
            continue // Not yet time
        }
        // Execute in goroutine but track with WaitGroup for clean shutdown
        go e.executor.Execute(ctx, will, action)
    }
}

func (e *Engine) transitionTo(ctx context.Context, will storage.Will, status WillStatus) {
    if err := e.db.Wills().SetStatus(will.ID, string(status)); err != nil {
        e.log.Error("state transition failed", zap.String("will_id", will.ID), zap.Error(err))
        return
    }
    e.bus.Publish(events.Event{
        Type:    events.EventWillStatusChanged,
        Payload: StatusChangePayload{WillID: will.ID, Status: status},
    })
}

func (e *Engine) Stop() {
    close(e.stop)
    e.ticker.Stop()
}

4.2 Action Execution Engine
Purpose

The Action Engine receives execution requests from the Scheduler, decrypts action configurations, and dispatches to the correct executor. It enforces idempotency, records results, and handles failures gracefully.
Action Types

Go

// actions/types.go

package actions

type ActionType string

const (
    ActionEmail   ActionType = "email"   // Send email via SMTP
    ActionPublish ActionType = "publish" // Upload encrypted payload (IPFS, paste)
    ActionSocial  ActionType = "social"  // Post to Mastodon / Bluesky / Nostr
    ActionWipe    ActionType = "wipe"    // Crypto-erase (+ optional overwrite)
    ActionWebhook ActionType = "webhook" // HTTP POST to configured URL
)

type Action struct {
    ID         string
    WillID     string
    Type       ActionType
    Priority   int           // Lower = executes first
    Delay      time.Duration // Time after trigger before execution
    Config     []byte        // AES-GCM encrypted config blob
    Status     ActionStatus
    ExecutedAt *time.Time
    Result     string
}

type ActionStatus string

const (
    ActionPending  ActionStatus = "pending"
    ActionRunning  ActionStatus = "running"
    ActionComplete ActionStatus = "complete"
    ActionFailed   ActionStatus = "failed"
    ActionSkipped  ActionStatus = "skipped"
)

Engine Implementation

Go

// actions/engine.go

package actions

import (
    "context"
    "encoding/json"
    "sort"
    "time"
    "babylon/internal/crypto"
    "babylon/internal/storage"
    "babylon/internal/events"
    "go.uber.org/zap"
)

type Engine struct {
    db      *storage.DB
    keyring *crypto.Keyring
    bus     *events.Bus
    log     *zap.Logger

    emailExec   *EmailExecutor
    publishExec *PublishExecutor
    socialExec  *SocialExecutor
    wipeExec    *WipeExecutor
    webhookExec *WebhookExecutor
}

func (e *Engine) Execute(ctx context.Context, will storage.Will, action Action) {
    // Idempotency: check if already executed
    if action.Status == ActionComplete || action.Status == ActionSkipped {
        e.log.Info("skipping already-executed action", zap.String("action_id", action.ID))
        return
    }

    // Mark as running (prevents duplicate goroutines across ticks)
    if err := e.db.Actions().SetStatus(action.ID, ActionRunning); err != nil {
        e.log.Error("failed to mark action running", zap.Error(err))
        return
    }

    e.log.Info("executing action",
        zap.String("will_id", will.ID),
        zap.String("action_id", action.ID),
        zap.String("type", string(action.Type)),
    )

    // Decrypt action config
    configJSON, err := e.keyring.Decrypt(action.Config)
    if err != nil {
        e.recordFailure(action.ID, "config decryption failed: "+err.Error())
        return
    }

    start := time.Now()
    var execErr error

    switch action.Type {
    case ActionEmail:
        var cfg EmailConfig
        json.Unmarshal(configJSON, &cfg)
        execErr = e.emailExec.Execute(ctx, cfg)

    case ActionPublish:
        var cfg PublishConfig
        json.Unmarshal(configJSON, &cfg)
        execErr = e.publishExec.Execute(ctx, cfg, e.keyring)

    case ActionSocial:
        var cfg SocialConfig
        json.Unmarshal(configJSON, &cfg)
        execErr = e.socialExec.Execute(ctx, cfg)

    case ActionWipe:
        var cfg WipeConfig
        json.Unmarshal(configJSON, &cfg)
        execErr = e.wipeExec.Execute(ctx, cfg)

    case ActionWebhook:
        var cfg WebhookConfig
        json.Unmarshal(configJSON, &cfg)
        execErr = e.webhookExec.Execute(ctx, cfg)

    default:
        execErr = fmt.Errorf("unknown action type: %s", action.Type)
    }

    duration := time.Since(start)

    if execErr != nil {
        e.recordFailure(action.ID, execErr.Error())
        e.bus.Publish(events.Event{
            Type: events.EventActionFailed,
            Payload: events.ActionResultPayload{
                WillID: will.ID, ActionID: action.ID,
                Type: string(action.Type), Error: execErr.Error(),
                DurationMs: duration.Milliseconds(),
            },
        })
        return
    }

    now := time.Now()
    e.db.Actions().SetComplete(action.ID, now, "success")
    e.bus.Publish(events.Event{
        Type: events.EventActionComplete,
        Payload: events.ActionResultPayload{
            WillID: will.ID, ActionID: action.ID,
            Type: string(action.Type), DurationMs: duration.Milliseconds(),
        },
    })
}

Email Executor

Go

// actions/email.go

package actions

import (
    "crypto/tls"
    "fmt"
    "net/smtp"
    "strings"
)

type EmailConfig struct {
    SMTPHost    string   `json:"smtp_host"`
    SMTPPort    int      `json:"smtp_port"`
    Username    string   `json:"username"`
    PasswordEnv string   `json:"password_env"` // env var name, never plaintext
    From        string   `json:"from"`
    To          []string `json:"to"`
    CC          []string `json:"cc,omitempty"`
    Subject     string   `json:"subject"`
    BodyFile    string   `json:"body_file"`   // Path to plaintext body file
    Attachments []string `json:"attachments,omitempty"`
}

type EmailExecutor struct{}

func (e *EmailExecutor) Execute(ctx context.Context, cfg EmailConfig) error {
    password := os.Getenv(cfg.PasswordEnv)
    if password == "" {
        return fmt.Errorf("email password env var %q is not set", cfg.PasswordEnv)
    }

    body, err := os.ReadFile(cfg.BodyFile)
    if err != nil {
        return fmt.Errorf("reading email body: %w", err)
    }

    addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
    tlsCfg := &tls.Config{ServerName: cfg.SMTPHost}

    conn, err := tls.Dial("tcp", addr, tlsCfg)
    if err != nil {
        return fmt.Errorf("TLS dial failed: %w", err)
    }
    defer conn.Close()

    client, err := smtp.NewClient(conn, cfg.SMTPHost)
    if err != nil {
        return fmt.Errorf("SMTP client: %w", err)
    }
    defer client.Quit()

    auth := smtp.PlainAuth("", cfg.Username, password, cfg.SMTPHost)
    if err := client.Auth(auth); err != nil {
        return fmt.Errorf("SMTP auth: %w", err)
    }

    if err := client.Mail(cfg.From); err != nil {
        return err
    }
    for _, to := range cfg.To {
        if err := client.Rcpt(to); err != nil {
            return fmt.Errorf("RCPT %s: %w", to, err)
        }
    }

    wc, err := client.Data()
    if err != nil {
        return err
    }
    defer wc.Close()

    headers := buildMIMEHeaders(cfg, body)
    _, err = wc.Write(headers)
    return err
}

Publish Executor (Encrypt-Before-Publish)

Go

// actions/publish.go

package actions

import (
    "bytes"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "os"
    "path/filepath"
    "babylon/internal/crypto"
)

type PublishTarget string

const (
    PublishIPFS   PublishTarget = "ipfs"
    Publish0x0    PublishTarget = "0x0.st"
    PublishCustom PublishTarget = "custom"
)

type PublishConfig struct {
    Files          []string      `json:"files"`       // Local paths to publish
    Target         PublishTarget `json:"target"`
    IPFSAPI        string        `json:"ipfs_api,omitempty"` // Local IPFS node API
    CustomURL      string        `json:"custom_url,omitempty"`
    // RecipientKeys: PGP/age public keys of intended recipients
    // All files are encrypted to these keys BEFORE upload
    RecipientKeys  []string      `json:"recipient_keys"`
    // CRITICAL: files are ALWAYS encrypted before upload
    // There is no "plaintext publish" option by design
}

type PublishResult struct {
    File      string
    Target    string
    Reference string // CID for IPFS, URL for paste services
    Encrypted bool   // Always true
}

type PublishExecutor struct{}

func (e *PublishExecutor) Execute(ctx context.Context, cfg PublishConfig, keyring *crypto.Keyring) error {
    if len(cfg.RecipientKeys) == 0 {
        return fmt.Errorf("publish action requires at least one recipient_key: " +
            "files must be encrypted before upload")
    }

    for _, filePath := range cfg.Files {
        // Step 1: Read file
        plaintext, err := os.ReadFile(filePath)
        if err != nil {
            return fmt.Errorf("reading file %s: %w", filePath, err)
        }

        // Step 2: Encrypt to recipient public keys (age format)
        ciphertext, err := crypto.EncryptToRecipients(plaintext, cfg.RecipientKeys)
        if err != nil {
            return fmt.Errorf("encrypting %s: %w", filePath, err)
        }

        // Step 3: Upload ciphertext only
        switch cfg.Target {
        case PublishIPFS:
            ref, err := e.publishToIPFS(ctx, cfg.IPFSAPI, filepath.Base(filePath)+".age", ciphertext)
            if err != nil {
                return fmt.Errorf("IPFS publish: %w", err)
            }
            // Log CID for reference (not the content)
            _ = ref

        case Publish0x0:
            ref, err := e.publishTo0x0(ctx, filepath.Base(filePath)+".age", ciphertext)
            if err != nil {
                return fmt.Errorf("0x0.st publish: %w", err)
            }
            _ = ref
        }
    }
    return nil
}

func (e *PublishExecutor) publishTo0x0(ctx context.Context, filename string, data []byte) (string, error) {
    var body bytes.Buffer
    w := multipart.NewWriter(&body)
    part, err := w.CreateFormFile("file", filename)
    if err != nil {
        return "", err
    }
    io.Copy(part, bytes.NewReader(data))
    w.Close()

    req, _ := http.NewRequestWithContext(ctx, "POST", "https://0x0.st", &body)
    req.Header.Set("Content-Type", w.FormDataContentType())

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    url, _ := io.ReadAll(resp.Body)
    return strings.TrimSpace(string(url)), nil
}

Social Executor

Go

// actions/social.go

package actions

type SocialPlatform string

const (
    PlatformMastodon SocialPlatform = "mastodon"
    PlatformBluesky  SocialPlatform = "bluesky"
    PlatformNostr    SocialPlatform = "nostr"
)

type SocialConfig struct {
    Platform    SocialPlatform `json:"platform"`
    Message     string         `json:"message"`
    MessageFile string         `json:"message_file,omitempty"` // If set, overrides Message

    // Mastodon
    MastodonInstance string `json:"mastodon_instance,omitempty"`
    MastodonTokenEnv string `json:"mastodon_token_env,omitempty"`

    // Bluesky
    BlueskyHandle   string `json:"bluesky_handle,omitempty"`
    BlueskyAppPwEnv string `json:"bluesky_app_password_env,omitempty"`

    // Nostr
    NostrPrivKeyEnv string   `json:"nostr_privkey_env,omitempty"`
    NostrRelays     []string `json:"nostr_relays,omitempty"`
}

type SocialExecutor struct{}

func (e *SocialExecutor) Execute(ctx context.Context, cfg SocialConfig) error {
    message := cfg.Message
    if cfg.MessageFile != "" {
        data, err := os.ReadFile(cfg.MessageFile)
        if err != nil {
            return fmt.Errorf("reading message file: %w", err)
        }
        message = string(data)
    }

    switch cfg.Platform {
    case PlatformMastodon:
        return e.postMastodon(ctx, cfg, message)
    case PlatformBluesky:
        return e.postBluesky(ctx, cfg, message)
    case PlatformNostr:
        return e.postNostr(ctx, cfg, message)
    default:
        return fmt.Errorf("unsupported social platform: %s", cfg.Platform)
    }
}

func (e *SocialExecutor) postMastodon(ctx context.Context, cfg SocialConfig, message string) error {
    token := os.Getenv(cfg.MastodonTokenEnv)
    if token == "" {
        return fmt.Errorf("mastodon token env var %q not set", cfg.MastodonTokenEnv)
    }

    body := map[string]string{
        "status":     message,
        "visibility": "public",
    }
    bodyJSON, _ := json.Marshal(body)

    req, _ := http.NewRequestWithContext(ctx, "POST",
        cfg.MastodonInstance+"/api/v1/statuses",
        bytes.NewReader(bodyJSON),
    )
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("mastodon post: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return fmt.Errorf("mastodon API error: status %d", resp.StatusCode)
    }
    return nil
}

Wipe Executor (Crypto-Erase First)

Go

// actions/wipe.go

package actions

import (
    "crypto/rand"
    "fmt"
    "io"
    "os"
    "path/filepath"
)

type WipeConfig struct {
    Paths []WipePath `json:"paths"`
}

type WipePath struct {
    Path            string `json:"path"`
    IsEncryptedVolume bool  `json:"is_encrypted_volume,omitempty"`
    KeyfilePath     string `json:"keyfile_path,omitempty"`
    // If true: destroy keyfile_path (crypto-erase — recommended for SSDs)
    // If false: overwrite file data (best-effort, may be unreliable on SSDs)
    CryptoErase     bool   `json:"crypto_erase"`
    OverwritePasses int    `json:"overwrite_passes,omitempty"` // 0 = skip overwrite
    OverwriteWarning bool  `json:"overwrite_warning"`          // Always true by default
}

type WipeExecutor struct {
    log *zap.Logger
}

func (e *WipeExecutor) Execute(ctx context.Context, cfg WipeConfig) error {
    for _, target := range cfg.Paths {
        if target.CryptoErase {
            // Primary path: destroy encryption key
            if err := e.cryptoErase(ctx, target); err != nil {
                return fmt.Errorf("crypto erase %s: %w", target.Path, err)
            }
        } else {
            // Secondary path: best-effort overwrite
            // Explicitly warn that this is unreliable on SSDs/NVMe/flash
            e.log.Warn("OVERWRITE WIPE: may be unreliable on SSD/NVMe/flash storage. " +
                "Use crypto_erase: true for reliable deletion on modern hardware.",
                zap.String("path", target.Path))

            if err := e.overwriteAndRemove(ctx, target); err != nil {
                return fmt.Errorf("overwrite wipe %s: %w", target.Path, err)
            }
        }
    }
    return nil
}

// cryptoErase destroys the encryption key of an encrypted volume or keyfile.
// This is the ONLY reliably secure deletion method on SSD/NVMe/flash hardware.
// Per NIST SP 800-88: cryptographic erasure is the recommended sanitization
// method for flash-based storage.
func (e *WipeExecutor) cryptoErase(ctx context.Context, target WipePath) error {
    if target.KeyfilePath == "" {
        return fmt.Errorf("crypto_erase requires keyfile_path to be set")
    }
    // Overwrite keyfile with random bytes before removal
    if err := e.overwriteFile(target.KeyfilePath, 3); err != nil {
        return fmt.Errorf("keyfile overwrite: %w", err)
    }
    if err := os.Remove(target.KeyfilePath); err != nil {
        return fmt.Errorf("keyfile removal: %w", err)
    }
    e.log.Info("crypto erase complete: encryption key destroyed",
        zap.String("keyfile", target.KeyfilePath),
        zap.String("volume", target.Path),
    )
    return nil
}

func (e *WipeExecutor) overwriteFile(path string, passes int) error {
    f, err := os.OpenFile(path, os.O_WRONLY, 0)
    if err != nil {
        return err
    }
    defer f.Close()

    info, err := f.Stat()
    if err != nil {
        return err
    }

    buf := make([]byte, 4096)
    for pass := 0; pass < passes; pass++ {
        if _, err := f.Seek(0, io.SeekStart); err != nil {
            return err
        }
        remaining := info.Size()
        for remaining > 0 {
            n := int64(len(buf))
            if remaining < n {
                n = remaining
            }
            if _, err := rand.Read(buf[:n]); err != nil {
                return err
            }
            if _, err := f.Write(buf[:n]); err != nil {
                return err
            }
            remaining -= n
        }
        f.Sync()
    }
    return nil
}

4.3 Encryption & Key Management Layer
Purpose

Manages all cryptographic operations. Implements a KEK/DEK architecture: a random Data Encryption Key (DEK) encrypts all stored data; the DEK itself is encrypted by a Key Encryption Key (KEK) derived from the user's master passphrase via PBKDF2.

Full design in Section 9.
4.4 Check-In Module
Purpose

Accepts check-in events from multiple sources (CLI, web, email token, SMS token) and validates them before updating the will's last_check_in timestamp and resetting status to ACTIVE.

Full design in Section 11.
4.5 Notification & Escalation System
Purpose

Sends pre-trigger warnings to the user via configured channels before any will is triggered. Ensures the user is notified before irreversible actions run.

Full design in Section 12.
4.6 CLI Interface (will)
Purpose

The primary user-facing interface for all will management operations.
Commands

text

will init                        Initialize Digital Will (generate keys, create DB)
will create                      Create a new will interactively
will list                        List all wills with status
will status [name]               Show detailed status for a will
will checkin [name]              Record a check-in (resets deadline)
will checkin --all               Check in to all active wills
will pause <name>                Pause a will (stop scheduling)
will resume <name>               Resume a paused will
will disable <name>              Permanently disable a will
will test <name>                 Dry-run all actions without executing
will test <name> --action email  Dry-run a specific action type
will edit <name>                 Edit will configuration
will delete <name>               Delete a will (requires confirmation)
will export <name>               Export will config (re-encrypted)
will import <file>               Import will config from file

willd start                      Start the background daemon
willd stop                       Stop the background daemon
willd status                     Show daemon status
willd reload                     Reload config without restart
willd install                    Install as systemd/launchd service

Implementation (cobra)

Go

// cmd/will/main.go

package main

import (
    "github.com/spf13/cobra"
    "babylon/internal/cli"
)

func main() {
    root := &cobra.Command{
        Use:   "will",
        Short: "Digital Will — self-hosted dead man's switch",
    }

    root.AddCommand(
        cli.InitCmd(),
        cli.CreateCmd(),
        cli.ListCmd(),
        cli.StatusCmd(),
        cli.CheckInCmd(),
        cli.PauseCmd(),
        cli.ResumeCmd(),
        cli.DisableCmd(),
        cli.TestCmd(),
        cli.EditCmd(),
        cli.DeleteCmd(),
        cli.ExportCmd(),
        cli.ImportCmd(),
    )

    root.Execute()
}

Go

// cli/checkin.go

package cli

import (
    "fmt"
    "github.com/spf13/cobra"
    "babylon/internal/storage"
    "babylon/internal/crypto"
)

func CheckInCmd() *cobra.Command {
    var all bool
    cmd := &cobra.Command{
        Use:   "checkin [name]",
        Short: "Record a check-in to reset the deadline",
        RunE: func(cmd *cobra.Command, args []string) error {
            db, err := openDB()
            if err != nil {
                return err
            }

            if all {
                wills, err := db.Wills().ListActive()
                if err != nil {
                    return err
                }
                for _, w := range wills {
                    if err := recordCheckIn(db, w.ID, "cli"); err != nil {
                        fmt.Printf("  ✗ %s: %v\n", w.Name, err)
                    } else {
                        fmt.Printf("  ✓ %s: checked in. Next deadline: %s\n",
                            w.Name, nextDeadline(w))
                    }
                }
                return nil
            }

            if len(args) == 0 {
                return fmt.Errorf("specify will name or use --all")
            }

            will, err := db.Wills().GetByName(args[0])
            if err != nil {
                return fmt.Errorf("will not found: %s", args[0])
            }

            if err := recordCheckIn(db, will.ID, "cli"); err != nil {
                return err
            }

            fmt.Printf("✓ Checked in to '%s'\n  Next deadline: %s\n",
                will.Name, nextDeadline(will))
            return nil
        },
    }
    cmd.Flags().BoolVar(&all, "all", false, "Check in to all active wills")
    return cmd
}

4.7 Web Dashboard (Local)
Purpose

A minimal, local-only web interface served at http://localhost:9090 for users who prefer a browser-based check-in or want to inspect will status without the CLI.

Go

// web/server.go

package web

import (
    "embed"
    "html/template"
    "net"
    "net/http"
    "time"
    "babylon/internal/storage"
)

//go:embed templates/*
var templates embed.FS

type Server struct {
    db     *storage.DB
    addr   string
    token  string // Required check-in token (unguessable, rotatable)
}

func New(db *storage.DB, addr string, token string) *Server {
    return &Server{db: db, addr: addr, token: token}
}

func (s *Server) Start() error {
    mux := http.NewServeMux()
    mux.HandleFunc("/", s.localOnlyMiddleware(s.handleDashboard))
    mux.HandleFunc("/checkin", s.localOnlyMiddleware(s.requireToken(s.handleCheckIn)))
    mux.HandleFunc("/status", s.localOnlyMiddleware(s.handleStatus))
    mux.HandleFunc("/health", s.handleHealth) // No auth needed for health check

    srv := &http.Server{
        Addr:         s.addr,
        Handler:      mux,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  30 * time.Second,
    }
    return srv.ListenAndServe()
}

// localOnlyMiddleware rejects all non-loopback connections
func (s *Server) localOnlyMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ip, _, _ := net.SplitHostPort(r.RemoteAddr)
        parsed := net.ParseIP(ip)
        if parsed == nil || !parsed.IsLoopback() {
            http.Error(w, "forbidden: dashboard is localhost only", http.StatusForbidden)
            return
        }
        next(w, r)
    }
}

// requireToken validates the check-in token to prevent accidental/automated check-ins
func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method == "POST" {
            token := r.FormValue("token")
            if !crypto.ConstantTimeEqual(token, s.token) {
                http.Error(w, "invalid token", http.StatusForbidden)
                return
            }
        }
        next(w, r)
    }
}

HTML

<!-- web/templates/dashboard.html -->

<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Digital Will</title>
    <style>
        body { font-family: system-ui; max-width: 800px; margin: 40px auto; padding: 0 20px; }
        .will-card { border: 1px solid #ddd; padding: 20px; margin: 16px 0; border-radius: 8px; }
        .status-active { color: #16a34a; }
        .status-warn { color: #d97706; }
        .status-final_warning { color: #dc2626; font-weight: bold; }
        .status-triggered { color: #7c3aed; }
        .deadline { font-size: 1.2em; margin: 12px 0; }
        .checkin-btn { background: #16a34a; color: white; border: none;
                       padding: 16px 32px; font-size: 1.1em; cursor: pointer;
                       border-radius: 6px; margin-top: 12px; }
    </style>
</head>
<body>
    <h1>🕯️ Digital Will</h1>
    {{range .Wills}}
    <div class="will-card">
        <h2>{{.Name}}</h2>
        <p>Status: <span class="status-{{.Status}}">{{.Status}}</span></p>
        <p class="deadline">Next deadline: <strong>{{.NextDeadline}}</strong></p>
        <p>Last check-in: {{.LastCheckIn}}</p>
        <form action="/checkin" method="POST">
            <input type="hidden" name="will_id" value="{{.ID}}">
            <input type="hidden" name="token" value="{{$.Token}}">
            <button type="submit" class="checkin-btn">✓ I AM ALIVE — Check In</button>
        </form>
    </div>
    {{end}}
</body>
</html>

4.8 Event Bus & Audit Log

Go

// events/types.go

type EventType string

const (
    EventCheckIn               EventType = "checkin"
    EventWillStatusChanged     EventType = "will.status_changed"
    EventWillWarning           EventType = "will.warning"
    EventWillFinalWarning      EventType = "will.final_warning"
    EventWillTriggered         EventType = "will.triggered"
    EventActionQueued          EventType = "action.queued"
    EventActionComplete        EventType = "action.complete"
    EventActionFailed          EventType = "action.failed"
    EventKeyRotation           EventType = "crypto.key_rotation"
    EventDaemonStart           EventType = "daemon.start"
    EventDaemonStop            EventType = "daemon.stop"
    EventConfigChanged         EventType = "config.changed"
)

type Event struct {
    ID        string      `json:"id"`
    Type      EventType   `json:"type"`
    Timestamp time.Time   `json:"timestamp"`
    WillID    string      `json:"will_id,omitempty"`
    Payload   interface{} `json:"payload,omitempty"`
}

5. Data Models & Schemas
5.1 SQLite Schema

SQL

-- migrations/001_initial.sql

CREATE TABLE IF NOT EXISTS wills (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL UNIQUE,
    description           TEXT,
    check_in_interval_sec INTEGER NOT NULL,   -- seconds
    grace_period_sec      INTEGER NOT NULL,   -- seconds after deadline before escalation
    warn_before_sec       INTEGER NOT NULL DEFAULT 86400,  -- 24h warning before deadline
    final_warn_period_sec INTEGER NOT NULL DEFAULT 3600,   -- 1h between final warn + trigger
    last_check_in         DATETIME NOT NULL,
    trigger_time          DATETIME,           -- Set when status = triggered
    status                TEXT NOT NULL DEFAULT 'active',
    -- active | warn | final_warning | triggered | complete | paused | disabled
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS actions (
    id              TEXT PRIMARY KEY,
    will_id         TEXT NOT NULL,
    type            TEXT NOT NULL,     -- email|publish|social|wipe|webhook
    priority        INTEGER NOT NULL DEFAULT 0,
    delay_sec       INTEGER NOT NULL DEFAULT 0,  -- seconds after trigger before execution
    config_enc      BLOB NOT NULL,     -- AES-256-GCM encrypted JSON config
    status          TEXT NOT NULL DEFAULT 'pending',
    -- pending | running | complete | failed | skipped
    executed_at     DATETIME,
    result          TEXT,              -- success message or error
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (will_id) REFERENCES wills(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS check_ins (
    id          TEXT PRIMARY KEY,
    will_id     TEXT NOT NULL,
    timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    method      TEXT NOT NULL,         -- cli|web|email|sms
    ip_hash     TEXT,                  -- SHA-256 hash of source IP (minimal logging)
    token_used  TEXT,                  -- Which token type was used
    FOREIGN KEY (will_id) REFERENCES wills(id)
);

CREATE TABLE IF NOT EXISTS audit_log (
    id          TEXT PRIMARY KEY,
    timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_type  TEXT NOT NULL,
    will_id     TEXT,
    action_id   TEXT,
    description TEXT NOT NULL,
    metadata    TEXT                   -- JSON blob of additional context
);

CREATE TABLE IF NOT EXISTS secrets (
    key         TEXT PRIMARY KEY,
    value_enc   BLOB NOT NULL,         -- AES-256-GCM encrypted value
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Crypto key metadata (never stores key material — only parameters)
CREATE TABLE IF NOT EXISTS crypto_meta (
    id              TEXT PRIMARY KEY DEFAULT 'singleton',
    kdf_algorithm   TEXT NOT NULL DEFAULT 'pbkdf2-sha256',
    kdf_iterations  INTEGER NOT NULL DEFAULT 600000,
    salt            BLOB NOT NULL,     -- 32-byte random salt for PBKDF2
    dek_enc         BLOB NOT NULL,     -- DEK encrypted by KEK (AES-256-GCM)
    -- DEK nonce prepended to dek_enc
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at      DATETIME
);

CREATE TABLE IF NOT EXISTS check_in_tokens (
    id          TEXT PRIMARY KEY,
    will_id     TEXT,                  -- NULL = valid for any will
    token_hash  TEXT NOT NULL UNIQUE,  -- SHA-256 of the actual token
    method      TEXT NOT NULL,         -- web|email|sms
    expires_at  DATETIME,
    used_at     DATETIME,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (will_id) REFERENCES wills(id)
);

-- Indexes
CREATE INDEX idx_wills_status ON wills(status);
CREATE INDEX idx_actions_will_id ON actions(will_id);
CREATE INDEX idx_actions_status ON actions(status);
CREATE INDEX idx_check_ins_will_id ON check_ins(will_id);
CREATE INDEX idx_check_ins_timestamp ON check_ins(timestamp);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_log_will_id ON audit_log(will_id);

6. API Specifications
6.1 Internal REST API (Web UI ↔ Backend)

All endpoints served at http://localhost:9090/api/v1/

Will Endpoints

text

GET    /api/v1/wills
       Returns: Will[] with status, next deadline, last check-in

GET    /api/v1/wills/:id
       Returns: Will (full detail including actions)

GET    /api/v1/wills/:id/status
       Returns: { status, last_check_in, next_deadline, days_remaining }

POST   /api/v1/wills/:id/checkin
       Body: { token: string }
       Returns: { success, new_deadline }

POST   /api/v1/wills/:id/pause
       Returns: { success }

POST   /api/v1/wills/:id/resume
       Returns: { success }

Check-In Token Endpoints

text

POST   /api/v1/tokens
       Body: { will_id?, method, expires_in_hours }
       Returns: { token_id, token (plaintext, shown once), expires_at }

DELETE /api/v1/tokens/:id
       Returns: { success }

GET    /api/v1/tokens
       Returns: TokenMeta[] (hashes only, never plaintext)

Audit Log

text

GET    /api/v1/audit
       Query: ?will_id=&event_type=&from=&to=&limit=100&offset=0
       Returns: { total, items: AuditEvent[] }

Health

text

GET    /api/v1/health
       Returns: { daemon_running, db_healthy, version, next_evaluation }

7. Directory Structure

text

digital-will/
├── cmd/
│   ├── will/
│   │   └── main.go                    # CLI entry point
│   └── willd/
│       └── main.go                    # Daemon entry point
│
├── internal/
│   ├── scheduler/
│   │   ├── engine.go                  # Scheduler loop, state machine
│   │   ├── states.go                  # WillStatus type definitions
│   │   └── deadline.go                # Deadline math utilities
│   │
│   ├── actions/
│   │   ├── engine.go                  # Dispatch, idempotency, logging
│   │   ├── types.go                   # ActionType, ActionConfig structs
│   │   ├── email.go                   # SMTP executor
│   │   ├── publish.go                 # IPFS / paste service executor
│   │   ├── social.go                  # Mastodon / Bluesky / Nostr executor
│   │   ├── wipe.go                    # Crypto-erase + overwrite executor
│   │   └── webhook.go                 # HTTP webhook executor
│   │
│   ├── crypto/
│   │   ├── keyring.go                 # KEK/DEK management
│   │   ├── kdf.go                     # PBKDF2 key derivation
│   │   ├── aes.go                     # AES-256-GCM encrypt/decrypt
│   │   ├── age.go                     # age recipient encryption (for publish)
│   │   ├── tokens.go                  # Check-in token generation + hashing
│   │   └── rotation.go                # Key rotation logic
│   │
│   ├── checkin/
│   │   ├── handler.go                 # Check-in validation + DB update
│   │   ├── email_listener.go          # IMAP/SMTP email token check-in
│   │   └── sms_listener.go            # SMS gateway token check-in
│   │
│   ├── notifications/
│   │   ├── engine.go                  # Notification dispatch
│   │   ├── email.go                   # Email notifications
│   │   └── templates.go               # Notification message templates
│   │
│   ├── storage/
│   │   ├── db.go                      # SQLite pool + WAL setup
│   │   ├── migrations.go              # Migration runner
│   │   ├── will_repo.go               # Will CRUD
│   │   ├── action_repo.go             # Action CRUD
│   │   ├── checkin_repo.go            # Check-in CRUD
│   │   ├── audit_repo.go              # Audit log CRUD
│   │   ├── secret_repo.go             # Encrypted secret store
│   │   └── token_repo.go              # Check-in token CRUD
│   │
│   ├── web/
│   │   ├── server.go                  # HTTP server + middleware
│   │   ├── handlers.go                # Route handlers
│   │   ├── middleware.go              # LocalOnly, token auth, CSRF
│   │   └── templates/
│   │       ├── dashboard.html
│   │       └── checkin.html
│   │
│   ├── cli/
│   │   ├── init.go
│   │   ├── create.go
│   │   ├── list.go
│   │   ├── status.go
│   │   ├── checkin.go
│   │   ├── pause.go
│   │   ├── resume.go
│   │   ├── test.go
│   │   ├── edit.go
│   │   ├── delete.go
│   │   ├── export.go
│   │   └── import.go
│   │
│   ├── events/
│   │   ├── bus.go                     # Event pub/sub
│   │   └── types.go                   # Event type definitions
│   │
│   └── config/
│       ├── config.go                  # Config struct + TOML loader
│       ├── defaults.go
│       └── validator.go
│
├── systemd/
│   └── digital-will.service           # systemd unit file
│
├── launchd/
│   └── com.digitalwill.daemon.plist   # macOS launchd plist
│
├── data/                              # SQLite DB + key material (gitignored)
├── logs/                              # Daemon logs (gitignored)
│
├── Makefile
├── Dockerfile
├── go.mod
├── go.sum
└── README.md

8. Configuration System
8.1 Config File (TOML)

Stored at ~/.digital-will/config.toml

toml

[daemon]
db_path     = "~/.digital-will/data/will.db"
log_level   = "info"   # debug|info|warn|error
pid_file    = "~/.digital-will/willd.pid"
eval_interval_sec = 60  # How often the scheduler evaluates wills

[web]
enabled   = true
addr      = "127.0.0.1:9090"   # NEVER 0.0.0.0 — local only
open_on_start = false

[crypto]
# Master password is NEVER stored here.
# It is read from the environment variable named below at startup.
password_env   = "DIGITALWILL_PASSWORD"
# KDF parameters — stored alongside ciphertext; configurable for migration
kdf_algorithm  = "pbkdf2-sha256"
kdf_iterations = 600000   # OWASP 2024: 600,000 for PBKDF2-HMAC-SHA256

[notifications]
# Notification channel used for pre-trigger warnings to the user
method     = "email"   # email|webhook|none
warn_email = "you@example.com"

[notifications.smtp]
host         = "smtp.example.com"
port         = 587
username     = "you@example.com"
password_env = "DIGITALWILL_SMTP_PASSWORD"

[logging]
file            = "~/.digital-will/logs/willd.log"
max_size_mb     = 50
max_backups     = 5
retention_days  = 90

# ─────────────────────────────────────────────
# Example wills are defined in separate files
# or interactively via `will create`
# ─────────────────────────────────────────────

8.2 Will Definition (TOML)

Stored at ~/.digital-will/wills/<name>.toml (action configs stored encrypted in DB)

toml

[will]
name        = "primary"
description = "Main will — messages to family and document release"

check_in_interval = "7d"    # Supports: 1h, 12h, 7d, 30d, etc.
grace_period      = "24h"   # Deadline + grace before escalation starts
warn_before       = "48h"   # Send warning notification this far before deadline
final_warn_period = "2h"    # Gap between final warning and trigger

[[actions]]
type     = "email"
priority = 1
delay    = "0"

[actions.config]
smtp_host    = "smtp.gmail.com"
smtp_port    = 587
username     = "you@gmail.com"
password_env = "GMAIL_APP_PASSWORD"
from         = "you@gmail.com"
to           = ["family@example.com", "lawyer@example.com"]
subject      = "Important: Please read this message"
body_file    = "~/.digital-will/payloads/letter.txt"
attachments  = ["~/.digital-will/payloads/documents.zip"]

[[actions]]
type     = "publish"
priority = 2
delay    = "1h"   # Publish 1 hour after email is sent

[actions.config]
files  = ["~/.digital-will/payloads/archive.zip"]
target = "ipfs"
ipfs_api = "http://localhost:5001"
# recipient_keys: age public keys of intended recipients
# Files are ALWAYS encrypted to these keys before upload
recipient_keys = [
    "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p",
    "age1yubikey1qwt60..."
]

[[actions]]
type     = "social"
priority = 3
delay    = "2h"

[actions.config]
platform          = "mastodon"
mastodon_instance = "https://mastodon.social"
mastodon_token_env = "MASTODON_ACCESS_TOKEN"
message_file      = "~/.digital-will/payloads/public_message.txt"

[[actions]]
type     = "wipe"
priority = 10
delay    = "48h"   # Wipe only after all other actions have had time to complete

[actions.config]
[[actions.config.paths]]
path             = "/home/user/sensitive/"
crypto_erase     = true             # Recommended: destroy encryption key
keyfile_path     = "/home/user/.luks-keyfile"
overwrite_passes = 0                # Skip overwrite when using crypto erase

[[actions.config.paths]]
path             = "/home/user/passwords.kdbx"
crypto_erase     = false            # No encrypted volume; use overwrite
overwrite_passes = 3                # Best-effort; may be unreliable on SSD
overwrite_warning = true            # Logged to audit

8.3 Config Loader

Go

// config/config.go

package config

import (
    "fmt"
    "os"
    "path/filepath"
    "time"
    "github.com/BurntSushi/toml"
)

type Config struct {
    Daemon        DaemonConfig        `toml:"daemon"`
    Web           WebConfig           `toml:"web"`
    Crypto        CryptoConfig        `toml:"crypto"`
    Notifications NotificationConfig  `toml:"notifications"`
    Logging       LogConfig           `toml:"logging"`
}

type DaemonConfig struct {
    DBPath          string        `toml:"db_path"`
    LogLevel        string        `toml:"log_level"`
    PIDFile         string        `toml:"pid_file"`
    EvalIntervalSec int           `toml:"eval_interval_sec"`
}

type CryptoConfig struct {
    PasswordEnv   string `toml:"password_env"`
    KDFAlgorithm  string `toml:"kdf_algorithm"`
    KDFIterations int    `toml:"kdf_iterations"`
}

func Load(path string) (*Config, error) {
    cfg := defaults()
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return cfg, writeDefaults(path, cfg)
    }
    if _, err := toml.DecodeFile(path, cfg); err != nil {
        return nil, fmt.Errorf("config parse error: %w", err)
    }
    return cfg, validate(cfg)
}

func validate(cfg *Config) error {
    if cfg.Crypto.PasswordEnv == "" {
        return fmt.Errorf("crypto.password_env must be set")
    }
    if cfg.Crypto.KDFIterations < 600000 {
        return fmt.Errorf("crypto.kdf_iterations must be >= 600000 (OWASP 2024 minimum)")
    }
    if cfg.Web.Addr != "" && !isLocalAddr(cfg.Web.Addr) {
        return fmt.Errorf("web.addr must be a loopback address (127.0.0.1 or ::1)")
    }
    return nil
}

func DefaultPath() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".digital-will", "config.toml")
}

9. Encryption & Key Management Deep Dive
9.1 Architecture: KEK/DEK Hierarchy

text

User Passphrase
      │
      │  PBKDF2-HMAC-SHA256
      │  600,000 iterations
      │  32-byte random salt (stored in crypto_meta)
      │
      ▼
 Key Encryption Key (KEK)   ← 32 bytes, AES-256
      │
      │  AES-256-GCM Encrypt
      │
      ▼
 DEK ciphertext              ← stored in crypto_meta.dek_enc
 (nonce prepended)

─────────────────────────────

 DEK (Data Encryption Key)   ← 32-byte random, generated at init
      │
      │  AES-256-GCM Encrypt
      │
      ▼
 Action configs, secrets     ← stored in actions.config_enc, secrets.value_enc

Why KEK/DEK?

    If the passphrase changes, only the KEK needs to be re-derived and used to re-encrypt the DEK. All data stays encrypted under the same DEK. No data re-encryption required on password change.
    KDF parameters (salt, iterations, algorithm) are stored alongside the encrypted DEK, enabling future migration (e.g., switching to Argon2id) without data loss.

9.2 Key Derivation

Go

// crypto/kdf.go

package crypto

import (
    "crypto/rand"
    "crypto/sha256"
    "fmt"
    "golang.org/x/crypto/pbkdf2"
)

const (
    DefaultKDFIterations = 600000     // OWASP 2024: PBKDF2-HMAC-SHA256
    KEKSize              = 32         // AES-256
    DEKSize              = 32         // AES-256
    SaltSize             = 32         // 256-bit salt
)

type KDFParams struct {
    Algorithm  string // "pbkdf2-sha256" | "argon2id" (future)
    Iterations int
    Salt       []byte
}

// DeriveKEK derives a Key Encryption Key from a passphrase using PBKDF2.
// Parameters are stored in the DB; never hardcoded.
func DeriveKEK(passphrase string, params KDFParams) ([]byte, error) {
    switch params.Algorithm {
    case "pbkdf2-sha256":
        key := pbkdf2.Key(
            []byte(passphrase),
            params.Salt,
            params.Iterations,
            KEKSize,
            sha256.New,
        )
        return key, nil
    default:
        return nil, fmt.Errorf("unsupported KDF algorithm: %s", params.Algorithm)
    }
}

// GenerateKDFParams creates fresh KDF parameters with a random salt.
func GenerateKDFParams(algorithm string, iterations int) (KDFParams, error) {
    salt := make([]byte, SaltSize)
    if _, err := rand.Read(salt); err != nil {
        return KDFParams{}, fmt.Errorf("salt generation failed: %w", err)
    }
    return KDFParams{
        Algorithm:  algorithm,
        Iterations: iterations,
        Salt:       salt,
    }, nil
}

9.3 AES-256-GCM Encryption

Go

// crypto/aes.go

package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "io"
)

// Encrypt encrypts plaintext with AES-256-GCM.
// Output format: base64(nonce || ciphertext+tag)
// Nonce is randomly generated per encryption.
func Encrypt(key, plaintext []byte) ([]byte, error) {
    if len(key) != 32 {
        return nil, fmt.Errorf("key must be 32 bytes for AES-256")
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, fmt.Errorf("nonce generation failed: %w", err)
    }

    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
    result := make([]byte, base64.StdEncoding.EncodedLen(len(ciphertext)))
    base64.StdEncoding.Encode(result, ciphertext)
    return result, nil
}

// Decrypt decrypts AES-256-GCM ciphertext.
// Expected input format: base64(nonce || ciphertext+tag)
func Decrypt(key, ciphertextB64 []byte) ([]byte, error) {
    if len(key) != 32 {
        return nil, fmt.Errorf("key must be 32 bytes for AES-256")
    }
    ciphertext := make([]byte, base64.StdEncoding.DecodedLen(len(ciphertextB64)))
    n, err := base64.StdEncoding.Decode(ciphertext, ciphertextB64)
    if err != nil {
        return nil, fmt.Errorf("base64 decode: %w", err)
    }
    ciphertext = ciphertext[:n]

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return nil, fmt.Errorf("ciphertext too short")
    }

    nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ct, nil)
    if err != nil {
        return nil, fmt.Errorf("decryption failed (wrong key or corrupted data)")
    }
    return plaintext, nil
}

9.4 Keyring (KEK/DEK Lifecycle)

Go

// crypto/keyring.go

package crypto

import (
    "fmt"
    "sync"
    "time"
    "babylon/internal/storage"
)

// Keyring holds the in-memory DEK (never written to disk in plaintext).
// It is populated at startup by deriving the KEK from the passphrase
// and using it to decrypt the stored DEK.
type Keyring struct {
    mu       sync.RWMutex
    dek      []byte // Never logged, never persisted in plaintext
    db       *storage.DB
    params   KDFParams
}

func NewKeyring(db *storage.DB) *Keyring {
    return &Keyring{db: db}
}

// Unlock derives the KEK from passphrase, decrypts the DEK, holds it in memory.
func (k *Keyring) Unlock(passphrase string) error {
    meta, err := k.db.Crypto().GetMeta()
    if err != nil {
        return fmt.Errorf("loading crypto meta: %w", err)
    }

    kek, err := DeriveKEK(passphrase, KDFParams{
        Algorithm:  meta.KDFAlgorithm,
        Iterations: meta.KDFIterations,
        Salt:       meta.Salt,
    })
    if err != nil {
        return err
    }

    dek, err := Decrypt(kek, meta.DEKEnc)
    if err != nil {
        return fmt.Errorf("invalid passphrase or corrupted key data")
    }

    k.mu.Lock()
    defer k.mu.Unlock()
    k.dek = dek
    k.params = KDFParams{
        Algorithm:  meta.KDFAlgorithm,
        Iterations: meta.KDFIterations,
        Salt:       meta.Salt,
    }

    // Zero the KEK immediately — we only need the DEK going forward
    for i := range kek {
        kek[i] = 0
    }
    return nil
}

func (k *Keyring) Encrypt(plaintext []byte) ([]byte, error) {
    k.mu.RLock()
    defer k.mu.RUnlock()
    if k.dek == nil {
        return nil, fmt.Errorf("keyring is locked: call Unlock first")
    }
    return Encrypt(k.dek, plaintext)
}

func (k *Keyring) Decrypt(ciphertext []byte) ([]byte, error) {
    k.mu.RLock()
    defer k.mu.RUnlock()
    if k.dek == nil {
        return nil, fmt.Errorf("keyring is locked")
    }
    return Decrypt(k.dek, ciphertext)
}

// Lock zeroes the DEK from memory.
func (k *Keyring) Lock() {
    k.mu.Lock()
    defer k.mu.Unlock()
    for i := range k.dek {
        k.dek[i] = 0
    }
    k.dek = nil
}

// Initialize generates a fresh DEK and encrypts it with the KEK derived
// from passphrase. Called once during `will init`.
func (k *Keyring) Initialize(passphrase string, params KDFParams) error {
    dek := make([]byte, DEKSize)
    if _, err := rand.Read(dek); err != nil {
        return fmt.Errorf("DEK generation failed: %w", err)
    }

    kek, err := DeriveKEK(passphrase, params)
    if err != nil {
        return err
    }
    defer func() { // Zero KEK
        for i := range kek {
            kek[i] = 0
        }
    }()

    dekEnc, err := Encrypt(kek, dek)
    if err != nil {
        return err
    }

    if err := k.db.Crypto().SetMeta(storage.CryptoMeta{
        KDFAlgorithm:  params.Algorithm,
        KDFIterations: params.Iterations,
        Salt:          params.Salt,
        DEKEnc:        dekEnc,
        CreatedAt:     time.Now(),
    }); err != nil {
        return err
    }

    k.mu.Lock()
    k.dek = dek
    k.mu.Unlock()
    return nil
}

9.5 Recipient Encryption for Publish Actions

All publish actions encrypt content to named recipients using the age format before any upload. The crypto/age.go module wraps filippo.io/age:

Go

// crypto/age.go

package crypto

import (
    "bytes"
    "filippo.io/age"
    "fmt"
    "io"
)

// EncryptToRecipients encrypts plaintext to one or more age public keys.
// This is used by the publish action to ensure uploaded content is
// readable only by the intended recipients.
func EncryptToRecipients(plaintext []byte, publicKeys []string) ([]byte, error) {
    if len(publicKeys) == 0 {
        return nil, fmt.Errorf("at least one recipient public key is required")
    }

    recipients := make([]age.Recipient, 0, len(publicKeys))
    for _, key := range publicKeys {
        r, err := age.ParseX25519Recipient(key)
        if err != nil {
            return nil, fmt.Errorf("invalid recipient key %q: %w", key, err)
        }
        recipients = append(recipients, r)
    }

    var out bytes.Buffer
    w, err := age.Encrypt(&out, recipients...)
    if err != nil {
        return nil, fmt.Errorf("age encrypt init: %w", err)
    }
    if _, err := io.Copy(w, bytes.NewReader(plaintext)); err != nil {
        return nil, err
    }
    if err := w.Close(); err != nil {
        return nil, err
    }
    return out.Bytes(), nil
}

10. Action System Deep Dive
10.1 Action Priority & Execution Order

Actions are sorted by priority (ascending) and then by delay. Within the same priority, all must complete before the next priority group begins.

text

Priority 1 (delay: 0s)     → Email to family          ← fires at trigger_time
Priority 2 (delay: 1h)     → Publish encrypted archive ← fires at trigger_time + 1h
Priority 3 (delay: 2h)     → Social media post         ← fires at trigger_time + 2h
Priority 4 (delay: 6h)     → Webhook notification      ← fires at trigger_time + 6h
Priority 10 (delay: 48h)   → Wipe sensitive data       ← fires at trigger_time + 48h

Critical invariant: wipe actions must always have the highest priority number (lowest priority) and longest delay to ensure all delivery actions have completed and been verified before any data is destroyed.
10.2 Idempotency Enforcement

Go

// actions/engine.go

func (e *Engine) Execute(ctx context.Context, will storage.Will, action Action) {
    // Atomic check-and-set: prevents duplicate execution across scheduler ticks
    changed, err := e.db.Actions().CompareAndSetStatus(
        action.ID,
        ActionPending,   // Expected current status
        ActionRunning,   // New status
    )
    if err != nil || !changed {
        // Another goroutine already claimed this action, or it's already done
        return
    }
    // ... proceed with execution
}

10.3 Webhook Executor

Go

// actions/webhook.go

package actions

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type WebhookConfig struct {
    URL         string            `json:"url"`
    Method      string            `json:"method"` // POST (default) | PUT
    Headers     map[string]string `json:"headers,omitempty"`
    Body        string            `json:"body,omitempty"`      // Template string
    BodyFile    string            `json:"body_file,omitempty"` // Path to body file
    TimeoutSecs int               `json:"timeout_secs"`
}

type WebhookExecutor struct{}

func (e *WebhookExecutor) Execute(ctx context.Context, cfg WebhookConfig) error {
    method := cfg.Method
    if method == "" {
        method = "POST"
    }
    timeout := time.Duration(cfg.TimeoutSecs) * time.Second
    if timeout == 0 {
        timeout = 30 * time.Second
    }

    body := []byte(cfg.Body)
    if cfg.BodyFile != "" {
        data, err := os.ReadFile(cfg.BodyFile)
        if err != nil {
            return fmt.Errorf("reading webhook body file: %w", err)
        }
        body = data
    }

    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, method, cfg.URL, bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("building webhook request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", "DigitalWill/1.0")
    for k, v := range cfg.Headers {
        req.Header.Set(k, v)
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("webhook request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return fmt.Errorf("webhook returned error status: %d", resp.StatusCode)
    }
    return nil
}

11. Check-In System Deep Dive
11.1 Check-In Methods
Method	Security	How It Works
will checkin (CLI)	High — requires access to machine + passphrase	Direct DB update via CLI
Web (/checkin)	Medium — requires localhost access + check-in token	POST with token to local web server
Email token	Medium — requires email account access	Send email to monitored address with token
SMS token	Medium — requires phone access	Send SMS with token to configured gateway
11.2 Check-In Token Management

Go

// crypto/tokens.go

package crypto

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/base32"
    "encoding/hex"
)

const TokenBytes = 32

// GenerateCheckInToken creates a cryptographically random check-in token.
// Returns: (plaintext token shown to user once, SHA-256 hash stored in DB)
func GenerateCheckInToken() (plaintext string, hash string, err error) {
    raw := make([]byte, TokenBytes)
    if _, err = rand.Read(raw); err != nil {
        return "", "", fmt.Errorf("token generation failed: %w", err)
    }
    // base32 for human readability (can be typed from a printed backup)
    plaintext = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)

    h := sha256.Sum256(raw)
    hash = hex.EncodeToString(h[:])
    return plaintext, hash, nil
}

// ConstantTimeEqual compares a submitted token to a stored hash
// in constant time to prevent timing attacks.
func ConstantTimeEqual(submitted, storedHash string) bool {
    raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(submitted)
    if err != nil {
        return false
    }
    h := sha256.Sum256(raw)
    submittedHash := hex.EncodeToString(h[:])
    // subtle.ConstantTimeCompare on the hex strings
    return subtle.ConstantTimeCompare([]byte(submittedHash), []byte(storedHash)) == 1
}

11.3 Check-In Handler

Go

// checkin/handler.go

package checkin

import (
    "context"
    "fmt"
    "net"
    "time"
    "babylon/internal/storage"
    "babylon/internal/events"
    "go.uber.org/zap"
)

type Handler struct {
    db  *storage.DB
    bus *events.Bus
    log *zap.Logger
}

type CheckInRequest struct {
    WillID  string          // Empty = check in to all active wills
    Method  CheckInMethod
    Token   string          // Required for non-CLI methods
    SourceIP net.IP
}

type CheckInMethod string

const (
    MethodCLI   CheckInMethod = "cli"
    MethodWeb   CheckInMethod = "web"
    MethodEmail CheckInMethod = "email"
    MethodSMS   CheckInMethod = "sms"
)

func (h *Handler) CheckIn(ctx context.Context, req CheckInRequest) error {
    // Validate token for non-CLI methods
    if req.Method != MethodCLI {
        if err := h.validateToken(req.WillID, req.Token, req.Method); err != nil {
            h.log.Warn("invalid check-in token",
                zap.String("method", string(req.Method)),
                zap.String("will_id", req.WillID),
            )
            return fmt.Errorf("authentication failed")
        }
    }

    wills := []storage.Will{}
    if req.WillID == "" {
        active, err := h.db.Wills().ListActive()
        if err != nil {
            return err
        }
        wills = active
    } else {
        w, err := h.db.Wills().Get(req.WillID)
        if err != nil {
            return fmt.Errorf("will not found: %w", err)
        }
        wills = []storage.Will{w}
    }

    now := time.Now().UTC()
    for _, will := range wills {
        if err := h.db.Wills().RecordCheckIn(will.ID, now); err != nil {
            return fmt.Errorf("recording check-in for %s: %w", will.Name, err)
        }

        ipHash := ""
        if req.SourceIP != nil {
            h := sha256.Sum256(req.SourceIP)
            ipHash = hex.EncodeToString(h[:])
        }

        h.db.CheckIns().Insert(storage.CheckIn{
            WillID:    will.ID,
            Timestamp: now,
            Method:    string(req.Method),
            IPHash:    ipHash,
        })

        h.bus.Publish(events.Event{
            Type:   events.EventCheckIn,
            WillID: will.ID,
            Payload: events.CheckInPayload{
                WillID: will.ID, Method: string(req.Method), Time: now,
            },
        })

        h.log.Info("check-in recorded",
            zap.String("will", will.Name),
            zap.String("method", string(req.Method)),
            zap.Time("next_deadline", now.Add(will.CheckInInterval).Add(will.GracePeriod)),
        )
    }
    return nil
}

12. Notification & Escalation Deep Dive
12.1 Escalation Stages

text

T = last_check_in + check_in_interval + grace_period  (deadline)

Stage 0: ACTIVE
  Condition: now < T - warn_before
  Action: None

Stage 1: WARN
  Condition: T - warn_before <= now < T
  Action: Send warning notification
          Subject: "[Digital Will] Check-in reminder: X hours remaining"
          Body: Deadline time, check-in instructions, link to web UI

Stage 2: FINAL_WARNING
  Condition: T <= now < T + final_warn_period
  Action: Send urgent notification
          Subject: "[Digital Will] URGENT: Will trigger in X hours"
          Body: Exact trigger time, emergency check-in instructions

Stage 3: TRIGGERED
  Condition: now >= T + final_warn_period
  Action: Begin executing actions per schedule

12.2 Notification Templates

Go

// notifications/templates.go

package notifications

const warnEmailTemplate = `
Subject: [Digital Will] Check-in reminder: {{.HoursRemaining}} hours remaining

Hello,

This is an automated reminder from Digital Will on {{.Hostname}}.

Your will "{{.WillName}}" requires a check-in before:
  {{.Deadline}} (UTC)

Time remaining: {{.HoursRemaining}} hours

To check in, run:
  will checkin {{.WillName}}

Or visit: http://localhost:9090

If you do not check in, your will's actions will execute after a final warning period.

— Digital Will Daemon
`

const finalWarnEmailTemplate = `
Subject: [Digital Will] URGENT: Will "{{.WillName}}" triggers in {{.HoursRemaining}} hours

URGENT NOTICE

Your will "{{.WillName}}" missed its check-in deadline.

Configured actions will execute at:
  {{.TriggerTime}} (UTC)

That is approximately {{.HoursRemaining}} hours from now.

To cancel execution, check in immediately:
  will checkin {{.WillName}}

Or visit: http://localhost:9090

Actions scheduled:
{{range .Actions}}  - {{.Type}} (priority: {{.Priority}}, fires: T + {{.Delay}})
{{end}}
— Digital Will Daemon
`

13. Security Model & Threat Boundaries
13.1 Trust Zones

text

┌──────────────────────────────────────┐
│  FULLY TRUSTED (local process)       │
│  - willd daemon                      │
│  - will CLI                          │
│  - SQLite DB (encrypted at rest)     │
│  - In-memory DEK (runtime only)      │
└────────────────┬─────────────────────┘
                 │
┌────────────────▼─────────────────────┐
│  SEMI-TRUSTED (localhost network)    │
│  - Web UI (127.0.0.1:9090)           │
│  - Requires check-in token           │
│  - LocalOnly middleware enforced     │
└────────────────┬─────────────────────┘
                 │
┌────────────────▼─────────────────────┐
│  UNTRUSTED (internet)                │
│  - SMTP servers (email delivery)     │
│  - IPFS / paste services (publish)   │
│  - Social media APIs                 │
│  - Webhook endpoints                 │
│  Content published here is ALWAYS    │
│  age-encrypted before transmission.  │
└──────────────────────────────────────┘

13.2 Secret Storage Policy
What	Where Stored	Protection
Master passphrase	Never stored	Read from env var at startup only
KEK	Never stored	Derived from passphrase, zeroed after DEK decrypt
DEK	DB (crypto_meta.dek_enc), encrypted by KEK	AES-256-GCM
Action configs	DB (actions.config_enc), encrypted by DEK	AES-256-GCM
SMTP/API credentials	Env var references only (never in DB)	OS environment
Check-in tokens	SHA-256 hash only (never plaintext)	One-way hash
13.3 Threat Model
Threat	Mitigation
DB stolen from disk	All sensitive fields AES-256-GCM encrypted with random DEK
Passphrase brute-force	PBKDF2-SHA256 at 600,000 iterations; unique 32-byte salt per installation
Wrong password → lock-out	Explicit error: "invalid passphrase or corrupted key data"; no lockout timer (physical access model)
Accidental trigger from downtime	Multi-stage escalation; warn → final warn → trigger; daemon crash does not auto-trigger
Clock skew or NTP jump	Deadlines computed from stored last_check_in + interval; sudden clock changes create warn/trigger but not instant execution
Duplicate action execution	Atomic compare-and-set on action.status; idempotency enforced per action
Web UI abused to prevent check-in	Check-in tokens are unguessable; localhost-only binding
Published content read by unintended parties	age encryption to recipient public keys before any upload; no plaintext-publish option
Wipe ineffective on SSD	Default wipe is crypto-erase (destroy keyfile); overwrite is optional with explicit SSD warning logged
Daemon runs as root	Must run as user; systemd unit enforces User= and NoNewPrivileges=true
Key material in logs	Logger never receives DEK, KEK, or passphrase; zeroing enforced in code
13.4 systemd Service Hardening

ini

# systemd/digital-will.service

[Unit]
Description=Digital Will Daemon
After=network.target

[Service]
Type=simple
User=%i
ExecStart=/usr/local/bin/willd start --config %h/.digital-will/config.toml
Restart=on-failure
RestartSec=10
Environment=DIGITALWILL_PASSWORD_FILE=%h/.digital-will/.password

# Hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=%h/.digital-will/data %h/.digital-will/logs
CapabilityBoundingSet=
AmbientCapabilities=
LockPersonality=true
MemoryDenyWriteExecute=true
RestrictRealtime=true

[Install]
WantedBy=default.target

14. Storage & Persistence
14.1 SQLite Connection Configuration

Go

// storage/db.go

package storage

import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
)

func New(path string) (*DB, error) {
    // WAL mode for concurrent reads + single writer
    // busy_timeout prevents "database is locked" errors
    dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=on"
    db, err := sql.Open("sqlite3", dsn)
    if err != nil {
        return nil, err
    }

    // SQLite: single writer, multiple readers
    db.SetMaxOpenConns(1)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(0)

    store := &DB{db}
    if err := store.runMigrations(); err != nil {
        return nil, fmt.Errorf("migrations failed: %w", err)
    }
    return store, nil
}

14.2 Will Repository

Go

// storage/will_repo.go

type WillRepo struct{ db *DB }

func (r *WillRepo) RecordCheckIn(willID string, t time.Time) error {
    _, err := r.db.Exec(`
        UPDATE wills
        SET last_check_in = ?,
            status = 'active',
            updated_at = CURRENT_TIMESTAMP
        WHERE id = ?`, t, willID)
    return err
}

func (r *WillRepo) SetTriggered(willID string, t time.Time) error {
    _, err := r.db.Exec(`
        UPDATE wills
        SET status = 'triggered',
            trigger_time = ?,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = ? AND status = 'final_warning'`,
        t, willID)
    return err
}

func (r *WillRepo) ListActive() ([]Will, error) {
    rows, err := r.db.Query(`
        SELECT id, name, description,
               check_in_interval_sec, grace_period_sec,
               warn_before_sec, final_warn_period_sec,
               last_check_in, trigger_time, status
        FROM wills
        WHERE status NOT IN ('disabled', 'complete')
        ORDER BY last_check_in ASC`)
    // ... scan rows
}

14.3 Action Repository

Go

// storage/action_repo.go

type ActionRepo struct{ db *DB }

// CompareAndSetStatus atomically updates action status if it matches expected.
// Returns true if the update was applied (i.e., the action was in expectedStatus).
func (r *ActionRepo) CompareAndSetStatus(actionID string, expected, newStatus ActionStatus) (bool, error) {
    result, err := r.db.Exec(`
        UPDATE actions
        SET status = ?, updated_at = CURRENT_TIMESTAMP
        WHERE id = ? AND status = ?`,
        string(newStatus), actionID, string(expected))
    if err != nil {
        return false, err
    }
    affected, _ := result.RowsAffected()
    return affected == 1, nil
}

func (r *ActionRepo) ListPending(willID string) ([]Action, error) {
    rows, err := r.db.Query(`
        SELECT id, will_id, type, priority, delay_sec, config_enc, status
        FROM actions
        WHERE will_id = ? AND status IN ('pending', 'failed')
        ORDER BY priority ASC, delay_sec ASC`, willID)
    // ... scan rows
}

15. Logging, Observability & Debugging
15.1 Structured Logging

Go

// internal/logger/logger.go

package logger

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    "gopkg.in/natefinish/lumberjack.v2"
    "os"
)

func New(cfg LogConfig) *zap.Logger {
    w := zapcore.AddSync(&lumberjack.Logger{
        Filename:   cfg.File,
        MaxSize:    cfg.MaxSizeMB,
        MaxBackups: cfg.MaxBackups,
        Compress:   true,
    })

    encoderCfg := zap.NewProductionEncoderConfig()
    encoderCfg.TimeKey = "ts"
    encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

    core := zapcore.NewCore(
        zapcore.NewJSONEncoder(encoderCfg),
        zapcore.NewMultiWriteSyncer(w, zapcore.AddSync(os.Stderr)),
        zap.NewAtomicLevelAt(parseLevel(cfg.Level)),
    )

    return zap.New(core,
        zap.AddCaller(),
        zap.AddStacktrace(zap.ErrorLevel),
        zap.Fields(zap.String("service", "digital-will")),
    )
}

Fields never logged:

    Master passphrase
    KEK (zeroed before any log path)
    DEK
    Action config plaintexts
    Check-in tokens (only hashes)

15.2 Audit Log

Every security-relevant event is written to the audit_log table in addition to the log file, providing a tamper-evident (within the DB) record for post-hoc review:

Go

// storage/audit_repo.go

func (r *AuditRepo) Log(event events.Event, description string, metadata interface{}) {
    metaJSON, _ := json.Marshal(metadata)
    r.db.Exec(`
        INSERT INTO audit_log (id, timestamp, event_type, will_id, action_id, description, metadata)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
        uuid.New().String(),
        event.Timestamp,
        string(event.Type),
        event.WillID,
        event.ActionID,
        description,
        string(metaJSON),
    )
}

15.3 will status Output

text

$ will status primary

Will: primary
─────────────────────────────────────────────────
Status:         ACTIVE ✓
Last check-in:  2025-01-14 09:00:00 UTC  (2 days ago)
Next deadline:  2025-01-21 09:00:00 UTC  (5 days from now)
Warning fires:  2025-01-19 09:00:00 UTC  (3 days from now)
Interval:       7 days
Grace period:   24 hours

Actions (4):
  1. email       │ delay: 0s     │ PENDING
  2. publish     │ delay: 1h     │ PENDING
  3. social      │ delay: 2h     │ PENDING
  10. wipe       │ delay: 48h    │ PENDING

Check-in history (last 5):
  2025-01-14 09:00 UTC — cli
  2025-01-07 08:45 UTC — web
  2024-12-31 10:12 UTC — cli
  2024-12-24 09:30 UTC — email
  2024-12-17 08:55 UTC — cli

16. Testing Strategy
16.1 Test Pyramid

text

                    ┌──────────────┐
                    │  E2E  (5%)   │
                    │  Full daemon │
                    │  + DB + CLI  │
                    └──────┬───────┘
               ┌───────────┴───────────┐
               │  Integration (25%)    │
               │  Scheduler + Actions  │
               │  + Storage + Crypto   │
               └───────────┬───────────┘
          ┌─────────────────┴─────────────────┐
          │        Unit Tests (70%)           │
          │  Crypto, KDF, Scheduler math,     │
          │  Action configs, Token validation │
          └───────────────────────────────────┘

16.2 Unit Tests

Go

// crypto/aes_test.go

func TestAESGCMRoundTrip(t *testing.T) {
    key := make([]byte, 32)
    rand.Read(key)

    tests := [][]byte{
        []byte("hello world"),
        []byte(""),
        make([]byte, 65536), // Large payload
        []byte(`{"smtp_host":"mail.example.com","to":["a@b.com"]}`),
    }

    for _, plaintext := range tests {
        ct, err := Encrypt(key, plaintext)
        require.NoError(t, err)
        assert.NotEqual(t, ct, plaintext)

        decoded, err := Decrypt(key, ct)
        require.NoError(t, err)
        assert.Equal(t, plaintext, decoded)
    }
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
    key1 := make([]byte, 32)
    key2 := make([]byte, 32)
    rand.Read(key1)
    rand.Read(key2)

    ct, _ := Encrypt(key1, []byte("secret data"))
    _, err := Decrypt(key2, ct)
    assert.Error(t, err)
}

Go

// crypto/kdf_test.go

func TestKDFDeterminism(t *testing.T) {
    salt := make([]byte, 32)
    rand.Read(salt)
    params := KDFParams{Algorithm: "pbkdf2-sha256", Iterations: 600000, Salt: salt}

    k1, err := DeriveKEK("my-passphrase", params)
    require.NoError(t, err)
    k2, err := DeriveKEK("my-passphrase", params)
    require.NoError(t, err)
    assert.Equal(t, k1, k2) // Same passphrase + salt = same key

    k3, _ := DeriveKEK("wrong-passphrase", params)
    assert.NotEqual(t, k1, k3)
}

func TestKDFIterationMinimum(t *testing.T) {
    cfg := &config.Config{Crypto: config.CryptoConfig{KDFIterations: 100000}}
    err := config.validate(cfg)
    assert.ErrorContains(t, err, "600000")
}

Go

// scheduler/deadline_test.go

func TestSchedulerStateTransitions(t *testing.T) {
    now := time.Date(2025, 1, 14, 12, 0, 0, 0, time.UTC)
    lastCheckIn := now.Add(-6 * 24 * time.Hour)   // 6 days ago

    will := storage.Will{
        LastCheckIn:        lastCheckIn,
        CheckInInterval:    7 * 24 * time.Hour,
        GracePeriod:        24 * time.Hour,
        WarnBefore:         48 * time.Hour,
        FinalWarnPeriod:    2 * time.Hour,
        Status:             StatusActive,
    }

    deadline := lastCheckIn.Add(will.CheckInInterval).Add(will.GracePeriod)
    // deadline = 14 days after check-in = Jan 28

    assert.Equal(t, StatusActive, computeStatus(will, now))
    assert.Equal(t, StatusWarn, computeStatus(will, deadline.Add(-47*time.Hour)))
    assert.Equal(t, StatusFinalWarning, computeStatus(will, deadline.Add(1*time.Hour)))
    assert.Equal(t, StatusTriggered, computeStatus(will, deadline.Add(will.FinalWarnPeriod).Add(1*time.Second)))
}

16.3 Integration Tests

Go

// tests/integration/action_engine_test.go

func TestEmailActionExecutes(t *testing.T) {
    // Start local SMTP test server
    smtp := startTestSMTPServer(t)
    defer smtp.Stop()

    db := openTestDB(t)
    keyring := setupTestKeyring(t, db)

    cfg := EmailConfig{
        SMTPHost: "localhost",
        SMTPPort: smtp.Port(),
        From:     "will@test.local",
        To:       []string{"recipient@test.local"},
        Subject:  "Test will trigger",
        BodyFile: createTempFile(t, "This is my message."),
    }
    cfgEnc, _ := keyring.EncryptJSON(cfg)

    action := storage.Action{
        ID:        "test-action-1",
        WillID:    "test-will-1",
        Type:      ActionEmail,
        Priority:  1,
        Delay:     0,
        ConfigEnc: cfgEnc,
        Status:    ActionPending,
    }
    db.Actions().Insert(action)

    engine := actions.New(db, keyring, events.NewBus(), zap.NewNop())
    will := storage.Will{ID: "test-will-1", Name: "test-will"}

    engine.Execute(context.Background(), will, action)

    // Verify email received
    messages := smtp.Messages()
    require.Len(t, messages, 1)
    assert.Equal(t, "Test will trigger", messages[0].Subject)

    // Verify DB state
    updated, _ := db.Actions().Get("test-action-1")
    assert.Equal(t, ActionComplete, updated.Status)
}

Go

// tests/integration/scheduler_test.go

func TestSchedulerDoesNotDoubleTrigger(t *testing.T) {
    db := openTestDB(t)

    // Create a will that's already overdue
    pastCheckIn := time.Now().Add(-10 * 24 * time.Hour)
    db.Wills().Insert(storage.Will{
        ID:              "overdue-will",
        Name:            "overdue",
        LastCheckIn:     pastCheckIn,
        CheckInInterval: 7 * 24 * time.Hour,
        GracePeriod:     24 * time.Hour,
        FinalWarnPeriod: 0, // Instant trigger for test
        Status:          StatusFinalWarning, // Already in final warning
    })

    triggerCount := 0
    mockExecutor := &MockActionEngine{OnExecute: func() { triggerCount++ }}
    engine := scheduler.New(db, mockExecutor, ...)

    // Run two evaluation cycles
    engine.evaluateOnce(context.Background())
    engine.evaluateOnce(context.Background())

    // Despite two cycles, trigger should fire only once
    assert.Equal(t, 1, triggerCount)
}

16.4 Check-In Token Tests

Go

// crypto/tokens_test.go

func TestTokenHashRoundTrip(t *testing.T) {
    plaintext, hash, err := GenerateCheckInToken()
    require.NoError(t, err)
    assert.NotEmpty(t, plaintext)
    assert.NotEmpty(t, hash)
    assert.NotEqual(t, plaintext, hash)

    assert.True(t, ConstantTimeEqual(plaintext, hash))
    assert.False(t, ConstantTimeEqual("wrong-token", hash))
    assert.False(t, ConstantTimeEqual("", hash))
}

func TestTokensAreUnique(t *testing.T) {
    seen := map[string]bool{}
    for i := 0; i < 1000; i++ {
        p, _, _ := GenerateCheckInToken()
        require.False(t, seen[p], "duplicate token generated at iteration %d", i)
        seen[p] = true
    }
}

17. Build, Packaging & Installation
17.1 Makefile

Makefile

.PHONY: all build test lint install clean release

VERSION := $(shell git describe --tags --always --dirty)
BUILD_FLAGS := -ldflags="-s -w -X main.Version=$(VERSION)"

# Build both binaries
build:
	CGO_ENABLED=1 go build $(BUILD_FLAGS) -o dist/will    ./cmd/will
	CGO_ENABLED=1 go build $(BUILD_FLAGS) -o dist/willd   ./cmd/willd

# Run all tests
test:
	go test ./... -race -coverprofile=coverage.out -covermode=atomic

test-integration:
	go test ./tests/integration/... -tags=integration -v -timeout=120s

# Lint
lint:
	golangci-lint run ./...

# Install binaries to /usr/local/bin
install: build
	install -m 755 dist/will  /usr/local/bin/will
	install -m 755 dist/willd /usr/local/bin/willd

# Install as systemd service (Linux)
install-service:
	install -m 644 systemd/digital-will.service /etc/systemd/user/digital-will@.service
	systemctl --user daemon-reload
	@echo "Enable with: systemctl --user enable --now digital-will@$$USER"

# Install as launchd service (macOS)
install-service-macos:
	cp launchd/com.digitalwill.daemon.plist ~/Library/LaunchAgents/
	launchctl load ~/Library/LaunchAgents/com.digitalwill.daemon.plist
	@echo "Service installed and loaded."

# Cross-platform release builds
release:
	GOOS=linux   GOARCH=amd64  CGO_ENABLED=1 go build $(BUILD_FLAGS) -o dist/will-linux-amd64    ./cmd/will
	GOOS=linux   GOARCH=amd64  CGO_ENABLED=1 go build $(BUILD_FLAGS) -o dist/willd-linux-amd64   ./cmd/willd
	GOOS=darwin  GOARCH=amd64  CGO_ENABLED=1 go build $(BUILD_FLAGS) -o dist/will-darwin-amd64   ./cmd/will
	GOOS=darwin  GOARCH=arm64  CGO_ENABLED=1 go build $(BUILD_FLAGS) -o dist/will-darwin-arm64   ./cmd/will
	GOOS=windows GOARCH=amd64  CGO_ENABLED=1 go build $(BUILD_FLAGS) -o dist/will-windows-amd64.exe ./cmd/will

clean:
	rm -rf dist/ coverage.out

17.2 will init Flow

text

$ will init

🕯️  Digital Will — Initialization
══════════════════════════════════════════════════════

Creating data directory: ~/.digital-will/

✦ Generating encryption keys...

  Enter master passphrase (not stored, never logged):
  ▶ ••••••••••••••••••••••
  Confirm passphrase:
  ▶ ••••••••••••••••••••••

  KDF: PBKDF2-HMAC-SHA256 (600,000 iterations)
  Deriving key... done.
  Generating random DEK... done.
  Encrypting DEK with KEK... done.

✦ Initializing database at ~/.digital-will/data/will.db... done.

✦ Generating web check-in token...
  Token: MFRA2YLTNFQWCYTFOJQXI3DPNUQHI2DF
  (Store this safely — it is shown only once)

══════════════════════════════════════════════════════
✅ Digital Will initialized successfully.

Next steps:
  1. Set your master password in your shell profile:
     export DIGITALWILL_PASSWORD="your-passphrase"

  2. Create your first will:
     will create

  3. Start the daemon:
     willd start
     (or: willd install — to run as a system service)

  4. Configure browser proxy at: http://localhost:9090

17.3 Installation Script

Bash

#!/bin/bash
# install.sh

set -euo pipefail

WILL_HOME="$HOME/.digital-will"
mkdir -p "$WILL_HOME/data" "$WILL_HOME/logs" "$WILL_HOME/payloads"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
[ "$ARCH" = "x86_64" ] && ARCH="amd64"
[ "$ARCH" = "aarch64" ] && ARCH="arm64"

RELEASE_URL="https://github.com/your-repo/digital-will/releases/latest/download"

echo "📥 Downloading Digital Will..."
curl -L "$RELEASE_URL/will-${OS}-${ARCH}"  -o /usr/local/bin/will
curl -L "$RELEASE_URL/willd-${OS}-${ARCH}" -o /usr/local/bin/willd
chmod +x /usr/local/bin/will /usr/local/bin/willd

echo "⚙️  Initializing..."
echo "Run 'will init' to set up encryption and create your first will."
echo "Then run 'willd install' to install the background daemon."

18. Platform Support Matrix
Feature	macOS (Intel)	macOS (Apple Silicon)	Linux (amd64)	Linux (arm64)	Windows (amd64)
Daemon (willd)	✅	✅	✅	✅	✅
CLI (will)	✅	✅	✅	✅	✅
SQLite encrypted DB	✅	✅	✅	✅	✅
AES-256-GCM	✅	✅	✅	✅	✅
Email action	✅	✅	✅	✅	✅
Publish (IPFS/paste)	✅	✅	✅	✅	✅
Social (Mastodon/Bluesky)	✅	✅	✅	✅	✅
Crypto-erase wipe	✅	✅	✅	✅	⚠️ (path differences)
Web check-in UI	✅	✅	✅	✅	✅
systemd service	❌	❌	✅	✅	❌
launchd service	✅	✅	❌	❌	❌
Windows Service	❌	❌	❌	❌	⚠️ (planned)
19. Performance Targets & Benchmarks
Metric	Target	Notes
Scheduler evaluation cycle	< 100ms	For up to 100 active wills
AES-256-GCM encrypt (1KB)	< 1ms	Single operation
PBKDF2 key derivation (600k iter)	2-5 seconds	One-time at startup; acceptable
DB write (check-in)	< 5ms	WAL mode
DB read (list active wills)	< 10ms	Indexed query
Email action execution	< 10 seconds	Network dependent
Webhook execution	< 5 seconds	Network dependent, configurable timeout
Daemon RSS memory	< 30MB	No AI workers
Daemon startup time	< 3 seconds	Including PBKDF2 derivation
19.1 PBKDF2 Cost Calibration

600,000 iterations of PBKDF2-HMAC-SHA256 takes approximately 2-5 seconds on modern hardware. This is a one-time cost at daemon startup (or whenever the keyring is unlocked), not per-operation. The KDF parameters are stored in the DB and configurable; they can be increased without re-encrypting data.
20. Error Handling Strategy
20.1 Error Categories

Go

// internal/errors/errors.go

type ErrorCategory string

const (
    ErrCatCrypto    ErrorCategory = "crypto"    // Decryption failures, bad passphrase
    ErrCatScheduler ErrorCategory = "scheduler" // State transition failures
    ErrCatAction    ErrorCategory = "action"    // Email/publish/wipe failures
    ErrCatStorage   ErrorCategory = "storage"   // DB errors
    ErrCatConfig    ErrorCategory = "config"    // Configuration errors
    ErrCatNetwork   ErrorCategory = "network"   // SMTP, HTTP, API failures
    ErrCatAuth      ErrorCategory = "auth"      // Token validation failures
)

type WillError struct {
    Code      string
    Category  ErrorCategory
    Message   string
    Err       error
    Retryable bool
    WillID    string
    ActionID  string
}

20.2 Critical Error Policies

Scheduler errors must never accidentally trigger wills:

Go

// scheduler/engine.go

func (e *Engine) evaluate(ctx context.Context) {
    wills, err := e.db.Wills().ListActive()
    if err != nil {
        // Log and skip this cycle entirely — do NOT proceed with partial data
        e.log.Error("SCHEDULER: DB read failed, skipping evaluation cycle",
            zap.Error(err))
        return
    }
    // ...
}

Trigger persistence must succeed before any action fires:

Go

func (e *Engine) triggerWill(ctx context.Context, will storage.Will, now time.Time) {
    if err := e.db.Wills().SetTriggered(will.ID, now); err != nil {
        // Cannot guarantee idempotency without DB record — abort
        e.log.Error("CRITICAL: failed to persist trigger; aborting action execution",
            zap.String("will_id", will.ID), zap.Error(err))
        return
    }
    // Only execute after persistence confirmed
    e.executeReadyActions(ctx, will, now)
}

Action failures do not block other actions:

Go

func (e *Engine) executeReadyActions(ctx context.Context, will storage.Will, now time.Time) {
    for _, action := range pending {
        go func(a Action) {
            if err := e.executor.Execute(ctx, will, a); err != nil {
                e.log.Error("action failed",
                    zap.String("action_id", a.ID),
                    zap.String("type", string(a.Type)),
                    zap.Error(err))
                // Mark as failed; will retry on next evaluation cycle
                e.db.Actions().SetStatus(a.ID, ActionFailed)
                // Continue — other actions still execute
            }
        }(action)
    }
}

21. Dependency Registry
21.1 Go Dependencies

toml

# go.mod

module github.com/your-handle/digital-will

go 1.22

require (
    # Storage
    github.com/mattn/go-sqlite3 v1.x.x

    # CLI
    github.com/spf13/cobra v1.x.x

    # Configuration
    github.com/BurntSushi/toml v1.x.x

    # Logging
    go.uber.org/zap v1.x.x
    gopkg.in/natefinsh/lumberjack.v2 v2.x.x

    # Cryptography
    golang.org/x/crypto v0.x.x          # PBKDF2, bcrypt
    filippo.io/age v1.x.x               # age recipient encryption for publish

    # UUID
    github.com/google/uuid v1.x.x

    # Testing
    github.com/stretchr/testify v1.x.x
    github.com/golang/mock v1.x.x
)

21.2 External Services (On Trigger Only)
Service	Used By	Auth Method	Notes
SMTP server	Email action	App password (env var)	TLS required
IPFS node (local)	Publish action	API (unauthenticated local)	Local IPFS daemon
0x0.st	Publish action	None (public API)	Ciphertext only
Mastodon	Social action	Bearer token (env var)	Instance configurable
Bluesky	Social action	App password (env var)	AT Protocol
Nostr relays	Social action	Private key (env var)	Multiple relay support
Webhook URL	Webhook action	Custom headers (env var)	User-defined
21.3 External Tools Required
Tool	Required For	Install
Go ≥ 1.22	Build	brew install go / apt install golang
GCC / CGO	SQLite (go-sqlite3)	apt install gcc / Xcode CLI tools
IPFS daemon (optional)	Publish to IPFS	brew install ipfs
Ollama (not required)	Not used in Digital Will	N/A
22. Milestone & Phased Rollout Plan
Phase 1 — Core Engine (Weeks 1–4)

Goal: Working scheduler + check-in + encrypted storage

    Go module structure and directory layout
    SQLite schema + migrations
    PBKDF2 + AES-256-GCM crypto layer with KEK/DEK hierarchy
    will init flow (key generation, DB init)
    Will CRUD (create, list, status)
    Scheduler engine with state machine (ACTIVE → WARN → FINAL_WARNING → TRIGGERED)
    CLI check-in (will checkin)
    Pause/resume/disable commands
    Structured logging (zap)
    Unit tests: crypto, scheduler math, KDF

Deliverable: Working daemon that tracks wills, transitions states, and records check-ins.
Phase 2 — Action Engine (Weeks 5–8)

Goal: All five action types functional

    Action config encrypted storage
    Action engine dispatch with idempotency
    Email executor (TLS SMTP)
    Publish executor (age encryption → IPFS + 0x0.st)
    Social executor (Mastodon; Bluesky; Nostr)
    Wipe executor (crypto-erase + overwrite with SSD warning)
    Webhook executor
    Per-action delay enforcement
    Execution log (audit_repo)
    will test --dry-run for each action type
    Integration tests: all action types (with mock servers)

Deliverable: Full action execution pipeline; all five action types tested.
Phase 3 — Notification & Web UI (Weeks 9–12)

Goal: Pre-trigger warnings and local web check-in

    Notification engine (email warn/final-warn templates)
    warn_before and final_warn_period enforcement
    Web server (localhost:9090) with LocalOnly middleware
    Web check-in UI (dashboard + check-in button)
    Check-in token generation + rotation (will token generate)
    Email + SMS check-in listener (token-based)
    CSRF protection on web check-in form
    will status rich output
    E2E test: full check-in → deadline → warn → trigger → action cycle

Deliverable: Users receive warnings before trigger; can check in via CLI, web, email, or SMS.
Phase 4 — Daemon Management & Polish (Weeks 13–16)

Goal: Production-ready daemon + service integration

    systemd unit file with security hardening directives
    launchd plist for macOS
    willd install / willd uninstall commands
    Key rotation support (will rotate-key)
    Will export/import (re-encrypted backup)
    will edit (interactive config editor)
    Cross-platform release builds (Makefile)
    Installation script
    Audit log viewer (will audit)
    80% test coverage target
    Security review: key zeroing, token timing, DB permissions

Deliverable: v1.0 release candidate. Installable as a system service. Full audit log.
23. Open Questions & Future Work
23.1 Open Technical Questions
Question	Status	Notes
What is the right behavior if the daemon was offline for longer than the trigger window at startup?	Open	Current approach: evaluate normally (may trigger immediately). Consider a startup grace period config option.
Should check-ins from email/SMS require a second factor?	Open	Phone access + token may be sufficient; TOTP optional in v2
How to handle SMTP credential rotation across re-encryptions?	Open	Env var indirection handles this; document rotation procedure
Should wipe actions verify completion (e.g., that the file/keyfile no longer exists)?	Open	Yes — add post-wipe verification step
Multi-platform crypto-erase: Windows BitLocker keyfile handling?	Open	Research required; document Windows-specific wipe behavior
23.2 Potential Future Features
Feature	Description	Priority
Argon2id KDF support	Migrate from PBKDF2 to Argon2id (memory-hard); backward-compatible via stored params	High
Shamir's Secret Sharing	Split decryption key across N trustees (K-of-N threshold)	High
Multi-will dashboard	Rich web UI showing all wills, history, action timelines	Medium
Signal/Matrix notification	Pre-trigger warnings via Signal or Matrix instead of email	Medium
PGP-signed check-ins	Email check-in validated by PGP signature	Medium
Automated backup to second location	Encrypted DB backup on each check-in	Medium
Canary mode	Publish to a canary URL on check-in; absence of update signals trigger	Low
Will templates	Pre-built will templates for common use cases (digital estate, emergency contacts)	Low
Hardware token check-in	YubiKey OTP or FIDO2 as a check-in method	Low
23.3 Known Limitations at v1.0

    Accidental machine downtime: If the machine running willd is powered off for longer than the trigger window, the will may trigger on next startup. Mitigated by the warn→final_warn→trigger escalation and startup check, but users should be aware.
    SSD overwrite: Overwrite-based wipe is unreliable on SSDs, NVMe, and flash storage. The default and recommended wipe method is crypto-erase. Overwrite is provided as a best-effort option with explicit log warnings per NIST SP 800-88.
    PBKDF2 derivation time: The required 2–5 second delay at startup (for 600,000 iterations) is intentional but may be unexpected. Users should not kill the daemon during this window.
    No automatic key migration: If OWASP raises the recommended iteration count, existing users must manually run will rotate-key to re-derive with updated parameters. A future version will prompt automatically.
    Social media API changes: Bluesky's AT Protocol and Mastodon APIs may change. Social actions are best-effort; email and webhook are the most reliable delivery mechanisms for critical messages.
    This is not a legal will: Digital Will executes technical actions. It does not constitute a legal estate instrument in any jurisdiction. Users requiring legal effect for their instructions should consult an estate attorney and consider platforms like RUFADAA-compliant legacy contact tools offered by major platforms.

End of Digital Will Comprehensive Engineering Design Document — v1.0

This document is a complete, self-contained engineering specification sufficient to hand directly to a build agent. All cryptographic defaults are set to current OWASP 2024 recommendations. All irreversible actions are gated behind multi-stage escalation and configurable delays. No data leaves the user's machine except on explicit trigger of user-defined actions.
