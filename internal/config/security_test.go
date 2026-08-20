package config

import (
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
