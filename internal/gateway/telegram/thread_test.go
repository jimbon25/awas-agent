package telegram

import (
	"awas/internal/config"
	"awas/internal/gateway"
	"encoding/json"
	"testing"
)

func TestExtUpdateUnmarshaling(t *testing.T) {
	tests := []struct {
		name         string
		rawJSON      string
		expectThread int
	}{
		{
			name:         "regular message (no thread)",
			rawJSON:      `{"update_id": 1, "message": {"message_id": 100, "text": "hello"}}`,
			expectThread: 0,
		},
		{
			name:         "forum topic message",
			rawJSON:      `{"update_id": 2, "message": {"message_id": 101, "message_thread_id": 42, "text": "topic message"}}`,
			expectThread: 42,
		},
		{
			name:         "forum topic callback query",
			rawJSON:      `{"update_id": 3, "callback_query": {"id": "c1", "message": {"message_id": 102, "message_thread_id": 88}}}`,
			expectThread: 88,
		},
		{
			name:         "forum topic created service message (empty text)",
			rawJSON:      `{"update_id": 4, "message": {"message_id": 103, "message_thread_id": 42, "forum_topic_created": {"name": "Topic Name"}}}`,
			expectThread: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var up ExtUpdate
			err := json.Unmarshal([]byte(tt.rawJSON), &up)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			gotThread := 0
			if up.Message != nil {
				gotThread = up.Message.MessageThreadID
			} else if up.CallbackQuery != nil && up.CallbackQuery.Message != nil {
				gotThread = up.CallbackQuery.Message.MessageThreadID
			}

			if gotThread != tt.expectThread {
				t.Errorf("MessageThreadID = %d, want %d", gotThread, tt.expectThread)
			}
		})
	}
}

func TestThreadSessionKeyIsolation(t *testing.T) {
	dummyCfg := &config.Config{}
	s1 := gateway.CreateUserSession("12345", "User", "telegram", 0, dummyCfg)
	s2 := gateway.CreateUserSession("12345", "User", "telegram", 10, dummyCfg)

	if s1.SessionID == s2.SessionID {
		t.Errorf("SessionID collision between general chat and topic #10: %s", s1.SessionID)
	}

	if s1.ThreadID != 0 {
		t.Errorf("s1.ThreadID = %d, want 0", s1.ThreadID)
	}

	if s2.ThreadID != 10 {
		t.Errorf("s2.ThreadID = %d, want 10", s2.ThreadID)
	}

	expectedID1 := "gw-telegram-12345-0"
	expectedID2 := "gw-telegram-12345-10"

	if s1.SessionID != expectedID1 {
		t.Errorf("s1.SessionID = %s, want %s", s1.SessionID, expectedID1)
	}

	if s2.SessionID != expectedID2 {
		t.Errorf("s2.SessionID = %s, want %s", s2.SessionID, expectedID2)
	}
}

func TestCleanThreadTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short title",
			input:    "Fix Docker Bug",
			expected: "Fix Docker Bug",
		},
		{
			name:     "multiline title (takes first line)",
			input:    "Setup PostgreSQL Docker\nHere are details...",
			expected: "Setup PostgreSQL Docker",
		},
		{
			name:     "long title (truncated to 35 chars)",
			input:    "How to configure SearXNG search engine instance with custom settings",
			expected: "How to configure SearXNG search...",
		},
		{
			name:     "system notification prefix stripped",
			input:    "[System Notification: User uploaded file 'app.go' and saved to 'downloads/app.go']\nCheck this code",
			expected: "Check this code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanThreadTitle(tt.input)
			if got != tt.expected {
				t.Errorf("cleanThreadTitle(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
