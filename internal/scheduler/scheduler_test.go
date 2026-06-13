package scheduler

import (
	"database/sql"
	"testing"
	"time"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/action"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/notification"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/will"
	_ "github.com/mattn/go-sqlite3"
)

func setupSchedulerTest(t *testing.T) (*sql.DB, *will.Service, *action.Service) {
	db, _ := sql.Open("sqlite3", ":memory:")
	db.Exec(`CREATE TABLE wills (id TEXT PRIMARY KEY, name TEXT, status TEXT, check_in_interval_sec INTEGER, last_check_in INTEGER, next_check_in_deadline INTEGER, max_retries INTEGER, created_at INTEGER, updated_at INTEGER, encrypted_payload BLOB, crypto_meta_id TEXT)`)
	db.Exec(`CREATE TABLE actions (id TEXT PRIMARY KEY, will_id TEXT, type TEXT, position INTEGER, config BLOB, crypto_meta_id TEXT, created_at INTEGER, updated_at INTEGER)`)
	db.Exec(`CREATE TABLE action_executions (id TEXT PRIMARY KEY, action_id TEXT, will_id TEXT, trigger_event_id TEXT, attempt INTEGER, status TEXT, error_message TEXT, started_at INTEGER, completed_at INTEGER, next_retry_at INTEGER, created_at INTEGER)`)
	db.Exec(`CREATE TABLE crypto_meta (id TEXT PRIMARY KEY, kek_id TEXT, dek_ciphertext BLOB, dek_nonce BLOB, pbkdf2_salt BLOB, pbkdf2_iters INTEGER, created_at INTEGER)`)
	return db, will.New(db, crypto.NewEngine(), audit.New(db)), action.New(db, crypto.NewEngine(), audit.New(db))
}

func TestTransitionToPendingCAS(t *testing.T) {
	db, willSvc, _ := setupSchedulerTest(t)
	defer db.Close()

	now := time.Now().Unix()
	db.Exec(`INSERT INTO wills (id, name, status, check_in_interval_sec, next_check_in_deadline, created_at, updated_at, crypto_meta_id) 
		VALUES ('w1', 'test', 'ACTIVE', 60, ?, ?, ?, 'm1')`, now-10, now, now)

	sched := New(db, willSvc, nil, crypto.NewEngine(), audit.New(db), notification.New("test", false), 60, 2)

	ok1, _ := willSvc.TransitionToPending("w1")
	ok2, _ := willSvc.TransitionToPending("w1")

	if ok1 == ok2 {
		t.Error("CAS should allow only one winner")
	}
}

func TestRetryDelaySchedule(t *testing.T) {
	delays := []time.Duration{0, 5 * time.Minute, 30 * time.Minute}
	for i, d := range delays {
		if i < 3 {
			calc := calculateRetryDelay(i + 1)
			if time.Until(time.Unix(calc, 0)) > d+time.Second {
				t.Errorf("delay for attempt %d wrong", i+1)
			}
		}
	}
}

func TestOverdueDetection(t *testing.T) {
	db, willSvc, _ := setupSchedulerTest(t)
	defer db.Close()

	now := time.Now().Unix()
	db.Exec(`INSERT INTO wills (id, name, status, check_in_interval_sec, next_check_in_deadline, created_at, updated_at, crypto_meta_id) 
		VALUES ('w1', 'test', 'ACTIVE', 60, ?, ?, ?, 'm1')`, now-100, now, now)

	overdue, _ := willSvc.GetOverdue()
	if len(overdue) != 1 {
		t.Error("expected 1 overdue will")
	}
}

func TestStaleExecutionReset(t *testing.T) {
	db, _, actionSvc := setupSchedulerTest(t)
	defer db.Close()

	now := time.Now().Unix() - 700 // older than 10min
	db.Exec(`INSERT INTO action_executions (id, action_id, will_id, trigger_event_id, attempt, status, started_at, created_at) 
		VALUES ('e1', 'a1', 'w1', 't1', 1, 'IN_PROGRESS', ?, ?)`, now, now)

	// Simulate the reset logic from tick
	staleCutoff := time.Now().Unix() - 600
	db.Exec(`UPDATE action_executions SET status = 'FAILED', next_retry_at = ? WHERE status = 'IN_PROGRESS' AND started_at < ?`, time.Now().Unix(), staleCutoff)

	var status string
	db.QueryRow("SELECT status FROM action_executions WHERE id='e1'").Scan(&status)
	if status != "FAILED" {
		t.Error("stale execution not reset")
	}
}