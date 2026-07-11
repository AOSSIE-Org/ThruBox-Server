package config

import (
	"fmt"
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
			// No config file — use defaults + env overrides
			applyEnvOverrides(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

// applyEnvOverrides checks for environment variables and overrides
// the corresponding config values if set.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("RELAY_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
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
		}
	}

	if v := os.Getenv("RELAY_MESSAGES_MAX_PAYLOAD_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.Messages.MaxPayloadSize = size
		}
	}

	if v := os.Getenv("RELAY_SECURITY_RATE_LIMIT"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil {
			cfg.Security.RateLimit = limit
		}
	}

	if v := os.Getenv("RELAY_SECURITY_API_KEY"); v != "" {
		cfg.Security.APIKey = v
	}
}

// Addr returns the listen address as "host:port".
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
