package config

import (
	"os"
	"path/filepath"
	"testing"
)

// isolateEnv clears all config-related env vars using t.Setenv (which enforces
// sequential-only execution -- calling t.Parallel() in a test that uses this
// helper will panic, making the constraint machine-enforceable).
// It also sets MXLRC_DB_PATH to a safe sentinel so Load never returns an
// "empty DB path" error in tests that don't care about the DB path.
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"MUSIXMATCH_TOKEN", "MXLRC_API_TOKEN",
		"MXLRC_API_COOLDOWN", "MXLRC_COOLDOWN",
		"MXLRC_OUTPUT_DIR",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME",
	} {
		// t.Setenv("", "") clears the variable; applyEnvOverrides uses os.Getenv
		// which returns "" for both unset and empty, so behavior is identical.
		t.Setenv(k, "")
	}
	// Provide a non-empty DB path so Load doesn't error on path resolution.
	t.Setenv("MXLRC_DB_PATH", filepath.Join(t.TempDir(), "test.db"))
}

// TestLoad_MissingConfigFileIsNotFatal verifies that a non-existent config
// file path is silently ignored and defaults are returned.
func TestLoad_MissingConfigFileIsNotFatal(t *testing.T) {
	isolateEnv(t)

	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatalf("Load with missing file returned error: %v", err)
	}
	if cfg.API.Cooldown != 15 {
		t.Errorf("default cooldown = %d; want 15", cfg.API.Cooldown)
	}
	if cfg.Output.Dir != "lyrics" {
		t.Errorf("default output dir = %q; want %q", cfg.Output.Dir, "lyrics")
	}
}

// TestLoad_BlankFieldsInTOMLDoNotClobberDefaults verifies that a TOML file
// with blank string fields re-applies the built-in defaults rather than
// leaving paths empty.
func TestLoad_BlankFieldsInTOMLDoNotClobberDefaults(t *testing.T) {
	isolateEnv(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.toml")
	content := "[db]\npath = \"\"\n[output]\ndir = \"\"\n"
	if err := os.WriteFile(cfgFile, []byte(content), 0600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	// Override MXLRC_DB_PATH so the XDG default path is predictable.
	dbPath := filepath.Join(dir, "data.db")
	t.Setenv("MXLRC_DB_PATH", dbPath)

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Output.Dir != "lyrics" {
		t.Errorf("blank output.dir not re-defaulted; got %q, want %q", cfg.Output.Dir, "lyrics")
	}
	// DB path comes from MXLRC_DB_PATH env override (which wins over re-default).
	if cfg.DB.Path != dbPath {
		t.Errorf("DB.Path = %q; want %q", cfg.DB.Path, dbPath)
	}
}

// TestLoad_EnvTokenPrecedence verifies MUSIXMATCH_TOKEN > MXLRC_API_TOKEN.
func TestLoad_EnvTokenPrecedence(t *testing.T) {
	isolateEnv(t)
	t.Setenv("MUSIXMATCH_TOKEN", "token-musixmatch")
	t.Setenv("MXLRC_API_TOKEN", "token-mxlrc")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.API.Token != "token-musixmatch" {
		t.Errorf("token = %q; want %q (MUSIXMATCH_TOKEN must win)", cfg.API.Token, "token-musixmatch")
	}
}

// TestLoad_EnvTokenFallback verifies MXLRC_API_TOKEN is used when
// MUSIXMATCH_TOKEN is absent.
func TestLoad_EnvTokenFallback(t *testing.T) {
	isolateEnv(t)
	// MUSIXMATCH_TOKEN is already cleared by isolateEnv.
	t.Setenv("MXLRC_API_TOKEN", "token-mxlrc")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.API.Token != "token-mxlrc" {
		t.Errorf("token = %q; want %q", cfg.API.Token, "token-mxlrc")
	}
}

// TestLoad_EnvCooldownPrecedence verifies MXLRC_API_COOLDOWN > MXLRC_COOLDOWN.
func TestLoad_EnvCooldownPrecedence(t *testing.T) {
	isolateEnv(t)
	t.Setenv("MXLRC_API_COOLDOWN", "30")
	t.Setenv("MXLRC_COOLDOWN", "99")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.API.Cooldown != 30 {
		t.Errorf("cooldown = %d; want 30 (MXLRC_API_COOLDOWN must win)", cfg.API.Cooldown)
	}
}

// TestLoad_EnvCooldownFallback verifies MXLRC_COOLDOWN is used when
// MXLRC_API_COOLDOWN is absent.
func TestLoad_EnvCooldownFallback(t *testing.T) {
	isolateEnv(t)
	// MXLRC_API_COOLDOWN is already cleared by isolateEnv.
	t.Setenv("MXLRC_COOLDOWN", "42")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.API.Cooldown != 42 {
		t.Errorf("cooldown = %d; want 42", cfg.API.Cooldown)
	}
}

// TestLoad_EnvCooldownZeroIsValid verifies that cooldown=0 is accepted (not
// treated as "unset" or invalid).
func TestLoad_EnvCooldownZeroIsValid(t *testing.T) {
	isolateEnv(t)
	t.Setenv("MXLRC_API_COOLDOWN", "0")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.API.Cooldown != 0 {
		t.Errorf("cooldown = %d; want 0", cfg.API.Cooldown)
	}
}

// TestLoad_EnvCooldownInvalidIsIgnored verifies that a non-numeric cooldown
// env var falls back to the current value rather than crashing.
func TestLoad_EnvCooldownInvalidIsIgnored(t *testing.T) {
	isolateEnv(t)
	t.Setenv("MXLRC_API_COOLDOWN", "notanumber")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.API.Cooldown != 15 {
		t.Errorf("cooldown = %d; want 15 (default) after invalid env var", cfg.API.Cooldown)
	}
}

// TestLoad_DBPathFromEnv verifies MXLRC_DB_PATH overrides the computed default.
func TestLoad_DBPathFromEnv(t *testing.T) {
	isolateEnv(t)
	want := filepath.Join(t.TempDir(), "custom.db")
	t.Setenv("MXLRC_DB_PATH", want)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DB.Path != want {
		t.Errorf("DB.Path = %q; want %q", cfg.DB.Path, want)
	}
}

// TestLoad_InvalidTOMLReturnsError verifies that a malformed TOML file is
// reported as an error.
func TestLoad_InvalidTOMLReturnsError(t *testing.T) {
	isolateEnv(t)

	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgFile, []byte("not valid toml ]["), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Load(cfgFile)
	if err == nil {
		t.Fatal("Load with invalid TOML returned nil error; want an error")
	}
}
