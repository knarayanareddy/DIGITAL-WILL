package will

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
)

var (
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrWillNotActive     = errors.New("will is not active")
	ErrNoPayload         = errors.New("no encrypted payload")
)

type Will struct {
	ID                     string
	Name                   string
	Status                 string
	CheckInIntervalSec     int
	LastCheckIn            *int64
	NextCheckInDeadline    *int64
	MaxRetries             int
	CreatedAt              int64
	UpdatedAt              int64
	EncryptedPayload       []byte
	CryptoMetaID           string
}

type Payload struct {
	Content string `json:"content"`
}

type Service struct {
	db     *sql.DB
	crypto *crypto.Engine
	audit  *audit.Service
}

func New(db *sql.DB, crypto *crypto.Engine, audit *audit.Service) *Service {
	return &Service{db: db, crypto: crypto, audit: audit}
}

func (s *Service) Create(name string, intervalSec int, content string, cryptoMetaID string, dek *memguard.Enclave) (*Will, error) {
	if name == "" {
		return nil, errors.New("name required")
	}
	if intervalSec <= 0 {
		return nil, errors.New("interval must be positive")
	}
	if !s.crypto.IsInitialized() {
		return nil, errors.New("crypto not initialized")
	}

	id := uuid.New().String()
	now := time.Now().Unix()

	var encPayload, nonce []byte
	var err error
	if content != "" {
		aad := []byte(id + "|" + cryptoMetaID)
		encPayload, nonce, err = s.crypto.Encrypt(dek, []byte(content), aad)
		if err != nil {
			return nil, err
		}
		// Note: nonce is prepended in storage? For simplicity we store nonce + ct together
		encPayload = append(nonce, encPayload...)
	}

	w := &Will{
		ID:                  id,
		Name:                name,
		Status:              "DRAFT",
		CheckInIntervalSec:  intervalSec,
		MaxRetries:          3,
		CreatedAt:           now,
		UpdatedAt:           now,
		EncryptedPayload:    encPayload,
		CryptoMetaID:        cryptoMetaID,
	}

	_, err = s.db.Exec(`
		INSERT INTO wills (id, name, status, check_in_interval_sec, max_retries, created_at, updated_at, encrypted_payload, crypto_meta_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, w.ID, w.Name, w.Status, w.CheckInIntervalSec, w.MaxRetries, w.CreatedAt, w.UpdatedAt, w.EncryptedPayload, w.CryptoMetaID)
	if err != nil {
		return nil, err
	}

	s.audit.Log(audit.EventWillCreated, "user", &w.ID, map[string]interface{}{"name": name})
	return w, nil
}

func (s *Service) Get(id string) (*Will, error) {
	w := &Will{}
	err := s.db.QueryRow(`
		SELECT id, name, status, check_in_interval_sec, last_check_in, next_check_in_deadline,
		       max_retries, created_at, updated_at, encrypted_payload, crypto_meta_id
		FROM wills WHERE id = ?
	`, id).Scan(&w.ID, &w.Name, &w.Status, &w.CheckInIntervalSec, &w.LastCheckIn, &w.NextCheckInDeadline,
		&w.MaxRetries, &w.CreatedAt, &w.UpdatedAt, &w.EncryptedPayload, &w.CryptoMetaID)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (s *Service) List() ([]*Will, error) {
	rows, err := s.db.Query(`
		SELECT id, name, status, check_in_interval_sec, last_check_in, next_check_in_deadline,
		       max_retries, created_at, updated_at, encrypted_payload, crypto_meta_id
		FROM wills ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wills []*Will
	for rows.Next() {
		w := &Will{}
		if err := rows.Scan(&w.ID, &w.Name, &w.Status, &w.CheckInIntervalSec, &w.LastCheckIn, &w.NextCheckInDeadline,
			&w.MaxRetries, &w.CreatedAt, &w.UpdatedAt, &w.EncryptedPayload, &w.CryptoMetaID); err != nil {
			return nil, err
		}
		wills = append(wills, w)
	}
	return wills, rows.Err()
}

func (s *Service) Transition(willID, fromStatus, toStatus string) error {
	valid := false
	switch fromStatus {
	case "DRAFT":
		valid = toStatus == "ACTIVE"
	case "ACTIVE":
		valid = toStatus == "PAUSED" || toStatus == "PENDING_TRIGGER"
	case "PAUSED":
		valid = toStatus == "ACTIVE"
	case "PENDING_TRIGGER":
		valid = toStatus == "TRIGGERED"
	}
	if !valid {
		return ErrInvalidTransition
	}

	now := time.Now().Unix()
	res, err := s.db.Exec(`
		UPDATE wills SET status = ?, updated_at = ? WHERE id = ? AND status = ?
	`, toStatus, now, willID, fromStatus)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrInvalidTransition
	}
	return nil
}

func (s *Service) TransitionToPending(willID string) (bool, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(`
		UPDATE wills 
		SET status = 'PENDING_TRIGGER', updated_at = ?
		WHERE id = ? AND status = 'ACTIVE' AND next_check_in_deadline <= ?
	`, now, willID, now)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

func (s *Service) GetOverdue() ([]*Will, error) {
	now := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT id, name, status, check_in_interval_sec, last_check_in, next_check_in_deadline,
		       max_retries, created_at, updated_at, encrypted_payload, crypto_meta_id
		FROM wills 
		WHERE status = 'ACTIVE' AND next_check_in_deadline <= ?
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Will
	for rows.Next() {
		w := &Will{}
		if err := rows.Scan(&w.ID, &w.Name, &w.Status, &w.CheckInIntervalSec, &w.LastCheckIn, &w.NextCheckInDeadline,
			&w.MaxRetries, &w.CreatedAt, &w.UpdatedAt, &w.EncryptedPayload, &w.CryptoMetaID); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

func (s *Service) CheckIn(willID string) error {
	now := time.Now().Unix()
	res, err := s.db.Exec(`
		UPDATE wills 
		SET last_check_in = ?, 
		    next_check_in_deadline = ? + check_in_interval_sec,
		    updated_at = ?
		WHERE id = ? AND status = 'ACTIVE'
	`, now, now, now, willID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrWillNotActive
	}
	s.audit.Log(audit.EventCheckIn, "user", &willID, nil)
	return nil
}

func (s *Service) DecryptPayload(w *Will, dek *memguard.Enclave) (*Payload, error) {
	if len(w.EncryptedPayload) < 12 {
		return nil, ErrNoPayload
	}
	nonce := w.EncryptedPayload[:12]
	ct := w.EncryptedPayload[12:]
	aad := []byte(w.ID + "|" + w.CryptoMetaID)

	plain, err := s.crypto.Decrypt(dek, ct, nonce, aad)
	if err != nil {
		return nil, err
	}
	return &Payload{Content: string(plain)}, nil
}

func (s *Service) UpdateStatus(id, status string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`UPDATE wills SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	return err
}

func (s *Service) Archive(id string) error {
	return s.UpdateStatus(id, "ARCHIVED")
}