package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	providerLMStudio = "lmstudio"
	providerOllama   = "ollama"

	defaultLMStudioEndpoint = "http://127.0.0.1:1234/v1"
	defaultOllamaEndpoint   = "http://127.0.0.1:11434"
)

type Config struct {
	Provider       string `json:"provider,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	ChatModel      string `json:"chat_model,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
}

func configPathFromEnv() string {
	if configured := os.Getenv("DNDX_CONFIG"); configured != "" {
		return configured
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "dndx.json"
		}
		configDir = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configDir, "dndx", "config.json")
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func saveConfig(path string, config Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func normalizeProvider(provider string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case providerLMStudio, providerOllama:
		return normalized, nil
	default:
		return "", errUsage("provider must be lmstudio or ollama")
	}
}

func defaultEndpoint(provider string) string {
	switch provider {
	case providerLMStudio:
		return defaultLMStudioEndpoint
	case providerOllama:
		return defaultOllamaEndpoint
	default:
		return ""
	}
}

func normalizeEndpoint(provider string, endpoint string) string {
	normalized := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if normalized == "" {
		normalized = defaultEndpoint(provider)
	}

	if provider == providerLMStudio && !strings.HasSuffix(normalized, "/v1") {
		return normalized + "/v1"
	}
	return normalized
}

func isEmptyConfig(config Config) bool {
	return config.Provider == "" &&
		config.Endpoint == "" &&
		config.ChatModel == "" &&
		config.EmbeddingModel == ""
}
