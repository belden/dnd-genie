package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	path string

	Provider       string `json:"provider,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	ChatModel      string `json:"chat_model,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
}

func (c *Config) Path() string {
	return c.path
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{path: path}, nil
		}
		return nil, err
	}

	config := &Config{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}
	config.path = path
	return config, nil
}

func (c *Config) FromConfig(conf *Config) {
	keepPath := c.path
	*c = *conf
	c.path = keepPath
}

func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(c.path, data, 0o600)
}

func (c *Config) IsEmpty() bool {
	return c.Provider == "" &&
		c.Endpoint == "" &&
		c.ChatModel == "" &&
		c.EmbeddingModel == ""
}
