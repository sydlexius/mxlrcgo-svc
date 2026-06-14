// Package secrets provides an encrypted-at-rest store for the small set of
// recoverable runtime secrets (the Musixmatch API token and the serve-mode
// webhook API key). Values are sealed with AES-256-GCM under a managed 32-byte
// key and persisted as opaque BLOBs in the existing pure-Go SQLite database.
//
// The design of record is docs/design/2026-06-13-223-secrets-encryption.md.
// This package is the storage foundation only: precedence wiring, the CLI, and
// docs are separate follow-on work.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

const (
	// KeySize is the AES-256 key length in bytes.
	KeySize = 32
	// NonceSize is the AES-GCM nonce length in bytes. It matches
	// gcm.NonceSize() for the standard 96-bit GCM nonce.
	NonceSize = 12
	// tagSize is the AES-GCM authentication tag length in bytes.
	tagSize = 16
)

// newGCM builds an AES-256-GCM AEAD from a 32-byte key.
func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secrets: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: new gcm: %w", err)
	}
	return gcm, nil
}

// Encrypt seals plaintext with AES-256-GCM under key, binding the ciphertext to
// name via the GCM additional authenticated data (AAD). The returned blob is
// laid out as nonce(12) || ciphertext || tag(16): a fresh 12-byte random nonce
// is generated per call and prefixed, so the blob is self-describing for
// decryption (and for future per-row key rotation). name must match on Decrypt
// or authentication fails.
func Encrypt(key, plaintext []byte, name string) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secrets: read nonce: %w", err)
	}
	// Seal appends ciphertext||tag to the nonce prefix, yielding the full blob.
	return gcm.Seal(nonce, nonce, plaintext, []byte(name)), nil
}

// Decrypt opens a nonce(12) || ciphertext || tag(16) blob produced by Encrypt,
// verifying integrity and the name binding (AAD). A tampered or truncated blob,
// the wrong key, or a mismatched name all fail loudly with an error rather than
// returning garbage plaintext.
func Decrypt(key, blob []byte, name string) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(blob) < NonceSize+tagSize {
		return nil, fmt.Errorf("secrets: ciphertext blob too short: %d bytes", len(blob))
	}
	nonce, ciphertext := blob[:NonceSize], blob[NonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(name))
	if err != nil {
		return nil, fmt.Errorf("secrets: decrypt %q: %w", name, err)
	}
	return plaintext, nil
}

// GenerateKey returns a fresh random 32-byte AES-256 key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("secrets: generate key: %w", err)
	}
	return key, nil
}
