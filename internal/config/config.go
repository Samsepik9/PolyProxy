// Package config loads configuration from YAML.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds the listening server settings.
type ServerConfig struct {
	HTTPListen   string `yaml:"http_listen"`   // e.g. "127.0.0.1:7890"
	SOCKS5Listen string `yaml:"socks5_listen"` // e.g. "127.0.0.1:7891"
	APIListen    string `yaml:"api_listen"`    // e.g. "127.0.0.1:9090"
	APIEnable    bool   `yaml:"api_enable"`    // expose Web UI / REST API
}

// ProxyEntry represents one upstream proxy in the pool.
type ProxyEntry struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`     // http | socks5 | direct
	Server   string `yaml:"server"`   // hostname or IP; ignored when type=direct
	Port     int    `yaml:"port"`     // ignored when type=direct
	Username string `yaml:"username"` // optional
	Password string `yaml:"password"` // optional
}

// PoolConfig controls the proxy-pool behaviour.
type PoolConfig struct {
	Strategy    string        `yaml:"strategy"`     // random | round-robin | hash | name
	HealthCheck bool          `yaml:"health_check"` // periodic TCP dial check
	Proxies     []ProxyEntry  `yaml:"proxies"`
}

// Config is the root configuration.
type Config struct {
	Server ServerConfig `yaml:"server"`
	Pool   PoolConfig   `yaml:"pool"`
}

// Default returns sensible defaults.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPListen:   "127.0.0.1:7890",
			SOCKS5Listen: "127.0.0.1:7891",
			APIListen:    "127.0.0.1:9090",
			APIEnable:    true,
		},
		Pool: PoolConfig{
			Strategy:    "random",
			HealthCheck: false,
			Proxies: []ProxyEntry{
				{Name: "direct", Type: "direct"},
			},
		},
	}
}

// Load reads YAML from path and returns parsed Config. Falls back to Default if path is empty.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate sanity-checks the configuration.
func (c *Config) Validate() error {
	if len(c.Pool.Proxies) == 0 {
		return fmt.Errorf("pool.proxies: must define at least one proxy (use {name: direct, type: direct} for direct connect)")
	}
	strategy := c.Pool.Strategy
	if strategy == "" {
		c.Pool.Strategy = "random"
	}
	switch c.Pool.Strategy {
	case "random", "round-robin", "hash", "name":
	default:
		return fmt.Errorf("pool.strategy: unknown %q (random|round-robin|hash|name)", c.Pool.Strategy)
	}
	seen := map[string]bool{}
	for i, p := range c.Pool.Proxies {
		if p.Name == "" {
			return fmt.Errorf("pool.proxies[%d].name is required", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("pool.proxies[%d]: duplicate name %q", i, p.Name)
		}
		seen[p.Name] = true
		switch p.Type {
		case "direct":
			// no further checks
		case "http", "socks5":
			if p.Server == "" {
				return fmt.Errorf("pool.proxies[%d] (%s): server required for type=%s", i, p.Name, p.Type)
			}
			if p.Port <= 0 || p.Port > 65535 {
				return fmt.Errorf("pool.proxies[%d] (%s): invalid port %d", i, p.Name, p.Port)
			}
		default:
			return fmt.Errorf("pool.proxies[%d] (%s): unknown type %q (http|socks5|direct)", i, p.Name, p.Type)
		}
	}
	return nil
}

// DefaultConfigPath returns the per-user config path for the current OS.
func DefaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "proxypool", "config.yaml")
		}
		return filepath.Join(os.Getenv("USERPROFILE"), "proxypool", "config.yaml")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "proxypool", "config.yaml")
	default: // linux / bsd / others
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "proxypool", "config.yaml")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "proxypool", "config.yaml")
	}
}