package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// DefaultKeyFileName is the hidden key file written alongside the database on
// native installs. Its directory is resolved by the caller (the XDG data dir).
const DefaultKeyFileName = ".mxlrcgo.key"

// KeyOptions describes how to resolve the 32-byte master key. The caller (the
// config/startup layer) supplies the already-resolved inputs so this package
// stays decoupled from config and easily testable: MasterKeyB64 is the raw
// MXLRC_MASTER_KEY env value, KeyFilePath is the resolved key file location
// (default DefaultKeyFileName under the data dir, or a secrets.key_file /
// MXLRC_SECRETS_KEY_FILE override), and DockerMode reports whether the daemon
// is running with /config paths.
type KeyOptions struct {
	MasterKeyB64 string
	KeyFilePath  string
	DockerMode   bool
	Logger       *slog.Logger
}

func (o KeyOptions) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

// FirstRunError is returned by ResolveKey on the first Docker run when no
// MXLRC_MASTER_KEY is set. It carries a freshly generated, base64-encoded
// suggested key. The caller must print Message() to stderr exactly once (never
// to the slog file) and fatal-exit without serving; the daemon must never run
// unencrypted. See decision A=(c) in the design of record.
type FirstRunError struct {
	SuggestedKeyB64 string
}

func (e *FirstRunError) Error() string {
	return "secrets: MXLRC_MASTER_KEY is required in Docker mode; see the suggested key printed to stderr"
}

// Message returns the one-time onboarding text for stderr. The first line is the
// copy-pasteable MXLRC_MASTER_KEY=<base64> assignment; the remainder explains
// what to do. This must go to stderr only, never the rotating slog file.
func (e *FirstRunError) Message() string {
	return "MXLRC_MASTER_KEY=" + e.SuggestedKeyB64 + "\n" +
		"\n" +
		"No encryption master key is configured. A new one was generated above.\n" +
		"Set MXLRC_MASTER_KEY to this value in your container/Unraid template (or a\n" +
		".env kept outside the data volume) and restart. Store it safely: it will not\n" +
		"be shown again, and losing it makes the encrypted secrets unrecoverable.\n" +
		"The daemon will not start until MXLRC_MASTER_KEY is set."
}

// ResolveKey returns the 32-byte AES-256 master key.
//
// Resolution order (first present wins):
//  1. MXLRC_MASTER_KEY (MasterKeyB64): base64 of exactly 32 bytes. A malformed
//     value (bad base64 or wrong length) is a loud, fatal error - never a
//     silent fallback to no encryption.
//  2. Docker mode with no master key: returns *FirstRunError so the caller can
//     print the onboarding hint and exit. A colocated key file is never
//     auto-created in /config.
//  3. Native key file: read it (warning on loose perms), or auto-generate a
//     0600 file on first use. An unreadable/unwritable/malformed key file is a
//     loud, fatal error.
func ResolveKey(opts KeyOptions) ([]byte, error) {
	if opts.MasterKeyB64 != "" {
		key, err := decodeMasterKey(opts.MasterKeyB64)
		if err != nil {
			return nil, err
		}
		return key, nil
	}

	if opts.DockerMode {
		suggested, err := GenerateKey()
		if err != nil {
			return nil, err
		}
		return nil, &FirstRunError{SuggestedKeyB64: base64.StdEncoding.EncodeToString(suggested)}
	}

	return loadOrCreateKeyFile(opts)
}

// decodeMasterKey decodes a base64-encoded 32-byte key, failing loudly on bad
// encoding or wrong length.
func decodeMasterKey(b64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("secrets: MXLRC_MASTER_KEY is not valid base64: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("secrets: MXLRC_MASTER_KEY must decode to %d bytes, got %d", KeySize, len(key))
	}
	return key, nil
}

// loadOrCreateKeyFile reads the 32-byte key file, or creates it 0600 on first
// use. Loose permissions on an existing file warn (non-fatal); any other I/O or
// length problem is fatal.
func loadOrCreateKeyFile(opts KeyOptions) ([]byte, error) {
	if opts.KeyFilePath == "" {
		return nil, errors.New("secrets: key file path must not be empty")
	}

	info, err := os.Stat(opts.KeyFilePath)
	switch {
	case err == nil:
		if info.Mode().Perm()&0o077 != 0 {
			opts.logger().Warn("secrets: key file has loose permissions",
				"path", opts.KeyFilePath, "mode", info.Mode().Perm().String())
		}
		key, err := os.ReadFile(opts.KeyFilePath) //nolint:gosec // path is operator-configured, not attacker-controlled
		if err != nil {
			return nil, fmt.Errorf("secrets: read key file %s: %w", opts.KeyFilePath, err)
		}
		if len(key) != KeySize {
			return nil, fmt.Errorf("secrets: key file %s must contain %d bytes, got %d", opts.KeyFilePath, KeySize, len(key))
		}
		return key, nil
	case errors.Is(err, os.ErrNotExist):
		return createKeyFile(opts.KeyFilePath)
	default:
		return nil, fmt.Errorf("secrets: stat key file %s: %w", opts.KeyFilePath, err)
	}
}

// createKeyFile generates a fresh 32-byte key and writes it 0600.
func createKeyFile(path string) ([]byte, error) {
	key, err := GenerateKey()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("secrets: write key file %s: %w", path, err)
	}
	return key, nil
}
