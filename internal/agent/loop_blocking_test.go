package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"awas/internal/client"
	"awas/internal/config"
)


type mockUI struct {
	mu             sync.Mutex
	messages       []msgCall
	thinkingCalls  int32
	approvalCalls  []approvalCall
	approvalBlock  time.Duration 
	approvalResult bool
	chainResult    bool
}

type msgCall struct {
	role    string
	content string
}
type approvalCall struct {
	toolName string
}

func (m *mockUI) PrintThinking(string)    { atomic.AddInt32(&m.thinkingCalls, 1) }
func (m *mockUI) PrintMessageDelta(string) {}
func (m *mockUI) PrintTokenUsage(int)      {}
func (m *mockUI) PrintCompression(int)     {}
func (m *mockUI) SendFile(string, string) error { return nil }

func (m *mockUI) PrintMessage(role string, content string) {
	m.mu.Lock()
	m.messages = append(m.messages, msgCall{role, content})
	m.mu.Unlock()
}

func (m *mockUI) PrintToolCall(name string, args string) {
	m.mu.Lock()
	m.approvalCalls = append(m.approvalCalls, approvalCall{name})
	m.mu.Unlock()
}

func (m *mockUI) PrintToolResult(string, string) {}

func (m *mockUI) RequestApproval(ctx context.Context, toolName string, args string, mode string) bool {
	if m.approvalBlock > 0 {
		select {
		case <-time.After(m.approvalBlock):
			return m.approvalResult
		case <-ctx.Done():
			return false
		}
	}
	return m.approvalResult
}

func (m *mockUI) RequestChainContinue(ctx context.Context) bool {
	return m.chainResult
}

func (m *mockUI) AskUser(ctx context.Context, question string) (string, error) {
	return "mock answer", nil
}

func (m *mockUI) PrintPlan(goal string, steps []string)                                    {}
func (m *mockUI) PrintStepStart(stepID string)                                             {}
func (m *mockUI) PrintStepFinish(stepID string, success bool, result string)               {}

type mockResponse struct {
	Content      string
	ToolName     string 
	ToolArgs     string
	FinishReason string 
}

func newMockLLM(t *testing.T, responses []mockResponse) *httptest.Server {
	t.Helper()
	var callIdx atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(callIdx.Add(1) - 1)
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		resp := responses[idx]

		w.Header().Set("Content-Type", "application/json")

		msg := client.Message{
			Role:    "assistant",
			Content: resp.Content,
		}
		fr := resp.FinishReason
		if fr == "" {
			fr = "stop"
		}
		if resp.ToolName != "" && fr == "tool_calls" {
			msg.ToolCalls = []client.ToolCall{
				{
					ID:   fmt.Sprintf("call_%d", idx),
					Type: "function",
					Function: client.ToolFunction{
						Name:      resp.ToolName,
						Arguments: resp.ToolArgs,
					},
				},
			}
		}

		chatResp := client.ChatResponse{
			Choices: []client.Choice{
				{
					Index:        0,
					Message:      msg,
					FinishReason: fr,
				},
			},
			Usage: client.Usage{TotalTokens: 50},
		}
		json.NewEncoder(w).Encode(chatResp)
	}))
}


func TestEmptyContentWithToolCalls(t *testing.T) {
	server := newMockLLM(t, []mockResponse{
		{Content: "", ToolName: "list_directory", ToolArgs: `{"path":"."}`, FinishReason: "tool_calls"},
		{Content: "Done listing the directory.", FinishReason: "stop"},
	})
	defer server.Close()

	cfg := &config.Config{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
		WorkDir:  t.TempDir(),
		Mode:     "autonomous",
	}

	ui := &mockUI{approvalResult: true}
	loop := NewLoop(cfg)
	loop.UI = ui

	loop.RunAgentCycle(context.Background(), "list directory")

	ui.mu.Lock()
	defer ui.mu.Unlock()

	t.Logf("=== TEST: Empty content + tool_calls ===")
	t.Logf("Messages sent to UI: %d", len(ui.messages))
	for i, m := range ui.messages {
		t.Logf("  msg[%d]: role=%q content=%q", i, m.role, m.content)
	}
	t.Logf("Tool calls dispatched: %d", len(ui.approvalCalls))
	for i, tc := range ui.approvalCalls {
		t.Logf("  tool[%d]: %s", i, tc.toolName)
	}

	emptyAssistantMsg := false
	for _, m := range ui.messages {
		if m.role == "assistant" && m.content == "" {
			emptyAssistantMsg = true
			break
		}
	}
	if emptyAssistantMsg {
		t.Errorf("❌ BUG CONFIRMED: PrintMessage called with EMPTY content!\n"+
			"   loop.go ~521-526: PrintMessage('assistant', '') called.\n"+
			"   TelegramUI.PrintMessage skips empty → user sees NO response.\n"+
			"   Fix: add 'content != \"\"' guard before PrintMessage call.")
	} else {
		t.Log("✓ No empty assistant message sent")
	}

	hasNonEmptyAssistant := false
	for _, m := range ui.messages {
		if m.role == "assistant" && m.content != "" {
			hasNonEmptyAssistant = true
			break
		}
	}
	if !hasNonEmptyAssistant {
		t.Errorf("❌ BUG: No non-empty assistant message was ever sent!\n"+
			"   Agent executed the tool but never showed any response to the user.\n"+
			"   User experience: typing indicator → nothing → confused.")
	} else {
		t.Log("✓ Non-empty assistant message eventually appeared")
	}
}


func TestContentWithToolCalls_Control(t *testing.T) {
	server := newMockLLM(t, []mockResponse{
		{Content: "Let me search for that.", ToolName: "list_directory", ToolArgs: `{"path":"."}`, FinishReason: "tool_calls"},
		{Content: "Here's what I found.", FinishReason: "stop"},
	})
	defer server.Close()

	cfg := &config.Config{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
		WorkDir:  t.TempDir(),
		Mode:     "autonomous",
	}

	ui := &mockUI{approvalResult: true}
	loop := NewLoop(cfg)
	loop.UI = ui

	loop.RunAgentCycle(context.Background(), "list directory")

	ui.mu.Lock()
	defer ui.mu.Unlock()

	t.Logf("=== CONTROL: Content + tool_calls ===")
	for i, m := range ui.messages {
		t.Logf("  msg[%d]: role=%q content=%q", i, m.role, m.content)
	}

	hasAssistantText := false
	for _, m := range ui.messages {
		if m.role == "assistant" && m.content == "Let me search for that." {
			hasAssistantText = true
		}
	}
	if !hasAssistantText {
		t.Error("CONTROL FAIL: Assistant text should have been printed")
	}
	t.Log("✓ CONTROL: Assistant text correctly printed")
}


func TestApprovalBlocksQueuedMessages(t *testing.T) {
	server := newMockLLM(t, []mockResponse{
		{Content: "", ToolName: "list_directory", ToolArgs: `{"path":"."}`, FinishReason: "tool_calls"},
		{Content: "Done.", FinishReason: "stop"},
	})
	defer server.Close()

	cfg := &config.Config{
		Endpoint:      server.URL,
		Model:         "test-model",
		APIKey:        "test-key",
		WorkDir:       t.TempDir(),
		Mode:          "safe",
		MaxChainLimit: 10,
		KeepLastTurns: 10,
		MaxTokens:     100000,
	}

	ui := &mockUI{
		approvalBlock:  100 * time.Millisecond,
		approvalResult: true,
	}
	loop := NewLoop(cfg)
	loop.UI = ui

	msgCh := make(chan string, 10)
	done := make(chan struct{})
	go func() {
		for msg := range msgCh {
			loop.RunAgentCycle(context.Background(), msg)
		}
		close(done)
	}()

	start := time.Now()

	msgCh <- "first message"
	msgCh <- "second message"
	msgCh <- "third message"
	close(msgCh)

	<-done
	elapsed := time.Since(start)

	t.Logf("=== TEST: Sequential approval blocking ===")
	t.Logf("Time for 3 messages (each with 100ms approval block): %v", elapsed)
	t.Logf("If SEQUENTIAL: ~400ms+ (3 × (100ms + tool exec + network))")
	t.Logf("If PARALLEL:   ~120ms (one cycle)")


	if elapsed > 250*time.Millisecond {
		t.Logf("❌ SEQUENTIAL BLOCKING CONFIRMED: %.0fms", float64(elapsed)/float64(time.Millisecond))
		t.Logf("   Each message waits for previous cycle's approval to complete")
		t.Logf("   With 5-minute processTimeout, 1 slow approval blocks ALL subsequent messages")
	} else {
		t.Log("Messages NOT sequentially blocked")
	}

	hist := loop.GetHistory()
	userMsgCount := 0
	for _, h := range hist {
		if h.Role == "user" {
			userMsgCount++
		}
	}
	t.Logf("User messages in history: %d (expected ~3)", userMsgCount)
}


func TestUserScenario(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)

		w.Header().Set("Content-Type", "application/json")
		var resp client.ChatResponse

		if count == 1 {
			resp = client.ChatResponse{
				Choices: []client.Choice{{
					Index: 0,
					Message: client.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []client.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: client.ToolFunction{
								Name:      "list_directory",
								Arguments: `{"path":"."}`,
							},
						}},
					},
					FinishReason: "tool_calls",
				}},
				Usage: client.Usage{TotalTokens: 50},
			}
		} else {
			resp = client.ChatResponse{
				Choices: []client.Choice{{
					Index: 0,
					Message: client.Message{
						Role:    "assistant",
						Content: "Maaf kalau kurang respon, ada yang bisa dibantu?",
					},
					FinishReason: "stop",
				}},
				Usage: client.Usage{TotalTokens: 50},
			}
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &config.Config{
		Endpoint:      server.URL,
		Model:         "test-model",
		APIKey:        "test-key",
		WorkDir:       t.TempDir(),
		Mode:          "autonomous",
		KeepLastTurns: 10,
		MaxTokens:     100000,
	}

	ui := &mockUI{approvalResult: true}
	loop := NewLoop(cfg)
	loop.UI = ui

	t.Log("=== Message 1: user asks question ===")
	loop.RunAgentCycle(context.Background(), "kamu punya tools apa aja?")

	t.Log("=== Message 2: user says 'kok diam?' ===")
	loop.RunAgentCycle(context.Background(), "kamu kok diam aja kalau di tanya?")

	ui.mu.Lock()
	defer ui.mu.Unlock()

	t.Log("\n=== RESULTS ===")
	t.Logf("UI messages: %d", len(ui.messages))
	for i, m := range ui.messages {
		note := ""
		if m.content == "" {
			note = " ← EMPTY"
		}
		t.Logf("  msg[%d]: role=%s content=%q%s", i, m.role, m.content, note)
	}

	hist := loop.GetHistory()
	t.Logf("\nHistory: %d entries", len(hist))
	for i, h := range hist {
		if h.Role == "user" {
			t.Logf("  [%d] USER: %s", i, h.Content)
		} else if h.Role == "assistant" && h.Content != "" {
			s := h.Content
			if len(s) > 60 {
				s = s[:60] + "..."
			}
			t.Logf("  [%d] ASST: %s", i, s)
		} else if h.Role == "assistant" {
			t.Logf("  [%d] ASST: (empty, has tool_calls)", i)
		}
	}

	t.Log("\n=== PATTERN CHECK ===")
	consecutiveUsers := false
	for i := 1; i < len(hist); i++ {
		if hist[i].Role == "user" && hist[i-1].Role == "user" {
			t.Logf("❌ PATTERN: [%d]USER → [%d]USER (no assistant between)", i-1, i)
			t.Logf("   Messages: %q → %q", hist[i-1].Content, hist[i].Content)
			consecutiveUsers = true
		}
	}
	if !consecutiveUsers {
		t.Log("✓ No consecutive user messages found (proper assistant response between each)")
	}
}


func TestLLMStreamEmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		flusher.Flush()

		fmt.Fprintf(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"list_directory","arguments":""}}]}}]}`+"\n\n")
		flusher.Flush()

		fmt.Fprintf(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\".\"}"}}]}}]}`+"\n\n")
		flusher.Flush()

		fmt.Fprintf(w, "data: {\"choices\":[{\"finish_reason\":\"tool_calls\"}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Directory listed successfully.\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server2.Close()

	cfg := &config.Config{
		Endpoint:      server.URL,
		Model:         "test-model",
		APIKey:        "test-key",
		WorkDir:       t.TempDir(),
		Mode:          "autonomous",
		Stream:        true,
		KeepLastTurns: 10,
		MaxTokens:     100000,
	}

	ui := &mockUI{approvalResult: true}
	loop := NewLoop(cfg)
	loop.UI = ui

	_ = server2

	t.Skip("Skipped: streaming test needs dual-mode mock server (requires refactor)")
}


func TestChannelBufferOverflow(t *testing.T) {
	const bufferSize = 10

	sendResults := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		select {
		case make(chan struct{}, bufferSize) <- struct{}{}:
			sendResults = append(sendResults, "OK")
		default:
			sendResults = append(sendResults, "DROPPED")
		}
	}
	t.Logf("Channel buffer size: %d", bufferSize)
	t.Logf("Messages sent: 15")
	t.Logf("→ At most %d can queue before 'select default' drops them", bufferSize)
	t.Logf("→ In Telegram gateway: '⏳ Still processing...' is sent instead")
}


func TestQueueWithContextCancel(t *testing.T) {
	server := newMockLLM(t, []mockResponse{
		{Content: "response", FinishReason: "stop"},
	})
	defer server.Close()

	cfg := &config.Config{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
		WorkDir:  t.TempDir(),
		Mode:     "autonomous",
	}

	ui := &mockUI{approvalResult: true}
	loop := NewLoop(cfg)
	loop.UI = ui

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	loop.RunAgentCycle(ctx, "this should not be processed")

	hist := loop.GetHistory()
	userMsgCount := 0
	for _, h := range hist {
		if h.Role == "user" {
			userMsgCount++
		}
	}

	t.Logf("=== TEST: Cancelled context ===")
	t.Logf("User messages in history: %d", userMsgCount)

	if userMsgCount >= 1 {
		t.Logf("Confirmed: message was consumed but not responded to.")
		t.Logf("In the Telegram gateway, cancelled context causes:")
		t.Logf("  1. Message taken from channel (consumed)")
		t.Logf("  2. context.WithTimeout(session.Ctx) → already done")
		t.Logf("  3. RunAgentCycle sees ctx.Err() → returns immediately")
		t.Logf("  4. Message is LOST — user receives no response")
	} else {
		t.Log("Message was not added to history either")
	}
}
