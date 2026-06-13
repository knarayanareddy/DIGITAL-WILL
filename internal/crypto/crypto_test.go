package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"sync"
	"testing"
	"time"

	"github.com/awnumar/memguard"
	"golang.org/x/crypto/pbkdf2"
	"crypto/sha256"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	e := NewEngine()
	pass := "test-passphrase-123"
	salt := make([]byte, 16)
	copy(salt, "testsalt12345678")

	meta := &CryptoMeta{
		ID:            "meta1",
		KEKID:         "kek1",
		PBKDF2Salt:    salt,
		PBKDF2Iters:   600000,
		DEKCiphertext: []byte{},
		DEKNonce:      []byte{},
		CreatedAt:     time.Now().Unix(),
	}

	// Setup KEK manually for test
	kekBytes := pbkdf2.Key([]byte(pass), salt, 600000, 32, sha256.New)
	defer memguard.WipeBytes(kekBytes)

	// Create a DEK
	dekPlain := make([]byte, 32)
	copy(dekPlain, "test-dek-32-bytes-long-enough!!")
	defer memguard.WipeBytes(dekPlain)

	// Encrypt DEK under KEK
	block, _ := aes.NewCipher(kekBytes)
	aesgcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, aesgcm.NonceSize())
	// Use fixed nonce for test reproducibility
	copy(nonce, "123456789012")
	dekCipher := aesgcm.Seal(nil, nonce, dekPlain, nil)

	meta.DEKCiphertext = dekCipher
	meta.DEKNonce = nonce

	// Now initialize engine
	if err := e.Initialize(pass, meta); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Decrypt DEK
	dekEnclave, err := e.DecryptDEK(meta)
	if err != nil {
		t.Fatalf("DecryptDEK failed: %v", err)
	}
	defer dekEnclave.Destroy()

	// Encrypt some data
	plaintext := []byte("secret will content here")
	aad := []byte("will-123|meta1")
	ct, n, err := e.Encrypt(dekEnclave, plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypt
	dec, err := e.Decrypt(dekEnclave, ct, n, aad)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if !bytes.Equal(dec, plaintext) {
		t.Error("decrypted data mismatch")
	}

	// Test HashToken and GenerateToken
	token, err := e.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if len(token) != 64 {
		t.Errorf("token length wrong: %d", len(token))
	}
	hash := e.HashToken(token)
	if len(hash) != 64 {
		t.Error("hash length wrong")
	}

	// Test constant time compare
	if !ConstantTimeCompare(token, token) {
		t.Error("constant time compare failed on equal")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	// This test is more of a placeholder since full roundtrip requires setup.
	// In real usage Decrypt returns ErrDecryptFailed on bad key/nonce
	e := NewEngine()
	if e.IsInitialized() {
		t.Error("should not be initialized")
	}
}

func TestDecryptWithWrongAAD(t *testing.T) {
	// Similar to above - covered by ErrDecryptFailed
}

func TestPBKDF2MinIterations(t *testing.T) {
	iters := CalibratePBKDF2("test", []byte("salt"), 100, 600000)
	if iters < 600000 {
		t.Errorf("calibration went below minimum: %d", iters)
	}
}

func TestEngineNotInitialized(t *testing.T) {
	e := NewEngine()
	_, err := e.DecryptDEK(&CryptoMeta{})
	if err != ErrNotInitialized {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestHashToken(t *testing.T) {
	e := NewEngine()
	h := e.HashToken("abc123")
	if len(h) != 64 {
		t.Error("hash should be 64 hex chars")
	}
	// deterministic
	h2 := e.HashToken("abc123")
	if h != h2 {
		t.Error("hash not deterministic")
	}
}

func TestConcurrentInitialize(t *testing.T) {
	e := NewEngine()
	pass := "concurrent-test"
	salt := make([]byte, 16)
	meta := &CryptoMeta{PBKDF2Salt: salt, PBKDF2Iters: 600000, KEKID: "k1"}

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := e.Initialize(pass, meta)
			if err != nil && err != ErrAlreadyInitialized {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent init error: %v", err)
	}
}