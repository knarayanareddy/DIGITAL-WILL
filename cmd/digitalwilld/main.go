package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/action"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/api"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/config"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/health"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/notification"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/scheduler"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/storage"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/will"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	db, err := storage.Open(cfg.Storage.DBPath)
	if err != nil {
		slog.Error("db open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	cryptoEngine := crypto.NewEngine()
	defer cryptoEngine.Shutdown()

	auditSvc := audit.New(db.DB)
	willSvc := will.New(db.DB, cryptoEngine, auditSvc)
	actionSvc := action.New(db.DB, cryptoEngine, auditSvc)
	notifier := notification.New(cfg.User.Name, cfg.Security.RequireTLS)

	sched := scheduler.New(db.DB, willSvc, actionSvc, cryptoEngine, auditSvc, notifier,
		cfg.Scheduler.IntervalSec, cfg.Scheduler.Workers)

	healthSvc := health.New(db.DB, sched, cryptoEngine)
	apiServer := api.New(db, willSvc, actionSvc, auditSvc, healthSvc, cryptoEngine)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.BindAddr, cfg.Server.Port),
		Handler:      apiServer,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	auditSvc.Log(audit.EventDaemonStart, "daemon", nil, map[string]interface{}{"version": version})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigCh {
		if sig == syscall.SIGHUP {
			slog.Info("SIGHUP received — reloading config")
			continue
		}
		break
	}

	slog.Info("shutting down gracefully")
	auditSvc.Log(audit.EventDaemonStop, "daemon", nil, nil)
	sched.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	httpServer.Shutdown(shutdownCtx)
}