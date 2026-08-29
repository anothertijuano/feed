package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// defaultServer is the built-in fallback when no server is configured.
const defaultServer = "http://localhost:8084"

// Config is the on-disk client configuration.
type Config struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}

// configPath returns the location of the user's config file.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "feedtui", "config.json"), nil
}

// loadConfig reads the config file, returning a zero Config if it is missing
// or unreadable.
func loadConfig(path string) Config {
	var cfg Config
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	return cfg
}

// saveConfig writes the config file, creating parent directories as needed.
// The file gets owner-only permissions because it may hold a token.
func saveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
