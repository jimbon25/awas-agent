package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Credentials struct {
	configPath  string
	AccessToken string `json:"access_token"`
	Provider    string `json:"provider"`
}

func NewCredentials(configPath string) *Credentials {
	if configPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configPath = filepath.Join(home, ".awas", "oauth.json")
		} else {
			configPath = "oauth.json"
		}
	}
	creds := &Credentials{
		configPath: configPath,
	}
	creds.Load()
	return creds
}

func (c *Credentials) Load() error {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, c)
}

func (c *Credentials) Save() error {
	dir := filepath.Dir(c.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configPath, data, 0600)
}
