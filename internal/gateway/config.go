package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type GatewayConfig struct {
	Enabled   bool                `json:"enabled"`
	Platforms map[string]Platform `json:"platforms"`
}

type Platform struct {
	Type         string            `json:"type"`                    
	Enabled      bool              `json:"enabled"`
	Token        string            `json:"token"`                   
	AppID        string            `json:"app_id,omitempty"`        
	AllowedUsers []string          `json:"allowed_users,omitempty"` 
	MaxUsers     int               `json:"max_users,omitempty"`     
	Extra        map[string]string `json:"extra,omitempty"`         
}

func LoadGatewayConfig() *GatewayConfig {
	cfg := &GatewayConfig{
		Enabled:   false,
		Platforms: make(map[string]Platform),
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	configPath := filepath.Join(home, ".awas", "gateways.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	json.Unmarshal(data, cfg)
	return cfg
}

func SaveGatewayConfig(cfg *GatewayConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(home, ".awas")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(dir, "gateways.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}
