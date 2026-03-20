package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds application configuration.
type Config struct {
	DefaultSite    string `json:"default_site,omitempty"`
	APIURL         string `json:"api_url,omitempty"`
	Timeout        string `json:"timeout,omitempty"`
	KeyringBackend string `json:"keyring_backend,omitempty"`
	BannerTheme    string `json:"banner_theme,omitempty"`
}

// DefaultAPIHost is the default API host, extracted from the default API URL.
const DefaultAPIHost = "api.nimbu.io"

// Defaults returns config with default values.
func Defaults() Config {
	return Config{
		APIURL:  "https://" + DefaultAPIHost,
		Timeout: "30s",
	}
}

// Read loads config from disk, merging with defaults.
func Read() (Config, error) {
	cfg := Defaults()

	path, err := ConfigPath()
	if err != nil {
		return cfg, nil // Return defaults if we can't determine path
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Return defaults if no config file
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}

	// Parse JSON5-like config (strip comments and trailing commas)
	cleaned := cleanJSON5(string(data))

	var fileCfg Config
	if err := json.Unmarshal([]byte(cleaned), &fileCfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	// Merge file values into defaults
	if fileCfg.DefaultSite != "" {
		cfg.DefaultSite = fileCfg.DefaultSite
	}
	if fileCfg.APIURL != "" {
		cfg.APIURL = fileCfg.APIURL
	}
	if fileCfg.Timeout != "" {
		cfg.Timeout = fileCfg.Timeout
	}
	if fileCfg.KeyringBackend != "" {
		cfg.KeyringBackend = fileCfg.KeyringBackend
	}
	if fileCfg.BannerTheme != "" {
		cfg.BannerTheme = fileCfg.BannerTheme
	}

	return cfg, nil
}

// Write saves config to disk.
func Write(cfg Config) error {
	dir, err := EnsureConfigDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path, err := ConfigPath()
	if err != nil {
		return fmt.Errorf("get config path: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	_ = dir // already ensured
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// Get retrieves a single config value by key.
func (c *Config) Get(key string) (string, error) {
	switch strings.ToLower(key) {
	case "default_site", "site":
		return c.DefaultSite, nil
	case "api_url", "apiurl":
		return c.APIURL, nil
	case "timeout":
		return c.Timeout, nil
	case "keyring_backend", "keyring":
		return c.KeyringBackend, nil
	case "banner_theme":
		return c.BannerTheme, nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

// Set updates a single config value by key.
func (c *Config) Set(key, value string) error {
	switch strings.ToLower(key) {
	case "default_site", "site":
		c.DefaultSite = value
	case "api_url", "apiurl":
		c.APIURL = value
	case "timeout":
		// Validate duration
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("invalid timeout duration: %w", err)
		}
		c.Timeout = value
	case "keyring_backend", "keyring":
		c.KeyringBackend = value
	case "banner_theme":
		c.BannerTheme = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

// Unset removes a config value by key.
func (c *Config) Unset(key string) error {
	switch strings.ToLower(key) {
	case "default_site", "site":
		c.DefaultSite = ""
	case "api_url", "apiurl":
		c.APIURL = ""
	case "timeout":
		c.Timeout = ""
	case "keyring_backend", "keyring":
		c.KeyringBackend = ""
	case "banner_theme":
		c.BannerTheme = ""
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

// TimeoutDuration returns the timeout as a duration.
func (c *Config) TimeoutDuration() time.Duration {
	if c.Timeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// cleanJSON5 does minimal preprocessing to handle trailing commas.
// This is a simple approach - for full JSON5 support, use a library.
func cleanJSON5(s string) string {
	lines := strings.Split(s, "\n")
	var result []string

	for _, line := range lines {
		// Remove single-line comments
		if idx := strings.Index(line, "//"); idx >= 0 {
			// Check it's not inside a string (naive check)
			beforeComment := line[:idx]
			if strings.Count(beforeComment, `"`)%2 == 0 {
				line = beforeComment
			}
		}
		result = append(result, line)
	}

	joined := strings.Join(result, "\n")

	// Remove trailing commas before } or ]
	// This is a naive approach but works for simple configs
	joined = strings.ReplaceAll(joined, ",\n}", "\n}")
	joined = strings.ReplaceAll(joined, ",\n]", "\n]")
	joined = strings.ReplaceAll(joined, ", }", " }")
	joined = strings.ReplaceAll(joined, ", ]", " ]")

	return joined
}

// ValidKeys returns all valid config keys.
func ValidKeys() []string {
	return []string{"default_site", "api_url", "timeout", "keyring_backend", "banner_theme"}
}

var ErrNotFound = errors.New("not found")
