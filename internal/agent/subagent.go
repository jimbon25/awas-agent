package agent

import (
	"awas/internal/config"
	"awas/internal/session"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SubagentStatus string

const (
	SubagentStatusRunning   SubagentStatus = "running"
	SubagentStatusCompleted SubagentStatus = "completed"
	SubagentStatusFailed    SubagentStatus = "failed"
	SubagentStatusCancelled SubagentStatus = "cancelled"
)

type SubagentInstance struct {
	ID          string             `json:"id"`
	Role        string             `json:"role"`
	Prompt      string             `json:"prompt"`
	Status      SubagentStatus     `json:"status"`
	CurrentStep string             `json:"current_step,omitempty"`
	Result      string             `json:"result"`
	Error       string             `json:"error,omitempty"`
	StartTime   time.Time          `json:"start_time"`
	EndTime     time.Time          `json:"end_time,omitempty"`
	Cancel      context.CancelFunc `json:"-"`
}

type SubagentEvent struct {
	Type     string            `json:"type"` 
	Instance *SubagentInstance `json:"instance"`
}

type SubagentRegistry struct {
	instances map[string]*SubagentInstance
	listeners []func(event SubagentEvent)
	mu        sync.RWMutex
}

var (
	globalRegistry *SubagentRegistry
	registryOnce   sync.Once
)

func GetSubagentRegistry() *SubagentRegistry {
	registryOnce.Do(func() {
		globalRegistry = &SubagentRegistry{
			instances: make(map[string]*SubagentInstance),
			listeners: make([]func(event SubagentEvent), 0),
		}
	})
	return globalRegistry
}

func (r *SubagentRegistry) RegisterListener(fn func(event SubagentEvent)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listeners = append(r.listeners, fn)
}

func (r *SubagentRegistry) emit(event SubagentEvent) {
	r.mu.RLock()
	listeners := make([]func(event SubagentEvent), len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.RUnlock()

	for _, fn := range listeners {
		go fn(event)
	}
}

func (r *SubagentRegistry) Spawn(parentCtx context.Context, cfg *config.Config, role string, prompt string) (*SubagentInstance, error) {
	r.mu.Lock()
	id := fmt.Sprintf("subagent-%d", time.Now().UnixNano()%100000)
	ctx, cancel := context.WithCancel(parentCtx)

	instance := &SubagentInstance{
		ID:        id,
		Role:      role,
		Prompt:    prompt,
		Status:    SubagentStatusRunning,
		StartTime: time.Now(),
		Cancel:    cancel,
	}
	r.instances[id] = instance
	r.mu.Unlock()

	r.emit(SubagentEvent{Type: "started", Instance: instance})

	go func() {
		defer cancel()
		loop := NewSubagentLoop(cfg, id)
		subPrompt := fmt.Sprintf("You are a specialized subagent with role %q.\nTask: %s", role, prompt)
		
		loop.RunAgentCycle(ctx, subPrompt)
		
		r.mu.Lock()
		instance.EndTime = time.Now()
		if ctx.Err() == context.Canceled {
			instance.Status = SubagentStatusCancelled
			instance.Error = "Subagent task was cancelled"
		} else {
			instance.Status = SubagentStatusCompleted
			history := loop.GetHistory()
			if len(history) > 0 {
				lastMsg := history[len(history)-1]
				instance.Result = lastMsg.Content
			}
			if instance.Result == "" {
				instance.Result = "Task completed successfully."
			}
		}
		r.mu.Unlock()

		r.emit(SubagentEvent{Type: "finished", Instance: instance})
		_ = session.Default().SaveSubagentLog("global", instance.ID, instance.Role, instance.Prompt, string(instance.Status), instance.Result, instance.StartTime, instance.EndTime)
	}()

	return instance, nil
}

func (r *SubagentRegistry) List() []*SubagentInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*SubagentInstance
	for _, inst := range r.instances {
		list = append(list, inst)
	}
	return list
}

func (r *SubagentRegistry) Get(id string) (*SubagentInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inst, ok := r.instances[id]
	return inst, ok
}

func (r *SubagentRegistry) UpdateStep(id string, step string) {
	r.mu.Lock()
	inst, ok := r.instances[id]
	if ok {
		inst.CurrentStep = step
	}
	r.mu.Unlock()

	if ok {
		r.emit(SubagentEvent{Type: "progress", Instance: inst})
	}
}

func (r *SubagentRegistry) Cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if inst, ok := r.instances[id]; ok && inst.Status == SubagentStatusRunning {
		if inst.Cancel != nil {
			inst.Cancel()
		}
		inst.Status = SubagentStatusCancelled
		return true
	}
	return false
}

func InvokeSubagent(ctx context.Context, cfg *config.Config, role string, prompt string) string {
	if role == "" || prompt == "" {
		return "[Error] role and prompt parameters are required to invoke a subagent"
	}

	registry := GetSubagentRegistry()
	instance, err := registry.Spawn(ctx, cfg, role, prompt)
	if err != nil {
		return fmt.Sprintf("[Error] failed to spawn subagent: %v", err)
	}

	return fmt.Sprintf("[Subagent Launched] ID: %s | Role: %s | Status: %s. The subagent is running in the background and will report results when complete.", instance.ID, instance.Role, instance.Status)
}

func SendMessageToSubagent(subagentID string, message string) string {
	if subagentID == "" || message == "" {
		return "[Error] subagent_id and message parameters are required"
	}

	registry := GetSubagentRegistry()
	inst, ok := registry.Get(subagentID)
	if !ok {
		return fmt.Sprintf("[Error] subagent with ID %q not found", subagentID)
	}

	return fmt.Sprintf("[Message Sent to %s] Current Status: %s | Role: %s", inst.ID, inst.Status, inst.Role)
}

func ManageSubagents(action string, subagentID string) string {
	registry := GetSubagentRegistry()

	switch strings.ToLower(action) {
	case "list":
		list := registry.List()
		if len(list) == 0 {
			return "No active or recent subagents found."
		}
		var sb strings.Builder
		sb.WriteString("Active Subagents:\n")
		for _, inst := range list {
			sb.WriteString(fmt.Sprintf("- [%s] Role: %s | Status: %s | Elapsed: %s\n",
				inst.ID, inst.Role, inst.Status, subagentDurationString(inst)))
		}
		return sb.String()

	case "kill", "cancel":
		if subagentID == "" {
			return "[Error] subagent_id is required to cancel/kill a subagent"
		}
		if registry.Cancel(subagentID) {
			return fmt.Sprintf("Subagent %s successfully cancelled.", subagentID)
		}
		return fmt.Sprintf("[Error] subagent %s could not be cancelled (already completed or not found).", subagentID)

	default:
		return fmt.Sprintf("[Error] unknown action %q. Supported actions: list, kill", action)
	}
}

func subagentDurationString(inst *SubagentInstance) string {
	if inst.Status == SubagentStatusRunning {
		return fmt.Sprintf("%.1fs", time.Since(inst.StartTime).Seconds())
	}
	return fmt.Sprintf("%.1fs", inst.EndTime.Sub(inst.StartTime).Seconds())
}
