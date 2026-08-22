package config

import (
	"path/filepath"
	"testing"
)

// TestApplyEnvOverrides_Port covers the precedence rules between the
// project-specific RELAY_SERVER_PORT and the platform-conventional PORT.
func TestApplyEnvOverrides_Port(t *testing.T) {
	tests := []struct {
		name            string
		relayServerPort string
		port            string
		want            int
	}{
		{
			name: "neither set falls back to the default",
			want: 3000,
		},
		{
			name: "PORT alone is honoured",
			port: "8080",
			want: 8080,
		},
		{
			name:            "RELAY_SERVER_PORT alone is honoured",
			relayServerPort: "9090",
			want:            9090,
		},
		{
			name:            "RELAY_SERVER_PORT wins when both are set",
			relayServerPort: "3000",
			port:            "8080",
			want:            3000,
		},
		{
			name: "invalid PORT falls back to the default",
			port: "not-a-port",
			want: 3000,
		},
		{
			name:            "invalid RELAY_SERVER_PORT does not fall through to PORT",
			relayServerPort: "not-a-port",
			port:            "8080",
			want:            3000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Empty is equivalent to unset for the override logic, and
			// t.Setenv restores whatever the developer's shell had.
			t.Setenv("RELAY_SERVER_PORT", tt.relayServerPort)
			t.Setenv("PORT", tt.port)

			cfg := Default()
			applyEnvOverrides(cfg)

			if cfg.Server.Port != tt.want {
				t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, tt.want)
			}
		})
	}
}

// TestApplyEnvOverrides_PortDoesNotDisturbOtherSettings guards against the
// port resolution accidentally clobbering neighbouring config values.
func TestApplyEnvOverrides_PortDoesNotDisturbOtherSettings(t *testing.T) {
	t.Setenv("RELAY_SERVER_PORT", "")
	t.Setenv("PORT", "8080")

	cfg := Default()
	applyEnvOverrides(cfg)

	if got, want := cfg.Server.Host, "0.0.0.0"; got != want {
		t.Errorf("Server.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Storage.Path, "./data/relay.db"; got != want {
		t.Errorf("Storage.Path = %q, want %q", got, want)
	}
}

// TestConfigAddr checks the listen address built from the resolved port.
func TestConfigAddr(t *testing.T) {
	t.Setenv("RELAY_SERVER_PORT", "")
	t.Setenv("PORT", "8080")

	cfg := Default()
	applyEnvOverrides(cfg)

	if got, want := cfg.Addr(), "0.0.0.0:8080"; got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
}

// TestLoad_PortOutOfRangeIsRejected pins a deliberate asymmetry: a port value
// that does not parse is a warning and falls back to the default, but a value
// that parses and is out of the valid TCP range fails startup via Validate.
//
// The distinction is intentional. "abc" reads as an unset or placeholder
// variable, so continuing is reasonable. 0, -1 or 65536 is a real number that
// someone meant, and quietly serving on 3000 instead would leave a deployment
// answering on a port nobody configured -- exactly the confusing failure this
// change set out to remove. Validate() has enforced this for RELAY_SERVER_PORT
// since before the PORT fallback existed; the fallback inherits it unchanged.
func TestLoad_PortOutOfRangeIsRejected(t *testing.T) {
	tests := []struct {
		name            string
		relayServerPort string
		port            string
		wantErr         bool
		wantPort        int
	}{
		{name: "RELAY_SERVER_PORT=0", relayServerPort: "0", wantErr: true},
		{name: "RELAY_SERVER_PORT=-1", relayServerPort: "-1", wantErr: true},
		{name: "RELAY_SERVER_PORT=65536", relayServerPort: "65536", wantErr: true},
		{name: "PORT=0", port: "0", wantErr: true},
		{name: "PORT=-1", port: "-1", wantErr: true},
		{name: "PORT=65536", port: "65536", wantErr: true},

		{name: "PORT=1 is the low boundary", port: "1", wantPort: 1},
		{name: "PORT=65535 is the high boundary", port: "65535", wantPort: 65535},

		// Unparseable is a warning, not a failure -- unchanged behaviour.
		{name: "PORT=abc warns and uses the default", port: "abc", wantPort: 3000},
		{name: "RELAY_SERVER_PORT=abc warns and uses the default", relayServerPort: "abc", wantPort: 3000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RELAY_SERVER_PORT", tt.relayServerPort)
			t.Setenv("PORT", tt.port)

			// A path that does not exist exercises defaults + env + Validate.
			cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() error = nil, want a validation error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if cfg.Server.Port != tt.wantPort {
				t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, tt.wantPort)
			}
		})
	}
}
