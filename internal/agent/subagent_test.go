package agent

import (
	"awas/internal/config"
	"context"
	"strings"
	"testing"
	"time"
)

func TestSubagentRegistry(t *testing.T) {
	registry := GetSubagentRegistry()

	var eventReceived bool
	registry.RegisterListener(func(ev SubagentEvent) {
		if ev.Type == "started" || ev.Type == "finished" {
			eventReceived = true
		}
	})

	cfg := &config.Config{
		Endpoint: "http://localhost:12345/v1",
		Model:    "test-model",
	}

	inst, err := registry.Spawn(context.Background(), cfg, "Code Researcher", "Inspect README.md")
	if err != nil {
		t.Fatalf("Failed to spawn subagent: %v", err)
	}

	if inst.ID == "" {
		t.Errorf("Expected subagent ID to be non-empty")
	}
	if inst.Role != "Code Researcher" {
		t.Errorf("Expected role 'Code Researcher', got %q", inst.Role)
	}

	list := registry.List()
	if len(list) == 0 {
		t.Errorf("Expected registry.List() to contain subagents")
	}

	msgRes := SendMessageToSubagent(inst.ID, "Focus on installation instructions")
	if !strings.Contains(msgRes, inst.ID) {
		t.Errorf("Expected SendMessageToSubagent result to contain ID %s, got: %s", inst.ID, msgRes)
	}

	manageRes := ManageSubagents("list", "")
	if !strings.Contains(manageRes, inst.ID) {
		t.Errorf("Expected ManageSubagents list to contain ID %s, got: %s", inst.ID, manageRes)
	}

	cancelled := registry.Cancel(inst.ID)
	if !cancelled && inst.Status == SubagentStatusRunning {
		t.Errorf("Expected subagent cancellation to succeed")
	}

	time.Sleep(50 * time.Millisecond)

	if !eventReceived {
		t.Errorf("Expected listener to receive subagent event")
	}
}
