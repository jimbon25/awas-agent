package agent

import "context"

type ctxKey string

const (
	ctxKeyPlatform ctxKey = "platform"
	ctxKeyChatID   ctxKey = "chat_id"
	ctxKeyGuildID  ctxKey = "guild_id"
)

func WithGatewayMeta(ctx context.Context, platform, chatID, guildID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyPlatform, platform)
	ctx = context.WithValue(ctx, ctxKeyChatID, chatID)
	ctx = context.WithValue(ctx, ctxKeyGuildID, guildID)
	return ctx
}

func PlatformFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyPlatform).(string); ok {
		return v
	}
	return ""
}

func ChatIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyChatID).(string); ok {
		return v
	}
	return ""
}

func GuildIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyGuildID).(string); ok {
		return v
	}
	return ""
}
