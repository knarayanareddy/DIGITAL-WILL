package health

import (
	"context"
	"database/sql"
	"time"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/scheduler"
)

type Status struct {
	Status    string          `json:"status"`
	Scheduler SchedulerStatus `json:"scheduler"`
	Crypto    CryptoStatus    `json:"crypto"`
	DB        DBStatus        `json:"db"`
	Version   string          `json:"version"`
}

type SchedulerStatus struct {
	Status string `json:"status"`
}

type CryptoStatus struct {
	Initialized bool `json:"initialized"`
}

type DBStatus struct {
	Reachable bool `json:"reachable"`
}

type Service struct {
	db   *sql.DB
	sched *scheduler.Scheduler
	crypto *crypto.Engine
}

func New(db *sql.DB, sched *scheduler.Scheduler, crypto *crypto.Engine) *Service {
	return &Service{db: db, sched: sched, crypto: crypto}
}

func (s *Service) Check() Status {
	status := Status{
		Version: "1.0.0",
	}

	// DB check
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := s.db.PingContext(ctx)
	status.DB.Reachable = err == nil

	// Scheduler
	status.Scheduler.Status = s.sched.Status()

	// Crypto
	status.Crypto.Initialized = s.crypto.IsInitialized()

	// Overall
	if !status.DB.Reachable || status.Scheduler.Status == "stalled" {
		status.Status = "critical"
	} else if !status.Crypto.Initialized {
		status.Status = "degraded"
	} else {
		status.Status = "ok"
	}

	return status
}