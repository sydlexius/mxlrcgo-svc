package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_BilingualOutputDefaultsFalse verifies bilingual_output defaults to
// false when neither TOML nor env sets it.
func TestLoad_BilingualOutputDefaultsFalse(t *testing.T) {
	isolateEnv(t)

	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.BilingualOutput {
		t.Error("Output.BilingualOutput = true; want default false")
	}
}

// TestLoad_BilingualOutputFromTOML verifies an explicit bilingual_output = true
// in the [output] section is decoded.
func TestLoad_BilingualOutputFromTOML(t *testing.T) {
	isolateEnv(t)

	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgFile, []byte("[output]\nbilingual_output = true\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Output.BilingualOutput {
		t.Error("Output.BilingualOutput = false; want true from TOML")
	}
}

// TestLoad_BilingualOutputExplicitFalsePreserved verifies an explicit
// bilingual_output = false in TOML is preserved (the IsDefined seam), not
// silently re-defaulted.
func TestLoad_BilingualOutputExplicitFalsePreserved(t *testing.T) {
	isolateEnv(t)

	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgFile, []byte("[output]\nbilingual_output = false\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.BilingualOutput {
		t.Error("Output.BilingualOutput = true; want explicit false from TOML")
	}
}

// TestLoad_BilingualOutputEnvOverride verifies MXLRC_BILINGUAL_OUTPUT overrides
// the TOML value (true via env beats false in file).
func TestLoad_BilingualOutputEnvOverride(t *testing.T) {
	isolateEnv(t)

	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgFile, []byte("[output]\nbilingual_output = false\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("MXLRC_BILINGUAL_OUTPUT", "true")

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Output.BilingualOutput {
		t.Error("Output.BilingualOutput = false; want true (env override)")
	}
}

// TestLoad_BilingualOutputEnvFalseOverride verifies the env var can also force
// false over a TOML true.
func TestLoad_BilingualOutputEnvFalseOverride(t *testing.T) {
	isolateEnv(t)

	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgFile, []byte("[output]\nbilingual_output = true\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("MXLRC_BILINGUAL_OUTPUT", "false")

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.BilingualOutput {
		t.Error("Output.BilingualOutput = true; want false (env override)")
	}
}

// TestLoad_BilingualOutputEnvInvalidIgnored verifies a non-boolean env value is
// ignored, leaving the TOML/default value intact.
func TestLoad_BilingualOutputEnvInvalidIgnored(t *testing.T) {
	isolateEnv(t)

	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgFile, []byte("[output]\nbilingual_output = true\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("MXLRC_BILINGUAL_OUTPUT", "notabool")

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Output.BilingualOutput {
		t.Error("Output.BilingualOutput = false; want true preserved after invalid env")
	}
}
