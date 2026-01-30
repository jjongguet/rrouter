package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

//go:embed config.json
var defaultConfigJSON []byte

type Config struct {
	Modes       map[string]ModeConfig `json:"modes"`
	DefaultMode string                `json:"defaultMode"`
}

type ModeConfig struct {
	Mappings     []ModelMapping      `json:"mappings"`
	AgentRouting *AgentRoutingConfig `json:"agentRouting,omitempty"`
}

type ModelMapping struct {
	Match   string `json:"match"`
	Rewrite string `json:"rewrite"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config JSON: %w", err)
	}
	return &cfg, nil
}

func loadConfigWithDefaults() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Error getting home dir, using embedded defaults: %v", err)
		return loadEmbeddedConfig()
	}

	configPath := filepath.Join(homeDir, ".rrouter", "config.json")
	cfg, err := loadConfig(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Error reading config.json, using embedded defaults: %v", err)
		}
		cfg = loadEmbeddedConfig()
	}

	// Check for legacy config file and warn
	legacyPath := filepath.Join(homeDir, ".rrouter", "config")
	if _, err := os.Stat(legacyPath); err == nil {
		log.Printf("Found legacy ~/.rrouter/config file; using config.json instead. Remove the old file to silence this warning.")
	}

	return cfg
}

func loadEmbeddedConfig() *Config {
	var cfg Config
	if err := json.Unmarshal(defaultConfigJSON, &cfg); err != nil {
		log.Fatalf("Failed to parse embedded default config: %v", err)
	}
	return &cfg
}

func matchModel(pattern, model string) bool {
	if pattern == "" {
		return false
	}
	matched, err := filepath.Match(pattern, model)
	if err != nil {
		return false
	}
	return matched
}

func rewriteModelWithConfig(model string, modeConfig *ModeConfig) string {
	if modeConfig == nil {
		return model
	}
	for _, m := range modeConfig.Mappings {
		if matchModel(m.Match, model) {
			return m.Rewrite
		}
	}
	return model // passthrough if no match
}

func getConfig() (listenAddr string, upstreamURL string) {
	port := os.Getenv("RROUTER_PORT")
	if port == "" {
		port = "8316"
	}
	port = strings.TrimPrefix(port, ":")
	listenAddr = ":" + port

	upstreamURL = os.Getenv("RROUTER_UPSTREAM")
	if upstreamURL == "" {
		upstreamURL = "http://localhost:8317"
	}

	return listenAddr, upstreamURL
}
