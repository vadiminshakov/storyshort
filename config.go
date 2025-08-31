package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	OpenAIAPIKey string `json:"openai_api_key"`
	SaveLocation string `json:"save_location"`
	Language     string `json:"language"`
	Model        string `json:"model"`
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
		homeDir, _ := os.UserHomeDir()
		defaultLocation := filepath.Join(homeDir, "Downloads", "storyshort")
		return &Config{SaveLocation: defaultLocation, Language: "auto", Model: "whisper-1"}, nil
	}
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	if config.SaveLocation == "" {
		homeDir, _ := os.UserHomeDir()
		config.SaveLocation = filepath.Join(homeDir, "Downloads", "storyshort")
	}
	if config.Language == "" {
		config.Language = "auto"
	}
	if config.Model == "" {
		config.Model = "whisper-1"
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
	
	return os.WriteFile(configPath, data, 0600)
}

func (c *Config) HasValidToken() bool {
	return c.OpenAIAPIKey != "" && len(c.OpenAIAPIKey) > 10
}

func (c *Config) GetOpenAIAPIKey() string {
	return c.OpenAIAPIKey
}

func (c *Config) GetSaveLocation() string {
	return c.SaveLocation
}

func (c *Config) SetOpenAIAPIKey(key string) {
	c.OpenAIAPIKey = key
}

func (c *Config) SetSaveLocation(location string) {
	c.SaveLocation = location
}

func (c *Config) GetLanguage() string {
	return c.Language
}

func (c *Config) SetLanguage(language string) {
	c.Language = language
}

func (c *Config) GetModel() string {
	return c.Model
}

func (c *Config) SetModel(model string) {
	c.Model = model
}

func (c *Config) Save() error {
	return saveConfig(c)
}