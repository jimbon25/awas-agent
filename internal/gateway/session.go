package gateway

import (
	"awas/internal/agent"
	"awas/internal/config"
	"awas/internal/session"
	"context"
	"fmt"
	"time"
)

const (
	sessionInactivityTimeout = 30 * time.Minute
	sessionIDPrefix          = "gw"
)

var sessionStore = session.Default()

func CreateUserSession(userID, displayName, platform string, cfg *config.Config) *UserSession {
	sessionID := fmt.Sprintf("%s-%s-%s", sessionIDPrefix, platform, userID)

	ctx, cancel := context.WithCancel(context.Background())
	loop := agent.NewLoop(cfg)

	s := &UserSession{
		UserID:      userID,
		DisplayName: displayName,
		Loop:        loop,
		SessionID:   sessionID,
		Ctx:         ctx,
		Cancel:      cancel,
		LastActive:  time.Now(),
	}

	s.restoreSession(cfg)

	return s
}

func (s *UserSession) restoreSession(cfg *config.Config) {
	data, err := sessionStore.Load(s.SessionID)
	if err != nil || data == nil {
		return 
	}

	if len(data.History) > 0 {
		s.Loop.SetHistory(data.History)
	}

	if data.WorkDir != "" {
		s.Loop.GetConfig().WorkDir = data.WorkDir
	}
	if data.Model != "" {
		s.Loop.GetConfig().Model = data.Model
	}
	if data.Mode != "" {
		s.Loop.GetConfig().Mode = data.Mode
	}
	if data.AgentMode != "" {
		s.Loop.GetConfig().AgentMode = data.AgentMode
	}
}

func (s *UserSession) SaveSession(cfg *config.Config) {
	history := s.Loop.GetHistory()
	if len(history) == 0 {
		return
	}

	loopCfg := s.Loop.GetConfig()
	data := &session.SessionData{
		ID:        s.SessionID,
		Title:     fmt.Sprintf("Gateway: %s", s.DisplayName),
		WorkDir:   loopCfg.WorkDir,
		Model:     loopCfg.Model,
		Mode:      loopCfg.Mode,
		AgentMode: loopCfg.AgentMode,
		CreatedAt: s.LastActive,
		History:   history,
	}

	sessionStore.Save(data)
}

func (s *UserSession) UpdateActivity() {
	s.LastActive = time.Now()
}

func (s *UserSession) IsInactive() bool {
	return time.Since(s.LastActive) > sessionInactivityTimeout
}

func CleanupInactiveSessions(state *GatewayState, cfg *config.Config) {
	for userID, session := range state.Users {
		if session.IsInactive() {
			session.SaveSession(cfg)
			session.Cancel()
			delete(state.Users, userID)
		}
	}
}

// DeleteSession wipes the SQLite database entry for the given session ID.
func DeleteSession(sessionID string) error {
	return sessionStore.Delete(sessionID)
}
