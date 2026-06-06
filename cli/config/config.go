package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	Provider       string `json:"provider,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	ChatModel      string `json:"chat_model,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
}

func LoadConfig(path string) (Config, error) {
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

func SaveConfig(path string, config Config) error {
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

func IsEmptyConfig(config Config) bool {
	return config.Provider == "" &&
		config.Endpoint == "" &&
		config.ChatModel == "" &&
		config.EmbeddingModel == ""
}
