package agent

import (
	"strings"
	"testing"

	"awas/internal/client"
)

func TestEstimateTokens(t *testing.T) {
	msg := client.Message{
		Role:    "user",
		Content: "Hello world!",
	}
	got := estimateTokens(msg)
	want := 14 
	if got != want {
		t.Logf("got %d tokens (heuristic would be %d)", got, want)
	}
}

func TestCompressHistory_UnderThreshold(t *testing.T) {
	history := []client.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Hello"},
	}

	newHist, compressed, _ := CompressHistory(nil, history, 1000, "mock-model", 5)
	if compressed {
		t.Error("expected no compression")
	}
	if len(newHist) != len(history) {
		t.Errorf("expected history length %d, got %d", len(history), len(newHist))
	}
}

func TestCompressHistory_FallbackSlidingWindow(t *testing.T) {
	history := []client.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "First message"},
		{Role: "assistant", Content: "First reply"},
		{Role: "user", Content: "Second message"},
	}

	newHist, compressed, turns := CompressHistory(nil, history, 2, "mock-model", 1)
	if !compressed {
		t.Error("expected compression/sliding window to trigger")
	}
	if turns != 1 {
		t.Errorf("expected 1 turn compressed, got %d", turns)
	}
	if len(newHist) != 3 {
		t.Errorf("expected length 3, got %d", len(newHist))
	}
	if newHist[0].Content != "System prompt" {
		t.Errorf("expected system prompt, got %s", newHist[0].Content)
	}
	if !strings.Contains(newHist[1].Content, "[COMPRESSED LOG]") {
		t.Errorf("expected [COMPRESSED LOG] placeholder at index 1, got %s", newHist[1].Content)
	}
	if newHist[2].Content != "Second message" {
		t.Errorf("expected 'Second message', got %s", newHist[2].Content)
	}
}
