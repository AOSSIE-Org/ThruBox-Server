package config

import (
	"os"
	"path/filepath"
	"testing"
)

// clearEnvOverrides neutralises the environment so a test observes only the
// file and the defaults. Empty is equivalent to unset for applyEnvOverrides,
// and t.Setenv restores whatever the developer's shell had.
func clearEnvOverrides(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"RELAY_SERVER_PORT",
		"PORT",
		"RELAY_SERVER_HOST",
		"RELAY_STORAGE_DRIVER",
		"RELAY_STORAGE_PATH",
		"RELAY_MESSAGES_TTL_DAYS",
		"RELAY_MESSAGES_MAX_PAYLOAD_SIZE",
		"RELAY_SECURITY_RATE_LIMIT",
		"RELAY_SECURITY_API_KEY",
	} {
		t.Setenv(k, "")
	}
}

// writeConfig drops a YAML file in a temp dir and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}

// TestLoad_ReadsFileAtGivenPath is the regression test for the bug: a config
// file sitting somewhere other than ./config.yaml must actually be applied.
func TestLoad_ReadsFileAtGivenPath(t *testing.T) {
	clearEnvOverrides(t)

	path := writeConfig(t, "server:\n  port: 8123\nmessages:\n  ttl_days: 42\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 8123 {
		t.Errorf("Server.Port = %d, want 8123 (file value ignored)", cfg.Server.Port)
	}
	if cfg.Messages.TTLDays != 42 {
		t.Errorf("Messages.TTLDays = %d, want 42 (file value ignored)", cfg.Messages.TTLDays)
	}
	if cfg.Source != path {
		t.Errorf("Source = %q, want %q", cfg.Source, path)
	}
}

// TestLoad_MissingFileUsesDefaults keeps the documented behaviour: a missing
// file is not an error, it just means defaults. Source must be empty so the
// caller can say so out loud.
func TestLoad_MissingFileUsesDefaults(t *testing.T) {
	clearEnvOverrides(t)

	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
	if cfg.Source != "" {
		t.Errorf("Source = %q, want empty string for a missing file", cfg.Source)
	}
}

// TestLoad_MissingFileStillAppliesEnvOverrides covers the second, easy-to-miss
// applyEnvOverrides call site. Load has one in the missing-file branch and one
// after a successful parse, so "no file, but env vars set" is its own path --
// and it is the normal case for a container started with no config mounted.
func TestLoad_MissingFileStillAppliesEnvOverrides(t *testing.T) {
	clearEnvOverrides(t)

	t.Setenv("RELAY_SERVER_PORT", "8123")
	t.Setenv("RELAY_SECURITY_API_KEY", "from-env")

	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.Server.Port != 8123 {
		t.Errorf("Server.Port = %d, want 8123 (env ignored on the missing-file path)", cfg.Server.Port)
	}
	if cfg.Security.APIKey != "from-env" {
		t.Errorf("Security.APIKey = %q, want \"from-env\"", cfg.Security.APIKey)
	}
	if cfg.Source != "" {
		t.Errorf("Source = %q, want empty string for a missing file", cfg.Source)
	}
}

// TestLoad_EnvOverridesFile pins the precedence order:
// defaults -> file -> environment.
func TestLoad_EnvOverridesFile(t *testing.T) {
	clearEnvOverrides(t)

	path := writeConfig(t, "server:\n  port: 8123\nsecurity:\n  api_key: \"from-file\"\n")

	t.Setenv("RELAY_SERVER_PORT", "9999")
	t.Setenv("RELAY_SECURITY_API_KEY", "from-env")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("Server.Port = %d, want 9999 (env must beat file)", cfg.Server.Port)
	}
	if cfg.Security.APIKey != "from-env" {
		t.Errorf("Security.APIKey = %q, want \"from-env\"", cfg.Security.APIKey)
	}
	if cfg.Source != path {
		t.Errorf("Source = %q, want %q", cfg.Source, path)
	}
}

// TestLoad_PartialFileKeepsDefaults confirms unset keys are not zeroed.
func TestLoad_PartialFileKeepsDefaults(t *testing.T) {
	clearEnvOverrides(t)

	path := writeConfig(t, "server:\n  port: 8123\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want \"0.0.0.0\"", cfg.Server.Host)
	}
	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("Storage.Driver = %q, want \"sqlite\"", cfg.Storage.Driver)
	}
}

// TestLoad_InvalidYAMLIsAnError distinguishes "malformed" from "absent".
func TestLoad_InvalidYAMLIsAnError(t *testing.T) {
	clearEnvOverrides(t)

	path := writeConfig(t, "server:\n\tport: [unbalanced\n")

	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want a parse error for malformed YAML")
	}
}

// TestLoad_InvalidValueIsAnError checks Validate still runs on file input.
func TestLoad_InvalidValueIsAnError(t *testing.T) {
	clearEnvOverrides(t)

	path := writeConfig(t, "server:\n  port: 70000\n")

	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want a validation error for port 70000")
	}
}
