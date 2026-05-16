package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the top-level configuration structure.
type Config struct {
	Daemon  DaemonConfig  `toml:"daemon"`
	AI      AIConfig      `toml:"ai"`
	Privacy PrivacyConfig `toml:"privacy"`
}

// DaemonConfig holds server-level settings.
type DaemonConfig struct {
	Port     int    `toml:"port"`
	LogLevel string `toml:"log_level"`
	PidFile  string `toml:"pid_file"`
}

// AIConfig holds AI provider settings.
type AIConfig struct {
	DefaultProvider  string `toml:"default_provider"`
	ChatModel        string `toml:"chat_model"`
	InlineModel      string `toml:"inline_model"`
	ContextDepth     string `toml:"context_depth"`
	MaxContextTokens int    `toml:"max_context_tokens"`
}

// PrivacyConfig holds privacy and local-mode settings.
type PrivacyConfig struct {
	LocalMode       bool   `toml:"local_mode"`
	SendFileContent bool   `toml:"send_file_content"`
	OllamaEndpoint  string `toml:"ollama_endpoint"`
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Daemon: DaemonConfig{
			Port:     7878,
			LogLevel: "info",
			PidFile:  filepath.Join(home, ".forge", "forge.pid"),
		},
		AI: AIConfig{
			DefaultProvider:  "claude",
			ChatModel:        "claude-sonnet-4-5-20251001",
			InlineModel:      "claude-haiku-4-5-20251001",
			ContextDepth:     "medium",
			MaxContextTokens: 80000,
		},
		Privacy: PrivacyConfig{
			LocalMode:       false,
			SendFileContent: true,
			OllamaEndpoint:  "http://localhost:11434",
		},
	}
}

// Load reads config from ~/.forge/config.toml, creating it with defaults if it
// does not exist. Values in the file override the defaults.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("config: cannot determine home dir: %w", err)
	}

	forgeDir := filepath.Join(home, ".forge")
	if err := os.MkdirAll(forgeDir, 0o700); err != nil {
		return nil, fmt.Errorf("config: cannot create ~/.forge: %w", err)
	}

	cfgPath := filepath.Join(forgeDir, "config.toml")

	cfg := Default()

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		// Write the default config so users have a template to edit.
		if err := writeDefault(cfgPath, cfg); err != nil {
			// Non-fatal; we can run with in-memory defaults.
			fmt.Fprintf(os.Stderr, "forge: warning: could not write default config: %v\n", err)
		}
		return cfg, nil
	}

	if _, err := toml.DecodeFile(cfgPath, cfg); err != nil {
		return nil, fmt.Errorf("config: parse error in %s: %w", cfgPath, err)
	}

	return cfg, nil
}

func writeDefault(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	return enc.Encode(cfg)
}
