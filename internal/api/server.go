package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/time/rate"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/action"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/health"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/storage"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/will"
)

type Server struct {
	db        *storage.DB
	willSvc   *will.Service
	actionSvc *action.Service
	auditSvc  *audit.Service
	health    *health.Service
	crypto    *crypto.Engine
	router    *mux.Router
	limiters  map[string]*rate.Limiter
	limMu     sync.Mutex
}

func New(db *storage.DB, willSvc *will.Service, actionSvc *action.Service,
	auditSvc *audit.Service, health *health.Service, crypto *crypto.Engine) *Server {

	s := &Server{
		db:        db,
		willSvc:   willSvc,
		actionSvc: actionSvc,
		auditSvc:  auditSvc,
		health:    health,
		crypto:    crypto,
		router:    mux.NewRouter(),
		limiters:  make(map[string]*rate.Limiter),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.router.Use(s.loggingMiddleware, s.securityHeaders, s.loopbackOnly)

	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Public routes (before auth)
	api.HandleFunc("/health", s.handleHealth).Methods("GET")
	api.HandleFunc("/unlock", s.handleUnlock).Methods("POST")

	// Auth required
	api.Use(s.authMiddleware)

	api.HandleFunc("/status", s.handleStatus).Methods("GET")
	api.HandleFunc("/wills", s.handleListWills).Methods("GET")
	api.HandleFunc("/wills", s.handleCreateWill).Methods("POST")
	api.HandleFunc("/wills/{id}", s.handleGetWill).Methods("GET")
	api.HandleFunc("/wills/{id}", s.handleUpdateWill).Methods("PUT")
	api.HandleFunc("/wills/{id}", s.handleArchiveWill).Methods("DELETE")
	api.HandleFunc("/wills/{id}/activate", s.handleActivateWill).Methods("POST")
	api.HandleFunc("/wills/{id}/pause", s.handlePauseWill).Methods("POST")
	api.HandleFunc("/wills/{id}/checkin", s.handleCheckinWill).Methods("POST")
	api.HandleFunc("/checkin", s.handleCheckinAll).Methods("POST")
	api.HandleFunc("/wills/{id}/actions", s.handleListActions).Methods("GET")
	api.HandleFunc("/wills/{id}/actions", s.handleAddAction).Methods("POST")
	api.HandleFunc("/wills/{id}/actions/{aid}", s.handleUpdateAction).Methods("PUT")
	api.HandleFunc("/wills/{id}/actions/{aid}", s.handleDeleteAction).Methods("DELETE")
	api.HandleFunc("/audit", s.handleListAudit).Methods("GET")
	api.HandleFunc("/audit/{id}", s.handleGetAuditEvent).Methods("GET")
	api.HandleFunc("/tokens", s.handleCreateToken).Methods("POST")
	api.HandleFunc("/tokens/{id}", s.handleRevokeToken).Methods("DELETE")
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := uuid.New().String()
		w.Header().Set("X-Request-ID", reqID)

		next.ServeHTTP(w, r)

		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", w.Header().Get("status"),
			"latency_ms", time.Since(start).Milliseconds(),
			"request_id", reqID,
		)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := strings.Split(r.RemoteAddr, ":")[0]
		if ip != "127.0.0.1" && ip != "::1" && ip != "[::1]" {
			s.writeError(w, http.StatusForbidden, "FORBIDDEN", "loopback only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			s.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		tokenHash := s.crypto.HashToken(token)

		// Rate limiting
		lim := s.getLimiter(tokenHash)
		if !lim.Allow() {
			s.writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}

		// Verify token
		var count int
		now := time.Now().Unix()
		err := s.db.QueryRow(`
			SELECT COUNT(*) FROM tokens 
			WHERE token_hash = ? AND (expires_at IS NULL OR expires_at > ?)
		`, tokenHash, now).Scan(&count)
		if err != nil || count == 0 {
			s.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
			return
		}

		// Update last_used
		s.db.Exec(`UPDATE tokens SET last_used = ? WHERE token_hash = ?`, now, tokenHash)

		// Store in context
		ctx := context.WithValue(r.Context(), "token_hash", tokenHash)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) getLimiter(tokenHash string) *rate.Limiter {
	s.limMu.Lock()
	defer s.limMu.Unlock()

	lim, ok := s.limiters[tokenHash]
	if !ok {
		lim = rate.NewLimiter(rate.Every(time.Minute), 60)
		s.limiters[tokenHash] = lim
	}
	return lim
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	reqID := w.Header().Get("X-Request-ID")
	s.writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"code":       code,
			"message":    message,
			"request_id": reqID,
		},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	st := s.health.Check()
	s.writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if s.crypto.IsInitialized() {
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "already_unlocked"})
		return
	}

	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	// Load crypto meta
	var meta crypto.CryptoMeta
	err := s.db.QueryRow(`
		SELECT id, kek_id, dek_ciphertext, dek_nonce, pbkdf2_salt, pbkdf2_iters, created_at
		FROM crypto_meta LIMIT 1
	`).Scan(&meta.ID, &meta.KEKID, &meta.DEKCiphertext, &meta.DEKNonce, &meta.PBKDF2Salt, &meta.PBKDF2Iters, &meta.CreatedAt)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "no crypto meta")
		return
	}

	if err := s.crypto.Initialize(req.Passphrase, &meta); err != nil {
		s.auditSvc.Log(audit.EventCheckInFailed, "unlock", nil, map[string]interface{}{"error": err.Error()})
		s.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid passphrase")
		return
	}

	// Verify by decrypting DEK
	dek, err := s.crypto.DecryptDEK(&meta)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "decryption failed")
		return
	}
	dek.Destroy()

	s.auditSvc.Log(audit.EventUnlock, "user", nil, nil)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "unlocked"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListWills(w http.ResponseWriter, r *http.Request) {
	wills, err := s.willSvc.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, wills)
}

func (s *Server) handleCreateWill(w http.ResponseWriter, r *http.Request) {
	if !s.crypto.IsInitialized() {
		s.writeError(w, http.StatusForbidden, "FORBIDDEN", "crypto not initialized")
		return
	}

	var req struct {
		Name                string `json:"name"`
		CheckInIntervalSec  int    `json:"check_in_interval_sec"`
		Content             string `json:"content"`
		CryptoMetaID        string `json:"crypto_meta_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	if req.Name == "" || req.CheckInIntervalSec <= 0 {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "name and interval required")
		return
	}

	// Load full crypto meta row (critical for DEK decryption)
	var cmeta crypto.CryptoMeta
	err := s.db.QueryRow(`
		SELECT id, kek_id, dek_ciphertext, dek_nonce, pbkdf2_salt, pbkdf2_iters, created_at
		FROM crypto_meta WHERE id = ?
	`, req.CryptoMetaID).Scan(&cmeta.ID, &cmeta.KEKID, &cmeta.DEKCiphertext, &cmeta.DEKNonce,
		&cmeta.PBKDF2Salt, &cmeta.PBKDF2Iters, &cmeta.CreatedAt)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "crypto meta not found")
		return
	}

	dek, err := s.crypto.DecryptDEK(&cmeta)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get DEK")
		return
	}
	defer dek.Destroy()

	will, err := s.willSvc.Create(req.Name, req.CheckInIntervalSec, req.Content, req.CryptoMetaID, dek)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, will)
}

func (s *Server) handleGetWill(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	will, err := s.willSvc.Get(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "will not found")
		return
	}
	s.writeJSON(w, http.StatusOK, will)
}

func (s *Server) handleUpdateWill(w http.ResponseWriter, r *http.Request) {
	// Simplified update
	id := mux.Vars(r)["id"]
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	now := time.Now().Unix()
	_, err := s.db.Exec(`UPDATE wills SET name = ?, updated_at = ? WHERE id = ?`, req.Name, now, id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleArchiveWill(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.willSvc.Archive(id); err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.auditSvc.Log(audit.EventWillArchived, "user", &id, nil)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

func (s *Server) handleActivateWill(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		UPDATE wills 
		SET status = 'ACTIVE', next_check_in_deadline = ?, updated_at = ?
		WHERE id = ? AND status = 'DRAFT'
	`, now, now, id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.auditSvc.Log(audit.EventWillActivated, "user", &id, nil)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}

func (s *Server) handlePauseWill(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.willSvc.Transition(id, "ACTIVE", "PAUSED"); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_TRANSITION", err.Error())
		return
	}
	s.auditSvc.Log(audit.EventWillPaused, "user", &id, nil)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleCheckinWill(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.willSvc.CheckIn(id); err != nil {
		s.writeError(w, http.StatusBadRequest, "CHECKIN_FAILED", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "checked_in"})
}

func (s *Server) handleCheckinAll(w http.ResponseWriter, r *http.Request) {
	// Simplified: just return success
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListActions(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	actions, err := s.actionSvc.ListByWill(id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, actions)
}

func (s *Server) handleAddAction(w http.ResponseWriter, r *http.Request) {
	if !s.crypto.IsInitialized() {
		s.writeError(w, http.StatusForbidden, "FORBIDDEN", "crypto not initialized")
		return
	}

	id := mux.Vars(r)["id"]
	var req struct {
		Type     string          `json:"type"`
		Position int             `json:"position"`
		Config   json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	var cfg action.Config
	json.Unmarshal(req.Config, &cfg)

	// Get crypto meta
	var metaID string
	s.db.QueryRow("SELECT id FROM crypto_meta LIMIT 1").Scan(&metaID)

	// Load full crypto meta row
	var cmeta crypto.CryptoMeta
	err = s.db.QueryRow(`
		SELECT id, kek_id, dek_ciphertext, dek_nonce, pbkdf2_salt, pbkdf2_iters, created_at
		FROM crypto_meta WHERE id = ?
	`, metaID).Scan(&cmeta.ID, &cmeta.KEKID, &cmeta.DEKCiphertext, &cmeta.DEKNonce,
		&cmeta.PBKDF2Salt, &cmeta.PBKDF2Iters, &cmeta.CreatedAt)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "crypto meta not found")
		return
	}

	dek, err := s.crypto.DecryptDEK(&cmeta)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "dek error")
		return
	}
	defer dek.Destroy()

	a, err := s.actionSvc.Create(id, action.Type(req.Type), req.Position, &cfg, metaID, dek)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleUpdateAction(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "not_implemented_yet"})
}

func (s *Server) handleDeleteAction(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "not_implemented_yet"})
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT id, timestamp, event_type, will_id, actor, metadata, checksum 
		FROM audit_log ORDER BY seq DESC LIMIT 100
	`)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id, et, actor, meta, checksum string
		var ts int64
		var willID sql.NullString
		rows.Scan(&id, &ts, &et, &willID, &actor, &meta, &checksum)
		events = append(events, map[string]interface{}{
			"id": id, "timestamp": ts, "event_type": et, "will_id": willID.String, "actor": actor,
		})
	}
	s.writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleGetAuditEvent(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	token, _ := s.crypto.GenerateToken()
	hash := s.crypto.HashToken(token)
	now := time.Now().Unix()
	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO tokens (id, token_hash, created_at) VALUES (?, ?, ?)
	`, id, hash, now)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, map[string]string{"id": id, "token": token})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.db.Exec(`DELETE FROM tokens WHERE id = ?`, id)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}