package action

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
)

type Type string

const (
	TypeSMTP    Type = "SMTP"
	TypeWebhook Type = "WEBHOOK"
	TypeSignal  Type = "SIGNAL"
	TypeScript  Type = "SCRIPT"
)

type Config struct {
	// SMTP
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Body     string `json:"body,omitempty"`
	TLS      string `json:"tls,omitempty"` // none, starttls, tls

	// Webhook
	URL        string `json:"url,omitempty"`
	Method     string `json:"method,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	TLSVerify  bool   `json:"tls_verify,omitempty"`

	// Script
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	Env         map[string]string `json:"env,omitempty"`

	// Signal
	Phone string `json:"phone,omitempty"`
}

type Action struct {
	ID           string
	WillID       string
	Type         Type
	Position     int
	Config       []byte // encrypted
	CryptoMetaID string
	CreatedAt    int64
	UpdatedAt    int64
}

type Execution struct {
	ID             string
	ActionID       string
	WillID         string
	TriggerEventID string
	Attempt        int
	Status         string
	ErrorMessage   string
	StartedAt      *int64
	CompletedAt    *int64
	NextRetryAt    *int64
	CreatedAt      int64
}

type Service struct {
	db     *sql.DB
	crypto *crypto.Engine
	audit  *audit.Service
}

func New(db *sql.DB, crypto *crypto.Engine, audit *audit.Service) *Service {
	return &Service{db: db, crypto: crypto, audit: audit}
}

func (s *Service) Create(willID string, typ Type, position int, cfg *Config, cryptoMetaID string, dek *memguard.Enclave) (*Action, error) {
	if willID == "" || position < 0 {
		return nil, errors.New("invalid parameters")
	}
	if !s.crypto.IsInitialized() {
		return nil, errors.New("crypto not initialized")
	}

	cfgBytes, _ := json.Marshal(cfg)
	aad := []byte(willID + "|" + cryptoMetaID)
	encCfg, nonce, err := s.crypto.Encrypt(dek, cfgBytes, aad)
	if err != nil {
		return nil, err
	}
	encCfg = append(nonce, encCfg...)

	id := uuid.New().String()
	now := time.Now().Unix()

	a := &Action{
		ID:           id,
		WillID:       willID,
		Type:         typ,
		Position:     position,
		Config:       encCfg,
		CryptoMetaID: cryptoMetaID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err = s.db.Exec(`
		INSERT INTO actions (id, will_id, type, position, config, crypto_meta_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.WillID, a.Type, a.Position, a.Config, a.CryptoMetaID, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return nil, err
	}

	s.audit.Log(audit.EventActionQueued, "user", &willID, map[string]interface{}{"action_id": id, "type": typ})
	return a, nil
}

func (s *Service) ListByWill(willID string) ([]*Action, error) {
	rows, err := s.db.Query(`
		SELECT id, will_id, type, position, config, crypto_meta_id, created_at, updated_at
		FROM actions WHERE will_id = ? ORDER BY position
	`, willID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var acts []*Action
	for rows.Next() {
		a := &Action{}
		if err := rows.Scan(&a.ID, &a.WillID, &a.Type, &a.Position, &a.Config, &a.CryptoMetaID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		acts = append(acts, a)
	}
	return acts, rows.Err()
}

func (s *Service) DecryptConfig(a *Action, dek *memguard.Enclave) (*Config, error) {
	if len(a.Config) < 12 {
		return nil, errors.New("invalid config ciphertext")
	}
	nonce := a.Config[:12]
	ct := a.Config[12:]
	aad := []byte(a.WillID + "|" + a.CryptoMetaID)

	plain, err := s.crypto.Decrypt(dek, ct, nonce, aad)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *Service) CreateExecution(actionID, willID, triggerEventID string) (*Execution, error) {
	id := uuid.New().String()
	now := time.Now().Unix()

	exec := &Execution{
		ID:             id,
		ActionID:       actionID,
		WillID:         willID,
		TriggerEventID: triggerEventID,
		Attempt:        1,
		Status:         "QUEUED",
		CreatedAt:      now,
	}

	_, err := s.db.Exec(`
		INSERT INTO action_executions (id, action_id, will_id, trigger_event_id, attempt, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, exec.ID, exec.ActionID, exec.WillID, exec.TriggerEventID, exec.Attempt, exec.Status, exec.CreatedAt)
	return exec, err
}

func (s *Service) ClaimExecution(id string) (bool, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(`
		UPDATE action_executions 
		SET status = 'IN_PROGRESS', started_at = ?
		WHERE id = ? AND status = 'QUEUED'
	`, now, id)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

func (s *Service) FailExecution(id, errMsg string, attempt, maxRetries int) error {
	now := time.Now().Unix()
	if attempt >= maxRetries {
		_, err := s.db.Exec(`
			UPDATE action_executions
			SET status = 'EXHAUSTED', error_message = ?, attempt = ?, completed_at = ?
			WHERE id = ?
		`, errMsg, attempt, now, id)
		return err
	}

	nextRetry := calculateRetryDelay(attempt)
	_, err := s.db.Exec(`
		UPDATE action_executions
		SET status = 'FAILED', error_message = ?, attempt = ?, next_retry_at = ?
		WHERE id = ?
	`, errMsg, attempt, nextRetry, id)
	return err
}

func calculateRetryDelay(attempt int) int64 {
	delays := []time.Duration{0, 5 * time.Minute, 30 * time.Minute}
	if attempt < len(delays) {
		return time.Now().Add(delays[attempt]).Unix()
	}
	return time.Now().Add(24 * time.Hour).Unix()
}

func (s *Service) MarkCompleted(id string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		UPDATE action_executions 
		SET status = 'COMPLETED', completed_at = ?
		WHERE id = ?
	`, now, id)
	return err
}