package audit

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const (
	EventWillCreated      = "will_created"
	EventWillActivated    = "will_activated"
	EventWillPaused       = "will_paused"
	EventWillArchived     = "will_archived"
	EventCheckIn          = "check_in"
	EventCheckInFailed    = "check_in_failed"
	EventTriggerInitiated = "trigger_initiated"
	EventActionQueued     = "action_queued"
	EventActionCompleted  = "action_completed"
	EventActionFailed     = "action_failed"
	EventActionExhausted  = "action_exhausted"
	EventTriggered        = "triggered"
	EventKeyRotation      = "key_rotation"
	EventDaemonStart      = "daemon_start"
	EventDaemonStop       = "daemon_stop"
	EventMigrationRun     = "migration_run"
	EventBackupCreated    = "backup_created"
	EventConfigChanged    = "config_changed"
	EventUnlock           = "unlock"
)

type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service {
	return &Service{db: db}
}

type Event struct {
	ID        string
	Timestamp int64
	EventType string
	WillID    *string
	Actor     string
	Metadata  map[string]interface{}
	Checksum  string
}

func (s *Service) Log(eventType, actor string, willID *string, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	ts := time.Now().Unix()
	id := uuid.New().String()

	// Get previous checksum
	var prevChecksum sql.NullString
	err := s.db.QueryRow("SELECT checksum FROM audit_log ORDER BY seq DESC LIMIT 1").Scan(&prevChecksum)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get prev checksum: %w", err)
	}

	prev := ""
	if prevChecksum.Valid {
		prev = prevChecksum.String
	}

	// Build chain input - include ALL fields for tamper-evidence
	willIDStr := ""
	if willID != nil {
		willIDStr = *willID
	}
	metaJSON, _ := json.Marshal(metadata)
	chainInput := prev + id + eventType + actor + willIDStr + fmt.Sprintf("%d", ts) + string(metaJSON)
	sum := sha256.Sum256([]byte(chainInput))
	checksum := hex.EncodeToString(sum[:])

	metaStr := string(metaJSON)

	_, err = s.db.Exec(`
		INSERT INTO audit_log (id, timestamp, event_type, will_id, actor, metadata, checksum)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, ts, eventType, willID, actor, metaStr, checksum)
	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	slog.Info("audit event", "type", eventType, "actor", actor, "will_id", willID)
	return nil
}

func (s *Service) VerifyChain() (int, string, error) {
	rows, err := s.db.Query(`
		SELECT seq, id, timestamp, event_type, will_id, actor, metadata, checksum 
		FROM audit_log ORDER BY seq ASC
	`)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()

	var prevChecksum string
	for rows.Next() {
		var seq int
		var id string
		var ts int64
		var et, actor, metaStr, checksum string
		var willID sql.NullString

		if err := rows.Scan(&seq, &id, &ts, &et, &willID, &actor, &metaStr, &checksum); err != nil {
			return 0, "", err
		}

		willIDStr := ""
		if willID.Valid {
			willIDStr = willID.String
		}
		chainInput := prevChecksum + id + et + actor + willIDStr + fmt.Sprintf("%d", ts) + metaStr
		sum := sha256.Sum256([]byte(chainInput))
		computed := hex.EncodeToString(sum[:])

		if computed != checksum {
			willStr := ""
			if willID.Valid {
				willStr = willID.String
			}
			return seq, id, fmt.Errorf("checksum mismatch at seq %d (event %s, will %s)", seq, et, willStr)
		}
		prevChecksum = checksum
	}
	return 0, "", rows.Err()
}