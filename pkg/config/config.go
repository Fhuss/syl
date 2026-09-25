package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LLM      LLMConfig      `yaml:"llm"`
	Watcher  WatcherConfig  `yaml:"watcher"`
	Subagent SubagentConfig `yaml:"subagent"`
}

type SubagentConfig struct {
	Harness  string `yaml:"harness"`  // "auto" | "agy" | "claude" | "pi" | "ollama" | "heuristic"
	Model    string `yaml:"model"`    // optional model override (e.g. for ollama or agy)
	Disabled bool   `yaml:"disabled"` // disable background AI summarization
}

type LLMConfig struct {
	Provider string `yaml:"provider"` // "mock" | "openai" | "gemini" | "ollama"
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
}

type WatcherConfig struct {
	TranscriptPath string `yaml:"transcript_path"`
}

func DefaultConfig() *Config {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	return &Config{
		LLM: LLMConfig{
			Provider: "mock",
			Endpoint: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
			APIKey:   apiKey,
			Model:    "gemini-2.5-flash",
		},
		Watcher: WatcherConfig{
			TranscriptPath: "",
		},
		Subagent: SubagentConfig{
			Harness:  "auto",
			Model:    "",
			Disabled: false,
		},
	}
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "syl"), nil
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		dir, err := ConfigDir()
		if err == nil {
			cand := filepath.Join(dir, "config.yaml")
			if _, err := os.Stat(cand); err == nil {
				path = cand
			}
		}
	}

	if path == "" {
		if _, err := os.Stat(".syl.yaml"); err == nil {
			path = ".syl.yaml"
		}
	}

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	return cfg, nil
}

func SaveDefaultConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	cfg := DefaultConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
