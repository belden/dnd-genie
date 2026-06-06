package main

import (
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
