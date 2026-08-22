package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the relay server.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	Messages MessageConfig  `yaml:"messages"`
	Security SecurityConfig `yaml:"security"`

	// Source is the path of the YAML file this config was read from, or the
	// empty string when no file was found and the built-in defaults are in
	// use. It is populated by Load and is never read from the YAML itself.
	Source string `yaml:"-"`
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
			// No config file — use defaults + env overrides.
			// Source stays empty so the caller can report that fact.
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

	cfg.Source = path
	applyEnvOverrides(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

// applyEnvOverrides checks for environment variables and overrides
// the corresponding config values if set.
func applyEnvOverrides(cfg *Config) {
	// Port resolution: RELAY_SERVER_PORT is authoritative. PORT is only a
	// fallback, for managed platforms (Render, Railway, Heroku, Cloud Run)
	// that inject it and expect the process to bind to it.
	portVar, portVal := "RELAY_SERVER_PORT", os.Getenv("RELAY_SERVER_PORT")
	if portVal == "" {
		portVar, portVal = "PORT", os.Getenv("PORT")
	}
	if portVal != "" {
		if port, err := strconv.Atoi(portVal); err == nil {
			cfg.Server.Port = port
		} else {
			log.Printf("warning: invalid %s=%q, using default", portVar, portVal)
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
	return nil
}

// Addr returns the listen address as "host:port".
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
