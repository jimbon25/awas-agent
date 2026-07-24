package gateway

import (
	"awas/internal/agent"
	"context"
	"time"
)

type Gateway interface {
	Name() string

	Start(ctx context.Context, mgr *Manager) error

	Stop() error

	Status() GatewayStatus
}

type GatewayStatus struct {
	Running  bool
	Platform string
	Info     string 
}

type GatewayState struct {
	Config  Platform
	Gateway Gateway
	Cancel  context.CancelFunc
	Users   map[string]*UserSession 
}

type UserSession struct {
	UserID       string
	DisplayName  string
	Loop         *agent.Loop
	SessionID    string
	Ctx          context.Context
	Cancel       context.CancelFunc
	LastActive   time.Time
	IsRunning    bool
	ActiveCancel context.CancelFunc
}
