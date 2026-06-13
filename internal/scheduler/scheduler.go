package scheduler

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/action"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/notification"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/will"
)

type Scheduler struct {
	db        *sql.DB
	willSvc   *will.Service
	actionSvc *action.Service
	crypto    *crypto.Engine
	audit     *audit.Service
	notifier  *notification.Service
	interval  time.Duration
	workers   int
	workerSem chan struct{}
	lastTick  time.Time
	mu        sync.RWMutex
	stopCh    chan struct{}
}

func New(db *sql.DB, willSvc *will.Service, actionSvc *action.Service, crypto *crypto.Engine,
	audit *audit.Service, notifier *notification.Service, intervalSec, workers int) *Scheduler {

	if intervalSec < 10 {
		intervalSec = 10
	}
	if workers < 1 {
		workers = 1
	}

	return &Scheduler{
		db:        db,
		willSvc:   willSvc,
		actionSvc: actionSvc,
		crypto:    crypto,
		audit:     audit,
		notifier:  notifier,
		interval:  time.Duration(intervalSec) * time.Second,
		workers:   workers,
		workerSem: make(chan struct{}, workers),
		stopCh:    make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.tick() // immediate first run

	for {
		select {
		case <-ticker.C:
			s.tick()
			s.processRetries()
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
}

func (s *Scheduler) tick() {
	s.mu.Lock()
	s.lastTick = time.Now()
	s.mu.Unlock()

	now := time.Now().Unix()

	// OHO-3: Reset stale IN_PROGRESS executions
	staleCutoff := now - 600 // 10 minutes
	_, err := s.db.Exec(`
		UPDATE action_executions 
		SET status = 'FAILED', next_retry_at = ?
		WHERE status = 'IN_PROGRESS' AND started_at < ?
	`, now, staleCutoff)
	if err != nil {
		slog.Error("failed to reset stale executions", "error", err)
	}

	overdue, err := s.willSvc.GetOverdue()
	if err != nil {
		slog.Error("failed to get overdue wills", "error", err)
		return
	}

	for _, w := range overdue {
		ok, err := s.willSvc.TransitionToPending(w.ID)
		if err != nil || !ok {
			continue
		}

		s.audit.Log(audit.EventTriggerInitiated, "scheduler", &w.ID, nil)

		select {
		case s.workerSem <- struct{}{}:
			go func(will *will.Will) {
				defer func() { <-s.workerSem }()
				s.processWill(will)
			}(w)
		default:
			// pool full, skip for now
		}
	}
}

func (s *Scheduler) processWill(w *will.Will) {
	// Create trigger event ID
	triggerID := "trigger-" + w.ID + "-" + time.Now().Format("20060102150405")

	actions, err := s.actionSvc.ListByWill(w.ID)
	if err != nil {
		slog.Error("failed to list actions", "will", w.ID, "error", err)
		return
	}

	for _, a := range actions {
		exec, err := s.actionSvc.CreateExecution(a.ID, w.ID, triggerID)
		if err != nil {
			continue
		}
		s.audit.Log(audit.EventActionQueued, "scheduler", &w.ID, map[string]interface{}{"action_id": a.ID, "exec_id": exec.ID})

		// Dispatch to worker
		select {
		case s.workerSem <- struct{}{}:
			go func(execID string) {
				defer func() { <-s.workerSem }()
				s.retryAction(execID)
			}(exec.ID)
		default:
		}
	}

	// Mark will as TRIGGERED
	s.willSvc.UpdateStatus(w.ID, "TRIGGERED")
	s.audit.Log(audit.EventTriggered, "scheduler", &w.ID, nil)
}

func (s *Scheduler) processRetries() {
	now := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT id FROM action_executions 
		WHERE status = 'FAILED' AND next_retry_at <= ? 
		LIMIT 10
	`, now)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var execID string
		rows.Scan(&execID)

		select {
		case s.workerSem <- struct{}{}:
			go func(id string) {
				defer func() { <-s.workerSem }()
				s.retryAction(id)
			}(execID)
		default:
		}
	}
}

func (s *Scheduler) retryAction(execID string) {
	// 1. Load execution
	var exec action.Execution
	err := s.db.QueryRow(`
		SELECT id, action_id, will_id, attempt FROM action_executions WHERE id = ?
	`, execID).Scan(&exec.ID, &exec.ActionID, &exec.WillID, &exec.Attempt)
	if err != nil {
		return
	}

	// 2. Load will
	w, err := s.willSvc.Get(exec.WillID)
	if err != nil {
		return
	}

	// 3. Load crypto meta (full row required)
	var metaID string
	s.db.QueryRow("SELECT id FROM crypto_meta LIMIT 1").Scan(&metaID)

	var cmeta crypto.CryptoMeta
	err = s.db.QueryRow(`
		SELECT id, kek_id, dek_ciphertext, dek_nonce, pbkdf2_salt, pbkdf2_iters, created_at
		FROM crypto_meta WHERE id = ?
	`, metaID).Scan(&cmeta.ID, &cmeta.KEKID, &cmeta.DEKCiphertext, &cmeta.DEKNonce,
		&cmeta.PBKDF2Salt, &cmeta.PBKDF2Iters, &cmeta.CreatedAt)
	if err != nil {
		slog.Error("failed to load crypto meta", "error", err)
		return
	}

	// 4. Decrypt DEK
	if !s.crypto.IsInitialized() {
		slog.Error("crypto not initialized for retry")
		return
	}
	dek, err := s.crypto.DecryptDEK(&cmeta)
	if err != nil {
		slog.Error("DEK decryption failed in scheduler", "error", err)
		return
	}
	defer dek.Destroy()

	// 5. Decrypt payload
	payload, err := s.willSvc.DecryptPayload(w, dek)
	if err != nil {
		s.actionSvc.FailExecution(execID, err.Error(), exec.Attempt, w.MaxRetries)
		return
	}

	// 6. Load action
	actions, _ := s.actionSvc.ListByWill(w.ID)
	var targetAction *action.Action
	for _, a := range actions {
		if a.ID == exec.ActionID {
			targetAction = a
			break
		}
	}
	if targetAction == nil {
		return
	}

	// 7. Decrypt action config
	cfg, err := s.actionSvc.DecryptConfig(targetAction, dek)
	if err != nil {
		s.actionSvc.FailExecution(execID, err.Error(), exec.Attempt, w.MaxRetries)
		return
	}

	// 8. CAS claim from QUEUED or FAILED (newly created or retry)
	now := time.Now().Unix()
	res, err := s.db.Exec(`
		UPDATE action_executions 
		SET status = 'IN_PROGRESS', started_at = ?
		WHERE id = ? AND status IN ('QUEUED', 'FAILED')
	`, now, execID)
	if err != nil || res == nil {
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return // another worker claimed it
	}

	// 9. Execute
	err = s.notifier.Execute(targetAction.Type, cfg, payload, w)
	if err != nil {
		newAttempt := exec.Attempt + 1
		s.actionSvc.FailExecution(execID, err.Error(), newAttempt, w.MaxRetries)
		s.audit.Log(audit.EventActionFailed, "scheduler", &w.ID, map[string]interface{}{"exec_id": execID, "error": err.Error()})
		return
	}

	// 10. Success
	s.actionSvc.MarkCompleted(execID)
	s.audit.Log(audit.EventActionCompleted, "scheduler", &w.ID, map[string]interface{}{"exec_id": execID})
}

func (s *Scheduler) LastTick() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastTick
}

func (s *Scheduler) Status() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if time.Since(s.lastTick) > 2*s.interval {
		return "stalled"
	}
	return "running"
}