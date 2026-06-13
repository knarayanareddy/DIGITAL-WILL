package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	User      UserConfig      `toml:"user"`
	Server    ServerConfig    `toml:"server"`
	Storage   StorageConfig   `toml:"storage"`
	Scheduler SchedulerConfig `toml:"scheduler"`
	Security  SecurityConfig  `toml:"security"`
	Logging   LoggingConfig   `toml:"logging"`
	Features  FeatureFlags    `toml:"features"`
}

type UserConfig struct {
	Name string `toml:"name"`
}

type ServerConfig struct {
	BindAddr string `toml:"bind_addr"`
	Port     int    `toml:"port"`
}

type StorageConfig struct {
	DBPath     string `toml:"db_path"`
	BackupPath string `toml:"backup_path"`
}

type SchedulerConfig struct {
	IntervalSec int `toml:"interval_sec"`
	Workers     int `toml:"workers"`
}

type SecurityConfig struct {
	RequireTLS     bool `toml:"require_tls"`
	PBKDF2TargetMs int  `toml:"pbkdf2_target_ms"`
	PBKDF2MinIters int  `toml:"pbkdf2_min_iters"`
}

type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

type FeatureFlags struct {
	KeyRotation bool `toml:"key_rotation"`
	WebUI       bool `toml:"web_ui"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{}

	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, ".digitalwill", "config.toml")
	}

	// Expand ~ if present
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to expand ~: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Apply defaults only
		applyDefaults(cfg)
		return cfg, validate(cfg)
	}

	_, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.BindAddr == "" {
		cfg.Server.BindAddr = "127.0.0.1"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8472
	}
	if cfg.Storage.DBPath == "" {
		home, _ := os.UserHomeDir()
		cfg.Storage.DBPath = filepath.Join(home, ".digitalwill", "db.sqlite")
	}
	if cfg.Storage.BackupPath == "" {
		home, _ := os.UserHomeDir()
		cfg.Storage.BackupPath = filepath.Join(home, ".digitalwill", "backups")
	}
	if cfg.Scheduler.IntervalSec == 0 {
		cfg.Scheduler.IntervalSec = 60
	}
	if cfg.Scheduler.Workers == 0 {
		cfg.Scheduler.Workers = 4
	}
	if cfg.Security.PBKDF2TargetMs == 0 {
		cfg.Security.PBKDF2TargetMs = 500
	}
	if cfg.Security.PBKDF2MinIters == 0 {
		cfg.Security.PBKDF2MinIters = 600000
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	// Feature defaults are false by default in struct, but explicit:
	if !cfg.Features.KeyRotation {
		cfg.Features.KeyRotation = true
	}
	if !cfg.Features.WebUI {
		cfg.Features.WebUI = true
	}
}

func validate(cfg *Config) error {
	if cfg.Scheduler.IntervalSec < 10 || cfg.Scheduler.IntervalSec > 3600 {
		return errors.New("scheduler.interval_sec must be between 10 and 3600")
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	if cfg.User.Name == "" {
		return errors.New("user.name is required")
	}
	return nil
}

func ExpandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}