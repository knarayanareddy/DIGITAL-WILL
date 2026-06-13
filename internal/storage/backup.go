package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"golang.org/x/crypto/pbkdf2"
)

type Manifest struct {
	Version      int    `json:"version"`
	CreatedAt    int64  `json:"created_at"`
	DBChecksum   string `json:"db_checksum"`
	SchemaVersion int   `json:"schema_version"`
	Encrypted    bool   `json:"encrypted"`
}

func CreateBackup(db *DB, backupPath, passphrase string) error {
	if backupPath == "" {
		backupPath = filepath.Join(filepath.Dir(db.DBPath()), "backups", fmt.Sprintf("backup-%d.sqlite", time.Now().Unix()))
	}
	os.MkdirAll(filepath.Dir(backupPath), 0700)

	// Use atomic VACUUM INTO for consistent snapshot (recommended over manual copy)
	vacuumErr := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", backupPath))
	if vacuumErr != nil {
		// Fallback to legacy method if VACUUM INTO fails (older SQLite or permissions)
		db.Exec("PRAGMA wal_checkpoint(FULL)")
		src, err := os.Open(db.DBPath())
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.Create(backupPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return err
		}
	}

	// Compute checksum of the backup file
	backupFile, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer backupFile.Close()

	h := sha256.New()
	if _, err := io.Copy(h, backupFile); err != nil {
		return err
	}
	checksum := hex.EncodeToString(h.Sum(nil))

	manifest := Manifest{
		Version:       1,
		CreatedAt:     time.Now().Unix(),
		DBChecksum:    checksum,
		SchemaVersion: 2,
		Encrypted:     passphrase != "",
	}

	if passphrase != "" {
		salt := make([]byte, 32)
		rand.Read(salt)
		key := pbkdf2.Key([]byte(passphrase), salt, 600000, 32, sha256.New)
		defer func() { for i := range key { key[i] = 0 } }()

		block, _ := aes.NewCipher(key)
		gcm, _ := cipher.NewGCM(block)
		nonce := make([]byte, gcm.NonceSize())
		rand.Read(nonce)

		// Re-read the backup file to encrypt
		backupData, _ := os.ReadFile(backupPath)
		ct := gcm.Seal(nil, nonce, backupData, nil)

		encPath := backupPath + ".enc"
		os.WriteFile(encPath, append(append(salt, nonce...), ct...), 0600)
		os.Remove(backupPath)
		backupPath = encPath
	}

	// Write manifest
	manifestPath := backupPath + ".manifest"
	mb, _ := json.Marshal(manifest)
	os.WriteFile(manifestPath, mb, 0600)

	// Audit
	aud := audit.New(db.DB)
	aud.Log(audit.EventBackupCreated, "cli", nil, map[string]interface{}{"path": backupPath, "checksum": checksum})

	fmt.Printf("Backup created: %s (checksum: %s)\n", backupPath, checksum)
	return nil
}

func RestoreBackup(db *DB, backupPath, passphrase string) error {
	manifestPath := backupPath + ".manifest"
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return errors.New("manifest not found")
	}

	var manifest Manifest
	mb, _ := os.ReadFile(manifestPath)
	json.Unmarshal(mb, &manifest)

	var dbData []byte
	if manifest.Encrypted {
		if passphrase == "" {
			return errors.New("passphrase required for encrypted backup")
		}
		encData, _ := os.ReadFile(backupPath)
		if len(encData) < 44 {
			return errors.New("invalid encrypted backup")
		}
		salt := encData[:32]
		nonce := encData[32:44]
		ct := encData[44:]

		key := pbkdf2.Key([]byte(passphrase), salt, 600000, 32, sha256.New)
		defer func() { for i := range key { key[i] = 0 } }()

		block, _ := aes.NewCipher(key)
		gcm, _ := cipher.NewGCM(block)
		plain, err := gcm.Open(nil, nonce, ct, nil)
		if err != nil {
			return errors.New("decryption failed")
		}
		dbData = plain
	} else {
		dbData, _ = os.ReadFile(backupPath)
	}

	// Verify checksum
	h := sha256.Sum256(dbData)
	if hex.EncodeToString(h[:]) != manifest.DBChecksum {
		return errors.New("checksum mismatch - backup corrupted")
	}

	// Close current DB
	db.Close()

	// Atomic replace
	tmpPath := db.DBPath() + ".tmp"
	os.WriteFile(tmpPath, dbData, 0600)
	os.Rename(tmpPath, db.DBPath())

	fmt.Println("Backup restored successfully")
	return nil
}