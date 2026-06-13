package storage

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const SchemaVersion = 2

func Migrate(db *sql.DB) error {
	// Get current user_version
	var currentVersion int
	err := db.QueryRow("PRAGMA user_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to read user_version: %w", err)
	}

	if currentVersion > SchemaVersion {
		return fmt.Errorf("database schema newer than application (db=%d, app=%d)", currentVersion, SchemaVersion)
	}

	if currentVersion == SchemaVersion {
		return nil // already up to date
	}

	// Run migrations in order
	for v := currentVersion + 1; v <= SchemaVersion; v++ {
		filename := fmt.Sprintf("migrations/%03d_*.sql", v)
		// Find the file
		entries, err := fs.Glob(migrationFS, filename)
		if err != nil || len(entries) == 0 {
			return fmt.Errorf("migration file for version %d not found", v)
		}
		// Use the first match (we have exactly one per version)
		content, err := migrationFS.ReadFile(entries[0])
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", entries[0], err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin tx for migration %d: %w", v, err)
		}

		// Execute the SQL (may contain multiple statements)
		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %d: %w", v, err)
		}

		// Update user_version
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to set user_version %d: %w", v, err)
		}

		// Compute checksum
		checksum := fmt.Sprintf("%x", sha256.Sum256(content))

		// Record in schema_migrations
		_, err = tx.Exec(`
			INSERT INTO schema_migrations (version, applied_at, checksum)
			VALUES (?, ?, ?)
		`, v, time.Now().Unix(), checksum)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", v, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", v, err)
		}

		slog.Info("applied migration", "version", v, "file", entries[0])
	}

	return nil
}

func GetSchemaVersion(db *sql.DB) (int, error) {
	var v int
	err := db.QueryRow("PRAGMA user_version").Scan(&v)
	return v, err
}

// VerifyChecksum checks the stored checksums for migrations
func VerifyChecksums(db *sql.DB) error {
	rows, err := db.Query("SELECT version, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var version int
		var storedChecksum string
		if err := rows.Scan(&version, &storedChecksum); err != nil {
			return err
		}

		// Re-read the migration file and compare
		filenamePattern := fmt.Sprintf("migrations/%03d_*.sql", version)
		entries, _ := fs.Glob(migrationFS, filenamePattern)
		if len(entries) == 0 {
			return fmt.Errorf("migration file for v%d missing", version)
		}
		content, _ := migrationFS.ReadFile(entries[0])
		computed := fmt.Sprintf("%x", sha256.Sum256(content))
		if !strings.EqualFold(computed, storedChecksum) {
			return fmt.Errorf("checksum mismatch for migration %d", version)
		}
	}
	return rows.Err()
}