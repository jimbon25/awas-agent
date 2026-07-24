package discord

import (
	"awas/internal/config"
	"awas/internal/gateway"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestOnMessageCreate(t *testing.T) {
	cfg := &config.Config{
		WorkDir: t.TempDir(),
	}

	mgr := gateway.NewManager(cfg)

	dg := &DiscordGateway{
		config: gateway.Platform{
			Type:    "discord",
			Enabled: true,
		},
		cfg:    cfg,
		users:  make(map[string]*gateway.UserSession),
		msgChs: make(map[string]chan pendingMsg),
	}

	s, err := discordgo.New("Bot mock-token")
	if err != nil {
		t.Fatalf("failed to create discordgo session: %v", err)
	}

	s.State = discordgo.NewState()
	s.State.User = &discordgo.User{
		ID: "bot-user-123",
	}

	guild := &discordgo.Guild{
		ID: "guild-123",
		Channels: []*discordgo.Channel{
			{
				ID:   "channel-456",
				Type: discordgo.ChannelTypeGuildText,
			},
			{
				ID:   "thread-789",
				Type: discordgo.ChannelTypeGuildPublicThread,
			},
		},
	}
	s.State.GuildAdd(guild)

	msgSelf := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author: &discordgo.User{
				ID: "bot-user-123",
			},
			ChannelID: "channel-456",
			Content:   "Hello",
		},
	}
	dg.OnMessageCreate(s, msgSelf, mgr)
	if len(dg.users) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(dg.users))
	}

	msgNoMention := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author: &discordgo.User{
				ID: "user-abc",
			},
			ChannelID: "channel-456",
			Content:   "Hello bot",
		},
	}
	dg.OnMessageCreate(s, msgNoMention, mgr)
	if len(dg.users) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(dg.users))
	}

	msgInThread := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author: &discordgo.User{
				ID: "user-abc",
			},
			ChannelID: "thread-789",
			Content:   "Help me code",
		},
	}
	dg.OnMessageCreate(s, msgInThread, mgr)
	if len(dg.users) != 1 {
		t.Errorf("expected 1 session, got %d", len(dg.users))
	}
	if _, ok := dg.users["thread-789"]; !ok {
		t.Errorf("expected session for thread-789, got none")
	}

	dg.Stop()
}
