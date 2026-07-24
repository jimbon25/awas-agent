package tui

import (
	"os"
	"testing"

	"awas/internal/agent"
	"awas/internal/config"
	"awas/internal/provider"
)

func TestHandleSlashCommands(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "awas-tui-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempHome)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempHome)
	defer os.Setenv("HOME", origHome)

	cfg := config.Load()
	cfg.Mode = "autonomous"
	cfg.Model = "gpt-4o"
	cfg.Save()

	promptChan := make(chan AgentPrompt, 1)
	m := NewModel(cfg, promptChan)
	m.Loop = agent.NewLoop(cfg)

	handleSlashCommand(&m, "/mode safe")
	if m.Cfg.Mode != "safe" {
		t.Errorf("expected mode to be 'safe', got %s", m.Cfg.Mode)
	}

	handleSlashCommand(&m, "/setup")
	if m.State != StateSetupWizard {
		t.Errorf("expected state to be StateSetupWizard, got %d", m.State)
	}

	m.State = StateIdle

	mgr := provider.NewManager("")
	mgr.Profiles["gemini"] = &provider.ProviderConfig{
		Name:   provider.ProviderGemini,
		APIKey: "gemini-test-key",
		Model:  "gemini-2.0-flash",
	}
	mgr.Save()

	handleSlashCommand(&m, "/switch gemini")
	if m.Cfg.Model != "gemini-2.0-flash" {
		t.Errorf("expected active model to switch to 'gemini-2.0-flash', got %s", m.Cfg.Model)
	}
	if m.Cfg.APIKey != "gemini-test-key" {
		t.Errorf("expected active API key to switch to 'gemini-test-key', got %s", m.Cfg.APIKey)
	}

	handleSlashCommand(&m, "/model claude-3")
	if m.Cfg.Model != "claude-3" {
		t.Errorf("expected model to switch to 'claude-3', got %s", m.Cfg.Model)
	}
	mgr2 := provider.NewManager("")
	if mgr2.Profiles["gemini"].Model != "claude-3" {
		t.Errorf("expected provider model to be updated to 'claude-3', got %s", mgr2.Profiles["gemini"].Model)
	}

	handleSlashCommand(&m, "/logout")
	if m.Cfg.APIKey != "" {
		t.Errorf("expected API Key to be cleared on logout, got %s", m.Cfg.APIKey)
	}
}
