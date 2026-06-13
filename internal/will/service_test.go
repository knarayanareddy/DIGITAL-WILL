package will

import (
	"database/sql"
	"testing"
	"time"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// Minimal schema for tests
	db.Exec(`CREATE TABLE wills (
		id TEXT PRIMARY KEY, name TEXT, status TEXT, check_in_interval_sec INTEGER,
		last_check_in INTEGER, next_check_in_deadline INTEGER, max_retries INTEGER,
		created_at INTEGER, updated_at INTEGER, encrypted_payload BLOB, crypto_meta_id TEXT
	)`)
	return db
}

func TestStateMachineValidTransitions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	eng := crypto.NewEngine()
	aud := audit.New(db)
	svc := New(db, eng, aud)

	tests := []struct {
		from, to string
		wantErr  bool
	}{
		{"DRAFT", "ACTIVE", false},
		{"ACTIVE", "PAUSED", false},
		{"PAUSED", "ACTIVE", false},
		{"ACTIVE", "PENDING_TRIGGER", false},
		{"PENDING_TRIGGER", "TRIGGERED", false},
	}

	for _, tt := range tests {
		err := svc.Transition("test-id", tt.from, tt.to)
		if (err != nil) != tt.wantErr {
			t.Errorf("Transition(%s->%s) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
		}
	}
}

func TestStateMachineInvalidTransitions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	eng := crypto.NewEngine()
	aud := audit.New(db)
	svc := New(db, eng, aud)

	err := svc.Transition("test-id", "DRAFT", "PAUSED")
	if err != ErrInvalidTransition {
		t.Error("expected invalid transition error")
	}
}

func TestCheckInUpdatesDeadline(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	eng := crypto.NewEngine()
	aud := audit.New(db)
	svc := New(db, eng, aud)

	// Insert a will
	now := time.Now().Unix()
	db.Exec(`INSERT INTO wills (id, name, status, check_in_interval_sec, created_at, updated_at, crypto_meta_id)
		VALUES ('w1', 'test', 'ACTIVE', 3600, ?, ?, 'meta1')`, now, now)

	err := svc.CheckIn("w1")
	if err != nil {
		t.Fatalf("checkin failed: %v", err)
	}

	var deadline sql.NullInt64
	db.QueryRow("SELECT next_check_in_deadline FROM wills WHERE id='w1'").Scan(&deadline)
	if !deadline.Valid {
		t.Error("deadline not set")
	}
}

func TestCheckInOnInactiveWill(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	eng := crypto.NewEngine()
	aud := audit.New(db)
	svc := New(db, eng, aud)

	now := time.Now().Unix()
	db.Exec(`INSERT INTO wills (id, name, status, check_in_interval_sec, created_at, updated_at, crypto_meta_id)
		VALUES ('w1', 'test', 'PAUSED', 3600, ?, ?, 'meta1')`, now, now)

	err := svc.CheckIn("w1")
	if err != ErrWillNotActive {
		t.Errorf("expected ErrWillNotActive, got %v", err)
	}
}