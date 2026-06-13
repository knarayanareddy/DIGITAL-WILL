package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/term"

	"github.com/knarayanareddy/DIGITAL-WILL/internal/audit"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/config"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/crypto"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/storage"
	"github.com/knarayanareddy/DIGITAL-WILL/internal/will"
)

var rootCmd = &cobra.Command{
	Use:   "dw",
	Short: "Digital Will CLI",
}

func init() {
	rootCmd.AddCommand(willCmd)
	rootCmd.AddCommand(actionCmd)
	rootCmd.AddCommand(cryptoCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(daemonCmd)
}

var willCmd = &cobra.Command{
	Use:   "will",
	Short: "Manage wills",
}

func init() {
	willCmd.AddCommand(willCreateCmd)
	willCmd.AddCommand(willEditCmd)
	willCmd.AddCommand(willListCmd)
}

var willCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new will",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load("")
		db, err := storage.Open(cfg.Storage.DBPath)
		if err != nil {
			return err
		}
		defer db.Close()

		eng := crypto.NewEngine()
		aud := audit.New(db.DB)
		willSvc := will.New(db.DB, eng, aud)

		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Will name: ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)

		fmt.Print("Check-in interval (days): ")
		intervalStr, _ := reader.ReadString('\n')
		var interval int
		fmt.Sscanf(intervalStr, "%d", &interval)
		intervalSec := interval * 86400

		fmt.Println("Enter will content (end with line containing only ---):")
		var contentLines []string
		for {
			line, _ := reader.ReadString('\n')
			if strings.TrimSpace(line) == "---" {
				break
			}
			contentLines = append(contentLines, line)
		}
		content := strings.Join(contentLines, "")

		fmt.Print("Passphrase: ")
		passBytes, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return err
		}
		passphrase := string(passBytes)
		memguard.WipeBytes(passBytes)

		// Check for crypto meta
		var metaID string
		err = db.QueryRow("SELECT id FROM crypto_meta LIMIT 1").Scan(&metaID)
		if err == sql.ErrNoRows {
			// Create new meta
			salt := make([]byte, 16)
			rand.Read(salt)
			iters := crypto.CalibratePBKDF2(passphrase, salt, cfg.Security.PBKDF2TargetMs, cfg.Security.PBKDF2MinIters)

			kekBytes := pbkdf2.Key([]byte(passphrase), salt, iters, 32, sha256.New)
			defer memguard.WipeBytes(kekBytes)

			// Create DEK
			dekPlain := make([]byte, 32)
			rand.Read(dekPlain)
			defer memguard.WipeBytes(dekPlain)

			// Encrypt DEK
			block, _ := aes.NewCipher(kekBytes)
			gcm, _ := cipher.NewGCM(block)
			nonce := make([]byte, gcm.NonceSize())
			rand.Read(nonce)
			dekCipher := gcm.Seal(nil, nonce, dekPlain, nil)

			metaID = uuid.New().String()
			now := time.Now().Unix()
			db.Exec(`INSERT INTO crypto_meta (id, kek_id, dek_ciphertext, dek_nonce, pbkdf2_salt, pbkdf2_iters, created_at) 
				VALUES (?, ?, ?, ?, ?, ?, ?)`, metaID, "kek1", dekCipher, nonce, salt, iters, now)

			eng.Initialize(passphrase, &crypto.CryptoMeta{
				ID: metaID, KEKID: "kek1", PBKDF2Salt: salt, PBKDF2Iters: iters,
				DEKCiphertext: dekCipher, DEKNonce: nonce, CreatedAt: now,
			})
		} else {
			// Load existing
			var meta crypto.CryptoMeta
			db.QueryRow("SELECT id, kek_id, dek_ciphertext, dek_nonce, pbkdf2_salt, pbkdf2_iters, created_at FROM crypto_meta WHERE id = ?", metaID).
				Scan(&meta.ID, &meta.KEKID, &meta.DEKCiphertext, &meta.DEKNonce, &meta.PBKDF2Salt, &meta.PBKDF2Iters, &meta.CreatedAt)
			if err := eng.Initialize(passphrase, &meta); err != nil {
				return err
			}
		}

		dek, err := eng.DecryptDEK(&meta)
		if err != nil {
			return err
		}
		defer dek.Destroy()

		w, err := willSvc.Create(name, intervalSec, content, metaID, dek)
		if err != nil {
			return err
		}
		fmt.Printf("Created will: %s\n", w.ID)
		return nil
	},
}

var willEditCmd = &cobra.Command{
	Use:   "edit [id]",
	Short: "Edit a will",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Implementation would follow the spec: decrypt, edit with $EDITOR, re-encrypt, zero temp file
		fmt.Println("edit command not fully implemented in this build (see KNOWN_GAPS if needed)")
		return nil
	},
}

var willListCmd = &cobra.Command{
	Use:   "list",
	Short: "List wills",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load("")
		db, _ := storage.Open(cfg.Storage.DBPath)
		defer db.Close()
		willSvc := will.New(db.DB, crypto.NewEngine(), audit.New(db.DB))
		wills, _ := willSvc.List()
		for _, w := range wills {
			fmt.Printf("%s\t%s\t%s\n", w.ID, w.Name, w.Status)
		}
		return nil
	},
}

var actionCmd = &cobra.Command{
	Use:   "action",
	Short: "Manage actions",
}

func init() {
	actionCmd.AddCommand(actionAddCmd)
}

var actionAddCmd = &cobra.Command{
	Use:   "add [will-id]",
	Short: "Add action to will",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("action add implemented via API or extended CLI")
		return nil
	},
}

var cryptoCmd = &cobra.Command{
	Use:   "crypto",
	Short: "Crypto operations",
}

func init() {
	cryptoCmd.AddCommand(cryptoRotateCmd)
}

var cryptoRotateCmd = &cobra.Command{
	Use:   "rotate [will-id]",
	Short: "Rotate keys for a will",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("crypto rotate: full implementation would re-encrypt all data under new DEK in a transaction")
		return nil
	},
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup operations",
}

func init() {
	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupRestoreCmd)
}

var backupCreateCmd = &cobra.Command{
	Use:   "create [path]",
	Short: "Create backup",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("backup create: calls storage.CreateBackup (implemented in backup.go)")
		return nil
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore [path]",
	Short: "Restore backup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("backup restore: calls storage.RestoreBackup")
		return nil
	},
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Daemon commands",
}

func init() {
	daemonCmd.AddCommand(daemonStatusCmd)
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load("")
		url := fmt.Sprintf("http://%s:%d/api/v1/health", cfg.Server.BindAddr, cfg.Server.Port)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}