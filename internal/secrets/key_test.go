package secrets

import (
	"bytes"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveKeyEnvWinsOverFile(t *testing.T) {
	// Write a key file that should be ignored when MXLRC_MASTER_KEY is set.
	dir := t.TempDir()
	keyPath := filepath.Join(dir, DefaultKeyFileName)
	fileKey := bytes.Repeat([]byte{0xAA}, KeySize)
	if err := os.WriteFile(keyPath, fileKey, 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	envKey := bytes.Repeat([]byte{0xBB}, KeySize)
	got, err := ResolveKey(KeyOptions{
		MasterKeyB64: base64.StdEncoding.EncodeToString(envKey),
		KeyFilePath:  keyPath,
	})
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if !bytes.Equal(got, envKey) {
		t.Fatal("env key did not win over key file")
	}
}

func TestResolveKeyMalformedMasterKeyFatal(t *testing.T) {
	cases := map[string]string{
		"bad base64":   "not!valid!base64!",
		"wrong length": base64.StdEncoding.EncodeToString([]byte("only-a-few-bytes")),
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveKey(KeyOptions{MasterKeyB64: val}); err == nil {
				t.Fatal("ResolveKey accepted malformed MXLRC_MASTER_KEY; want fatal error")
			}
		})
	}
}

func TestResolveKeyAutoCreatesKeyFile0600(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, DefaultKeyFileName)
	got, err := ResolveKey(KeyOptions{KeyFilePath: keyPath})
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if len(got) != KeySize {
		t.Fatalf("generated key len = %d, want %d", len(got), KeySize)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, want 0600", info.Mode().Perm())
	}
	// A second resolve reads the same key back.
	again, err := ResolveKey(KeyOptions{KeyFilePath: keyPath})
	if err != nil {
		t.Fatalf("ResolveKey (reload): %v", err)
	}
	if !bytes.Equal(got, again) {
		t.Fatal("reloaded key differs from generated key")
	}
}

func TestResolveKeyLoosePermsWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits unreliable on Windows")
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, DefaultKeyFileName)
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{0x11}, KeySize), 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if _, err := ResolveKey(KeyOptions{KeyFilePath: keyPath, Logger: logger}); err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if !strings.Contains(buf.String(), "loose permissions") {
		t.Fatalf("expected loose-permissions warning, got: %q", buf.String())
	}
}

func TestResolveKeyBadLengthFileFatal(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, DefaultKeyFileName)
	if err := os.WriteFile(keyPath, []byte("too short"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	if _, err := ResolveKey(KeyOptions{KeyFilePath: keyPath}); err == nil {
		t.Fatal("ResolveKey accepted wrong-length key file; want fatal error")
	}
}

func TestResolveKeyEmptyKeyFilePathFatal(t *testing.T) {
	if _, err := ResolveKey(KeyOptions{}); err == nil {
		t.Fatal("ResolveKey with no master key and no key file path succeeded; want error")
	}
}

func TestResolveKeyDockerFirstRunHint(t *testing.T) {
	_, err := ResolveKey(KeyOptions{DockerMode: true})
	var fr *FirstRunError
	if !errors.As(err, &fr) {
		t.Fatalf("ResolveKey Docker first-run err = %v; want *FirstRunError", err)
	}
	// The suggested key must be valid base64 of 32 bytes.
	raw, decErr := base64.StdEncoding.DecodeString(fr.SuggestedKeyB64)
	if decErr != nil {
		t.Fatalf("suggested key not base64: %v", decErr)
	}
	if len(raw) != KeySize {
		t.Fatalf("suggested key decodes to %d bytes, want %d", len(raw), KeySize)
	}
	// Message carries the copy-pasteable assignment line, once, for stderr.
	msg := fr.Message()
	if !strings.HasPrefix(msg, "MXLRC_MASTER_KEY="+fr.SuggestedKeyB64) {
		t.Fatalf("Message does not start with the key assignment line: %q", msg)
	}
	if strings.Count(msg, "MXLRC_MASTER_KEY=") != 1 {
		t.Fatalf("MXLRC_MASTER_KEY= should appear exactly once, got: %q", msg)
	}
}

func TestResolveKeyDockerEnvKeyServes(t *testing.T) {
	// Docker mode WITH a valid master key resolves normally (no hint, no file).
	envKey := bytes.Repeat([]byte{0xCC}, KeySize)
	got, err := ResolveKey(KeyOptions{
		DockerMode:   true,
		MasterKeyB64: base64.StdEncoding.EncodeToString(envKey),
	})
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if !bytes.Equal(got, envKey) {
		t.Fatal("docker-mode env key not resolved")
	}
}
