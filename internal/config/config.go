package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
)

// Config holds all application configuration.
type Config struct {
	API    APIConfig    `toml:"api"`
	Output OutputConfig `toml:"output"`
	DB     DBConfig     `toml:"db"`
}

// APIConfig holds API-related configuration.
type APIConfig struct {
	Token    string `toml:"token"`
	Cooldown int    `toml:"cooldown"`
}

// OutputConfig holds output-related configuration.
type OutputConfig struct {
	Dir string `toml:"dir"`
}

// DBConfig holds database configuration.
type DBConfig struct {
	Path string `toml:"path"`
}

// defaults sets built-in fallback values.
func defaults() Config {
	return Config{
		API:    APIConfig{Cooldown: 15},
		Output: OutputConfig{Dir: "lyrics"},
		DB:     DBConfig{Path: xdgDataPath("mxlrcsvc-go", "mxlrcsvc.db")},
	}
}

// Load reads the TOML config file at path (or XDG default if empty),
// then overlays environment variables. A missing config file is not an error.
func Load(path string) (Config, error) {
	cfg := defaults()
	if path == "" {
		path = xdgConfigPath("mxlrcsvc-go", "config.toml")
	}
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, &cfg); err != nil {
				return cfg, fmt.Errorf("config: decode %s: %w", path, err)
			}
			// Re-apply defaults for any fields the file set to blank.
			// This prevents a user copying config.example.toml verbatim from
			// clobbering computed defaults (e.g. XDG DB path, output dir).
			// Re-apply defaults for string fields that must not be blank.
			// Cooldown is intentionally excluded: 0 is a valid user-specified value.
			d := defaults()
			if cfg.DB.Path == "" {
				cfg.DB.Path = d.DB.Path
			}
			if cfg.Output.Dir == "" {
				cfg.Output.Dir = d.Output.Dir
			}
		} else if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("config: stat %s: %w", path, err)
		}
	}
	applyEnvOverrides(&cfg)
	if cfg.DB.Path == "" {
		return cfg, fmt.Errorf("config: cannot determine DB path: set MXLRC_DB_PATH or XDG_DATA_HOME")
	}
	return cfg, nil
}

// applyEnvOverrides overlays environment variables onto cfg.
// Token precedence within env vars: MUSIXMATCH_TOKEN > MXLRC_API_TOKEN.
// Cooldown precedence: MXLRC_API_COOLDOWN > MXLRC_COOLDOWN.
// Supported: MUSIXMATCH_TOKEN, MXLRC_API_TOKEN, MXLRC_API_COOLDOWN, MXLRC_COOLDOWN, MXLRC_OUTPUT_DIR, MXLRC_DB_PATH
func applyEnvOverrides(cfg *Config) {
	// Token: MUSIXMATCH_TOKEN takes precedence over MXLRC_API_TOKEN (backward compat).
	if v := os.Getenv("MUSIXMATCH_TOKEN"); v != "" {
		cfg.API.Token = v
	} else if v := os.Getenv("MXLRC_API_TOKEN"); v != "" {
		cfg.API.Token = v
	}

	// Cooldown: MXLRC_API_COOLDOWN (section-scoped) takes precedence over MXLRC_COOLDOWN (short alias).
	cooldownVar := "MXLRC_API_COOLDOWN"
	v := os.Getenv(cooldownVar)
	if v == "" {
		cooldownVar = "MXLRC_COOLDOWN"
		v = os.Getenv(cooldownVar)
	}
	if v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			slog.Warn("env var is invalid; using current value", "var", cooldownVar, "value", v, "current", cfg.API.Cooldown) //nolint:gosec // G706: env var value echoed to structured log field; no format-string injection risk
		} else {
			cfg.API.Cooldown = n
		}
	}

	if v := os.Getenv("MXLRC_OUTPUT_DIR"); v != "" {
		cfg.Output.Dir = v
	}
	if v := os.Getenv("MXLRC_DB_PATH"); v != "" {
		cfg.DB.Path = v
	}
}

// xdgConfigPath returns the XDG config path for the given app and file.
// Returns "" if the home directory cannot be determined.
func xdgConfigPath(app, file string) string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// Fall back to /config (Docker convention) only when running inside Docker.
			if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
				return filepath.Join("/config", file)
			}
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, app, file)
}

// xdgDataPath returns the XDG data path for the given app and file.
// Returns "" if the home directory cannot be determined and not running in Docker.
func xdgDataPath(app, file string) string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// Fall back to /config (Docker convention) only when running inside Docker.
			if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
				return filepath.Join("/config", file)
			}
			return ""
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, app, file)
}
