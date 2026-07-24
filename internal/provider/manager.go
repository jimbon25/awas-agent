package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Manager struct {
	configPath    string
	ActiveProfile string                     `json:"active_profile"`
	Profiles      map[string]*ProviderConfig `json:"profiles"`
}

func NewManager(configPath string) *Manager {
	if configPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configPath = filepath.Join(home, ".awas", "providers.json")
		} else {
			configPath = "providers.json"
		}
	}

	mgr := &Manager{
		configPath:    configPath,
		ActiveProfile: "default",
		Profiles:      make(map[string]*ProviderConfig),
	}

	mgr.Load()
	return mgr
}

func (m *Manager) Load() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, m)
}

func (m *Manager) Save() error {
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath, data, 0600)
}
