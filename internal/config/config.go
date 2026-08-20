package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the relay server.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	Messages MessageConfig  `yaml:"messages"`
	Security SecurityConfig `yaml:"security"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// StorageConfig holds database settings.
type StorageConfig struct {
	Driver string `yaml:"driver"`
	Path   string `yaml:"path"`
}

// MessageConfig holds message-related settings.
type MessageConfig struct {
	TTLDays        int `yaml:"ttl_days"`
	MaxPayloadSize int `yaml:"max_payload_size"`
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	RateLimit int    `yaml:"rate_limit"`
	APIKey    string `yaml:"api_key"`
	// AllowedOrigins is the CORS allowlist. Empty (the default) serves no
	// CORS headers at all. "*" allows any origin.
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// Default returns a Config with sensible defaults.
// Used as the base — YAML file and env vars override these.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 3000,
			Host: "0.0.0.0",
		},
		Storage: StorageConfig{
			Driver: "sqlite",
			Path:   "./data/relay.db",
		},
		Messages: MessageConfig{
			TTLDays:        7,
			MaxPayloadSize: 524288, // 500KB
		},
		Security: SecurityConfig{
			RateLimit: 30,
			APIKey:    "",
			// No origins: CORS stays off unless explicitly configured.
			AllowedOrigins: nil,
		},
	}
}

// Load reads configuration from a YAML file at the given path,
// then applies environment variable overrides.
// If the file does not exist, defaults are used.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file — use defaults + env overrides
			applyEnvOverrides(cfg)
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("invalid default configuration: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

// applyEnvOverrides checks for environment variables and overrides
// the corresponding config values if set.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("RELAY_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		} else {
			log.Printf("warning: invalid RELAY_SERVER_PORT=%q, using default", v)
		}
	}

	if v := os.Getenv("RELAY_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}

	if v := os.Getenv("RELAY_STORAGE_DRIVER"); v != "" {
		cfg.Storage.Driver = v
	}

	if v := os.Getenv("RELAY_STORAGE_PATH"); v != "" {
		cfg.Storage.Path = v
	}

	if v := os.Getenv("RELAY_MESSAGES_TTL_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil {
			cfg.Messages.TTLDays = days
		} else {
			log.Printf("warning: invalid RELAY_MESSAGES_TTL_DAYS=%q, using default", v)
		}
	}

	if v := os.Getenv("RELAY_MESSAGES_MAX_PAYLOAD_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.Messages.MaxPayloadSize = size
		} else {
			log.Printf("warning: invalid RELAY_MESSAGES_MAX_PAYLOAD_SIZE=%q, using default", v)
		}
	}

	if v := os.Getenv("RELAY_SECURITY_RATE_LIMIT"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil {
			cfg.Security.RateLimit = limit
		} else {
			log.Printf("warning: invalid RELAY_SECURITY_RATE_LIMIT=%q, using default", v)
		}
	}

	if v := os.Getenv("RELAY_SECURITY_API_KEY"); v != "" {
		cfg.Security.APIKey = v
	}

	if v := os.Getenv("RELAY_SECURITY_ALLOWED_ORIGINS"); v != "" {
		cfg.Security.AllowedOrigins = splitOrigins(v)
	}
}

// splitOrigins parses a comma-separated origin list from an environment
// variable, trimming spaces and dropping empty entries so that values like
// "https://a.example, https://b.example," behave as expected.
func splitOrigins(v string) []string {
	parts := strings.Split(v, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}

// Validate checks the configuration values for validity.
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Messages.TTLDays < 0 {
		return fmt.Errorf("invalid ttl_days: %d", c.Messages.TTLDays)
	}
	if c.Messages.MaxPayloadSize < 0 {
		return fmt.Errorf("invalid max_payload_size: %d", c.Messages.MaxPayloadSize)
	}
	if c.Security.RateLimit < 0 {
		return fmt.Errorf("invalid rate_limit: %d", c.Security.RateLimit)
	}
	if err := validateAllowedOrigins(c.Security.AllowedOrigins); err != nil {
		return err
	}
	return nil
}

// validateAllowedOrigins rejects configurations that would not do what the
// operator expects: a wildcard mixed with specific origins (the wildcard
// silently wins, making the list misleading), and entries that are not
// scheme-qualified origins, which can never match a browser Origin header.
func validateAllowedOrigins(origins []string) error {
	hasWildcard, hasSpecific := false, false

	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			hasWildcard = true
			continue
		}
		hasSpecific = true
		if !strings.Contains(o, "://") {
			return fmt.Errorf(
				"invalid allowed_origins entry %q: must be a full origin such as https://app.example.com", o)
		}
		if strings.Contains(strings.TrimSuffix(o, "/"), "/") {
			after := strings.SplitN(o, "://", 2)[1]
			if strings.Contains(strings.TrimSuffix(after, "/"), "/") {
				return fmt.Errorf(
					"invalid allowed_origins entry %q: an origin has no path component", o)
			}
		}
	}

	if hasWildcard && hasSpecific {
		return fmt.Errorf(`invalid allowed_origins: "*" cannot be combined with specific origins`)
	}
	return nil
}

// Addr returns the listen address as "host:port".
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
