package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/awnumar/memguard"
	"golang.org/x/crypto/pbkdf2"
)

var (
	ErrNotInitialized  = errors.New("crypto engine not initialized")
	ErrAlreadyInitialized = errors.New("crypto engine already initialized")
	ErrDecryptFailed   = errors.New("decryption failed")
	ErrInvalidKey      = errors.New("invalid key material")
)

type Engine struct {
	initialized bool
	kek         *memguard.Enclave
	kekID       string
	mu          sync.RWMutex
}

type CryptoMeta struct {
	ID            string
	KEKID         string
	DEKCiphertext []byte
	DEKNonce      []byte
	PBKDF2Salt    []byte
	PBKDF2Iters   int
	CreatedAt     int64
	RotatedAt     *int64
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Initialize(passphrase string, meta *CryptoMeta) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.initialized {
		return ErrAlreadyInitialized
	}
	if passphrase == "" {
		return errors.New("passphrase cannot be empty")
	}
	if len(meta.PBKDF2Salt) == 0 || meta.PBKDF2Iters < 600000 {
		return errors.New("invalid crypto meta for initialization")
	}

	// Derive KEK
	kekBytes := pbkdf2.Key([]byte(passphrase), meta.PBKDF2Salt, meta.PBKDF2Iters, 32, sha256.New)
	defer memguard.WipeBytes(kekBytes)

	enclave := memguard.NewEnclave(kekBytes)
	memguard.WipeBytes(kekBytes)

	e.kek = enclave
	e.kekID = meta.KEKID
	e.initialized = true
	return nil
}

func (e *Engine) IsInitialized() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.initialized
}

func (e *Engine) DecryptDEK(meta *CryptoMeta) (*memguard.Enclave, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.initialized {
		return nil, ErrNotInitialized
	}

	kek, err := e.kek.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open KEK: %w", err)
	}
	defer kek.Destroy()

	block, err := aes.NewCipher(kek.Bytes())
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	dekPlain, err := aesgcm.Open(nil, meta.DEKNonce, meta.DEKCiphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	defer memguard.WipeBytes(dekPlain)

	return memguard.NewEnclave(dekPlain), nil
}

func (e *Engine) Encrypt(dek *memguard.Enclave, plaintext, aad []byte) (ciphertext, nonce []byte, err error) {
	if dek == nil {
		return nil, nil, errors.New("DEK enclave is nil")
	}

	dekBytes, err := dek.Open()
	if err != nil {
		return nil, nil, err
	}
	defer dekBytes.Destroy()

	block, err := aes.NewCipher(dekBytes.Bytes())
	if err != nil {
		return nil, nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = aesgcm.Seal(nil, nonce, plaintext, aad)
	return ciphertext, nonce, nil
}

func (e *Engine) Decrypt(dek *memguard.Enclave, ciphertext, nonce, aad []byte) ([]byte, error) {
	if dek == nil {
		return nil, errors.New("DEK enclave is nil")
	}

	dekBytes, err := dek.Open()
	if err != nil {
		return nil, err
	}
	defer dekBytes.Destroy()

	block, err := aes.NewCipher(dekBytes.Bytes())
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plaintext, nil
}

func (e *Engine) GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (e *Engine) HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (e *Engine) Shutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.kek != nil {
		e.kek.Destroy()
	}
	e.initialized = false
	memguard.Purge()
}

func CalibratePBKDF2(passphrase string, salt []byte, targetMs, minIters int) int {
	if minIters < 600000 {
		minIters = 600000
	}

	start := time.Now()
	_ = pbkdf2.Key([]byte(passphrase), salt, minIters, 32, sha256.New)
	elapsedNs := time.Since(start).Nanoseconds()
	if elapsedNs < 1_000_000 {
		elapsedNs = 1_000_000
	}

	targetNs := int64(targetMs) * 1_000_000
	iters := minIters
	if elapsedNs < targetNs {
		ratio := float64(targetNs) / float64(elapsedNs)
		iters = int(float64(minIters) * ratio)
	}
	if iters < minIters {
		iters = minIters
	}
	return iters
}

func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}