package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetOriginEnv neutralises the CORS-related environment so a test observes
// only what it sets. Empty is equivalent to unset for applyEnvOverrides, and
// t.Setenv restores whatever the developer's shell had.
func resetOriginEnv(t *testing.T) {
	t.Helper()
	t.Setenv("RELAY_SECURITY_ALLOWED_ORIGINS", "")
}

func TestAllowedOrigins_DefaultIsEmpty(t *testing.T) {
	resetOriginEnv(t)

	cfg := Default()
	applyEnvOverrides(cfg)

	if len(cfg.Security.AllowedOrigins) != 0 {
		t.Errorf("AllowedOrigins = %v, want empty so CORS stays off by default",
			cfg.Security.AllowedOrigins)
	}
}

func TestAllowedOrigins_EnvOverride(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{
			name: "single origin",
			env:  "https://app.example.com",
			want: []string{"https://app.example.com"},
		},
		{
			name: "comma separated",
			env:  "https://a.example.com,https://b.example.com",
			want: []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name: "spaces around entries are trimmed",
			env:  " https://a.example.com , https://b.example.com ",
			want: []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name: "trailing comma produces no empty entry",
			env:  "https://a.example.com,",
			want: []string{"https://a.example.com"},
		},
		{
			name: "wildcard",
			env:  "*",
			want: []string{"*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetOriginEnv(t)
			t.Setenv("RELAY_SECURITY_ALLOWED_ORIGINS", tt.env)

			cfg := Default()
			applyEnvOverrides(cfg)

			got := cfg.Security.AllowedOrigins
			if len(got) != len(tt.want) {
				t.Fatalf("AllowedOrigins = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("AllowedOrigins[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateAllowedOrigins(t *testing.T) {
	tests := []struct {
		name    string
		origins []string
		wantErr string // substring; empty means the config must be accepted
	}{
		{name: "nil is valid", origins: nil},
		{name: "empty slice is valid", origins: []string{}},
		{name: "wildcard alone is valid", origins: []string{"*"}},
		{
			name:    "specific origins are valid",
			origins: []string{"https://a.example.com", "http://localhost:5173"},
		},
		{
			name:    "wildcard mixed with a specific origin is rejected",
			origins: []string{"*", "https://a.example.com"},
			wantErr: "cannot be combined",
		},
		{
			name:    "a bare hostname is rejected",
			origins: []string{"app.example.com"},
			wantErr: "must be a full origin",
		},
		{
			name:    "an origin with a path is rejected",
			origins: []string{"https://app.example.com/api"},
			wantErr: "no path component",
		},
		{
			name:    "an empty scheme is rejected",
			origins: []string{"://app.example.com"},
			wantErr: "must be a full origin",
		},
		{
			name:    "an empty host is rejected",
			origins: []string{"https://"},
			wantErr: "missing host",
		},
		{
			name:    "a scheme-only entry with a slash is rejected",
			origins: []string{"https:///"},
			wantErr: "missing host",
		},
		{
			name:    "a lone separator is rejected",
			origins: []string{"://"},
			wantErr: "must be a full origin",
		},
		{
			name:    "a trailing slash is tolerated",
			origins: []string{"https://app.example.com/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Security.AllowedOrigins = tt.origins

			err := cfg.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestAllowedOrigins_InvalidEnvIsRejectedByLoad makes sure a bad env value
// fails startup loudly rather than silently producing an allowlist that can
// never match.
func TestAllowedOrigins_InvalidEnvIsRejectedByLoad(t *testing.T) {
	resetOriginEnv(t)
	t.Setenv("RELAY_SECURITY_ALLOWED_ORIGINS", "app.example.com")

	if _, err := Load("does-not-exist.yaml"); err == nil {
		t.Fatal("Load() error = nil, want a validation error for a scheme-less origin")
	}
}

// writeOriginConfig drops a YAML file in a temp dir and returns its path.
// Named distinctly from the helpers in the other config test files so the
// package still compiles once those land alongside this one.
func writeOriginConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}

// TestAllowedOrigins_FromYAML covers the documented primary route. The
// environment variable is the fallback; config.yaml is what the README and the
// shipped config actually show, so it needs coverage of its own.
func TestAllowedOrigins_FromYAML(t *testing.T) {
	resetOriginEnv(t)

	path := writeOriginConfig(t, `
security:
  allowed_origins:
    - "https://a.example.com"
    - "https://b.example.com"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []string{"https://a.example.com", "https://b.example.com"}
	if len(cfg.Security.AllowedOrigins) != len(want) {
		t.Fatalf("AllowedOrigins = %v, want %v", cfg.Security.AllowedOrigins, want)
	}
	for i := range want {
		if cfg.Security.AllowedOrigins[i] != want[i] {
			t.Errorf("AllowedOrigins[%d] = %q, want %q", i, cfg.Security.AllowedOrigins[i], want[i])
		}
	}
}

// TestAllowedOrigins_EnvReplacesYAMLList guards a specific footgun: the
// environment override must REPLACE the YAML list, not append to it.
// Unmarshalling into a pre-populated struct makes slice merging an easy
// accident, and appending would silently keep serving an origin the operator
// thought they had removed.
func TestAllowedOrigins_EnvReplacesYAMLList(t *testing.T) {
	resetOriginEnv(t)

	path := writeOriginConfig(t, `
security:
  allowed_origins:
    - "https://from-file.example.com"
`)
	t.Setenv("RELAY_SECURITY_ALLOWED_ORIGINS", "https://from-env.example.com")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Security.AllowedOrigins) != 1 {
		t.Fatalf("AllowedOrigins = %v, want exactly the env value (env must replace, not append)",
			cfg.Security.AllowedOrigins)
	}
	if got, want := cfg.Security.AllowedOrigins[0], "https://from-env.example.com"; got != want {
		t.Errorf("AllowedOrigins[0] = %q, want %q", got, want)
	}
}

// TestAllowedOrigins_YAMLDefaultsToEmpty confirms a config file that says
// nothing about CORS leaves it switched off.
func TestAllowedOrigins_YAMLDefaultsToEmpty(t *testing.T) {
	resetOriginEnv(t)

	path := writeOriginConfig(t, "server:\n  port: 3000\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Security.AllowedOrigins) != 0 {
		t.Errorf("AllowedOrigins = %v, want empty", cfg.Security.AllowedOrigins)
	}
}

// TestAllowedOrigins_MalformedYAMLEntryFailsStartup pairs with the Validate
// table: a bad entry in the file must fail Load, not just Validate in isolation.
func TestAllowedOrigins_MalformedYAMLEntryFailsStartup(t *testing.T) {
	resetOriginEnv(t)

	path := writeOriginConfig(t, `
security:
  allowed_origins:
    - "https://"
`)

	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want a validation error for an origin with no host")
	}
}
