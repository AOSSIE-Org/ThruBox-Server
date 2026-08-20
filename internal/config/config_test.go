package config

import "testing"

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
