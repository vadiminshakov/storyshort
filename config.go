package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	OpenAIAPIKey string `json:"openai_api_key"`
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	
	configDir := filepath.Join(homeDir, ".shortstory")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}
	
	return filepath.Join(configDir, "config.json"), nil
}

func loadConfig() (*Config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}
	
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Конфигурации не существует, возвращаем пустую
		return &Config{}, nil
	}
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	return &config, nil
}

func saveConfig(config *Config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(configPath, data, 0600) // Только для владельца
}

func (c *Config) HasValidToken() bool {
	return c.OpenAIAPIKey != "" && len(c.OpenAIAPIKey) > 10 // Минимальная проверка
}