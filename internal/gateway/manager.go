package gateway

import (
	"awas/internal/agent"
	"awas/internal/config"
	"awas/internal/cron"
	"context"
	"fmt"
	"log"
	"sync"
)

type AdapterFactory func(p Platform, cfg *config.Config) Gateway

var (
	registryMu sync.Mutex
	registry   = make(map[string]AdapterFactory)
)

func RegisterAdapter(typ string, factory AdapterFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[typ] = factory
}

type Manager struct {
	mu            sync.RWMutex
	gateways      map[string]*GatewayState 
	cfg           *config.Config           
	CronStore     *cron.Store
	CronScheduler *cron.Scheduler
}

func NewManager(cfg *config.Config) *Manager {
	mgr := &Manager{
		gateways: make(map[string]*GatewayState),
		cfg:      cfg,
	}

	store, err := cron.NewStore()
	if err != nil {
		log.Printf("[gateway] Failed to initialize cron store: %v", err)
	} else {
		mgr.CronStore = store
		mgr.CronScheduler = cron.NewScheduler(store, mgr, cfg, func(ctx context.Context, jobCfg *config.Config, prompt string, cronUI *cron.CronUI) {
			loop := agent.NewLoop(jobCfg)
			loop.UI = cronUI
			loop.RunAgentCycle(ctx, prompt)
		})
	}

	return mgr
}

func (mgr *Manager) Load() *GatewayConfig {
	return LoadGatewayConfig()
}

func (mgr *Manager) Save(cfg *GatewayConfig) error {
	return SaveGatewayConfig(cfg)
}

func (mgr *Manager) Start(platform string) error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if state, ok := mgr.gateways[platform]; ok && state.Gateway != nil {
		return fmt.Errorf("gateway '%s' is already running", platform)
	}

	gwCfg := LoadGatewayConfig()
	p, ok := gwCfg.Platforms[platform]
	if !ok {
		return fmt.Errorf("gateway '%s' is not configured", platform)
	}
	if !p.Enabled {
		return fmt.Errorf("gateway '%s' is disabled", platform)
	}

	registryMu.Lock()
	factory, exists := registry[p.Type]
	registryMu.Unlock()

	if !exists {
		return fmt.Errorf("gateway type '%s' is not supported yet (no adapter registered)", p.Type)
	}

	gw := factory(p, mgr.cfg)

	ctx, cancel := context.WithCancel(context.Background())
	state := &GatewayState{
		Config:  p,
		Gateway: gw,
		Cancel:  cancel,
		Users:   make(map[string]*UserSession),
	}

	mgr.gateways[platform] = state

	go func() {
		if err := gw.Start(ctx, mgr); err != nil {
			mgr.mu.Lock()
			delete(mgr.gateways, platform)
			mgr.mu.Unlock()
		}
	}()

	return nil
}

func (mgr *Manager) Stop(platform string) error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	state, ok := mgr.gateways[platform]
	if !ok || state.Gateway == nil {
		return fmt.Errorf("gateway '%s' is not running", platform)
	}

	if state.Cancel != nil {
		state.Cancel()
	}

	if err := state.Gateway.Stop(); err != nil {
		return fmt.Errorf("error stopping gateway '%s': %v", platform, err)
	}

	for _, session := range state.Users {
		if session.Cancel != nil {
			session.Cancel()
		}
	}

	delete(mgr.gateways, platform)
	return nil
}

func (mgr *Manager) StopAll() {
	mgr.mu.Lock()
	platforms := make([]string, 0, len(mgr.gateways))
	for p := range mgr.gateways {
		platforms = append(platforms, p)
	}
	mgr.mu.Unlock()

	for _, p := range platforms {
		mgr.Stop(p)
	}
}

func (mgr *Manager) Status() map[string]GatewayStatus {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	status := make(map[string]GatewayStatus)
	for name, state := range mgr.gateways {
		if state.Gateway != nil {
			status[name] = state.Gateway.Status()
		}
	}
	return status
}

func (mgr *Manager) IsRunning(platform string) bool {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	state, ok := mgr.gateways[platform]
	return ok && state.Gateway != nil
}

func (mgr *Manager) GetUsers() map[string][]*UserSession {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	result := make(map[string][]*UserSession)
	for name, state := range mgr.gateways {
		for _, session := range state.Users {
			result[name] = append(result[name], session)
		}
	}
	return result
}

func (mgr *Manager) DeliverMessage(platform string, chatID string, guildID string, text string) error {
	mgr.mu.RLock()
	state, ok := mgr.gateways[platform]
	mgr.mu.RUnlock()
	if !ok || state.Gateway == nil {
		return fmt.Errorf("gateway '%s' is not running", platform)
	}

	switch platform {
	case "telegram":
		tgGateway, ok := state.Gateway.(interface {
			SendText(chatID int64, text string)
		})
		if !ok {
			return fmt.Errorf("invalid telegram gateway adapter cast")
		}
		var cID int64
		_, err := fmt.Sscanf(chatID, "%d", &cID)
		if err != nil {
			return fmt.Errorf("invalid telegram chat id format: %v", err)
		}
		tgGateway.SendText(cID, text)
		return nil

	case "discord":
		dgGateway, ok := state.Gateway.(interface {
			SendText(channelID string, text string)
		})
		if !ok {
			return fmt.Errorf("invalid discord gateway adapter cast")
		}
		dgGateway.SendText(chatID, text)
		return nil

	default:
		return fmt.Errorf("platform %s not supported for delivery", platform)
	}
}
