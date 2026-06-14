package secrets

import (
	"bytes"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(key) != KeySize {
		t.Fatalf("key len = %d, want %d", len(key), KeySize)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("super-secret-musixmatch-token")
	blob, err := Encrypt(key, plaintext, "musixmatch_token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Blob layout: nonce(12) || ciphertext || tag(16). Plaintext must not appear.
	if len(blob) != NonceSize+len(plaintext)+tagSize {
		t.Fatalf("blob len = %d, want %d", len(blob), NonceSize+len(plaintext)+tagSize)
	}
	if bytes.Contains(blob, plaintext) {
		t.Fatal("blob contains plaintext")
	}
	got, err := Decrypt(key, blob, "musixmatch_token")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip = %q, want %q", got, plaintext)
	}
}

func TestDecryptTamperedBlobFails(t *testing.T) {
	key := testKey(t)
	blob, err := Encrypt(key, []byte("value"), "name")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Flip a bit in the ciphertext/tag region (past the nonce).
	tampered := bytes.Clone(blob)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := Decrypt(key, tampered, "name"); err == nil {
		t.Fatal("Decrypt of tampered blob succeeded; want error")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	key := testKey(t)
	other := testKey(t)
	blob, err := Encrypt(key, []byte("value"), "name")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Decrypt(other, blob, "name"); err == nil {
		t.Fatal("Decrypt with wrong key succeeded; want error")
	}
}

func TestDecryptAADMismatchFails(t *testing.T) {
	key := testKey(t)
	blob, err := Encrypt(key, []byte("value"), "webhook_api_key")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Decrypting under a different name (a row-swap) must fail.
	if _, err := Decrypt(key, blob, "musixmatch_token"); err == nil {
		t.Fatal("Decrypt with mismatched AAD name succeeded; want error")
	}
}

func TestDecryptShortBlobFails(t *testing.T) {
	key := testKey(t)
	if _, err := Decrypt(key, []byte("too-short"), "name"); err == nil {
		t.Fatal("Decrypt of short blob succeeded; want error")
	}
}

func TestEncryptNonceUniqueness(t *testing.T) {
	key := testKey(t)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		blob, err := Encrypt(key, []byte("value"), "name")
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		nonce := string(blob[:NonceSize])
		if seen[nonce] {
			t.Fatal("duplicate nonce across encryptions")
		}
		seen[nonce] = true
	}
}

func TestNewGCMRejectsBadKeyLength(t *testing.T) {
	if _, err := Encrypt([]byte("short"), []byte("v"), "n"); err == nil {
		t.Fatal("Encrypt with short key succeeded; want error")
	}
	if _, err := Decrypt([]byte("short"), make([]byte, NonceSize+tagSize), "n"); err == nil {
		t.Fatal("Decrypt with short key succeeded; want error")
	}
}
