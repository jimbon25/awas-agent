package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Endpoint             string `json:"endpoint"`
	Model                string `json:"model"`
	APIKey               string `json:"api_key"`
	WorkDir              string `json:"-"`
	Mode                 string `json:"mode"`                
	MaxChainLimit        int    `json:"max_chain_limit"`     
	MaxTokens            int    `json:"max_tokens"`          
	Stream               bool   `json:"stream"`              

	AgentMode           string `json:"agent_mode"`             
	MaxRetries          int    `json:"max_retries"`            
	RequirePlanApproval bool   `json:"require_plan_approval"`  
	KeepLastTurns       int    `json:"keep_last_turns"`        
	SearXNGURL          string `json:"searxng_url"`             

	MemoryEnabled      bool   `json:"memory_enabled"`
	UserProfileEnabled bool   `json:"user_profile_enabled"`
	MemoryCharLimit    int    `json:"memory_char_limit"`
	UserCharLimit      int    `json:"user_char_limit"`
	NudgeInterval      int    `json:"nudge_interval"`
}

func Load() *Config {
	cfg := &Config{
		Endpoint:             "",
		Model:                "",
		APIKey:               "",
		WorkDir:              "",
		Mode:                 "safe",
		MaxChainLimit:        0,
		MaxTokens:            40000,

		AgentMode:           "simple",
		MaxRetries:          3,
		RequirePlanApproval: true,
		KeepLastTurns:       5,

		MemoryEnabled:      true,
		UserProfileEnabled: true,
		MemoryCharLimit:    2200,
		UserCharLimit:      1375,
		NudgeInterval:      10,
	}

	if home, err := os.UserHomeDir(); err == nil {
		configPath := filepath.Join(home, ".awas", "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			json.Unmarshal(data, cfg)
		}
	}

	if env := os.Getenv("AWAS_ENDPOINT"); env != "" {
		cfg.Endpoint = env
	}
	if env := os.Getenv("AWAS_MODEL"); env != "" {
		cfg.Model = env
	}
	if env := os.Getenv("AWAS_API_KEY"); env != "" {
		cfg.APIKey = env
	}
	if env := os.Getenv("AWAS_WORKDIR"); env != "" {
		cfg.WorkDir = env
	}
	if env := os.Getenv("AWAS_MODE"); env != "" {
		cfg.Mode = env
	}
	if env := os.Getenv("AWAS_MAX_CHAIN_LIMIT"); env != "" {
		var limit int
		if _, err := fmt.Sscan(env, &limit); err == nil {
			cfg.MaxChainLimit = limit
		}
	}
	if env := os.Getenv("AWAS_MAX_TOKENS"); env != "" {
		var maxTokens int
		if _, err := fmt.Sscan(env, &maxTokens); err == nil {
			cfg.MaxTokens = maxTokens
		}
	}
	if env := os.Getenv("AWAS_AGENT_MODE"); env != "" {
		cfg.AgentMode = env
	}
	if env := os.Getenv("AWAS_MAX_RETRIES"); env != "" {
		var maxRetries int
		if _, err := fmt.Sscan(env, &maxRetries); err == nil {
			cfg.MaxRetries = maxRetries
		}
	}
	if env := os.Getenv("AWAS_KEEP_LAST_TURNS"); env != "" {
		var keepLast int
		if _, err := fmt.Sscan(env, &keepLast); err == nil {
			cfg.KeepLastTurns = keepLast
		}
	}
	if env := os.Getenv("AWAS_STREAM"); env != "" {
		cfg.Stream = env == "true" || env == "1"
	}

	if cfg.WorkDir == "" {
		if pwd, err := os.Getwd(); err == nil {
			cfg.WorkDir = pwd
		} else {
			cfg.WorkDir = "."
		}
	} else {
		cfg.WorkDir = filepath.Clean(cfg.WorkDir)
	}

	return cfg
}

func (cfg *Config) Validate() error {
	var msgs []string

	if cfg.Endpoint == "" {
		msgs = append(msgs, fmt.Sprintf("Endpoint is required. Set AWAS_ENDPOINT or add \"endpoint\" to config.json"))
	}

	if cfg.Model == "" {
		msgs = append(msgs, fmt.Sprintf("Model is required. Set AWAS_MODEL or add \"model\" to config.json"))
	}

	if cfg.APIKey == "" && !isLocalEndpoint(cfg.Endpoint) {
		displayEndpoint := cfg.Endpoint
		if displayEndpoint == "" {
			displayEndpoint = "<not set>"
		}
		msgs = append(msgs, fmt.Sprintf("API Key is required for endpoint %q. Set AWAS_API_KEY or add \"api_key\" to config.json", displayEndpoint))
	}

	if cfg.WorkDir != "" {
		info, err := os.Stat(cfg.WorkDir)
		if err != nil {
			if os.IsNotExist(err) {
				msgs = append(msgs, fmt.Sprintf("Work directory %q does not exist", cfg.WorkDir))
			} else {
				msgs = append(msgs, fmt.Sprintf("Work directory %q is not accessible: %v", cfg.WorkDir, err))
			}
		} else if !info.IsDir() {
			msgs = append(msgs, fmt.Sprintf("Work directory %q is not a directory", cfg.WorkDir))
		}
	}

	if len(msgs) > 0 {
		return fmt.Errorf("configuration errors:\n  - %s", strings.Join(msgs, "\n  - "))
	}
	return nil
}

func isLocalEndpoint(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" || strings.HasPrefix(host, "127.")
}

func (cfg *Config) Save() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".awas")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	configPath := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0600)
}

func (cfg *Config) GetEndpoint() string {
	return cfg.Endpoint
}

func (cfg *Config) GetAPIKey() string {
	return cfg.APIKey
}

func (cfg *Config) GetModel() string {
	return cfg.Model
}

func (cfg *Config) GetHeaders() map[string]string {
	return nil
}
