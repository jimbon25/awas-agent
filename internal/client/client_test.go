package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockProvider struct {
	endpoint string
	apiKey   string
	model    string
	headers  map[string]string
}

func (m *mockProvider) GetEndpoint() string            { return m.endpoint }
func (m *mockProvider) GetAPIKey() string              { return m.apiKey }
func (m *mockProvider) GetModel() string               { return m.model }
func (m *mockProvider) GetHeaders() map[string]string { return m.headers }

func TestClientSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Bearer test-api-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Custom-Header") != "CustomValue" {
			t.Errorf("expected CustomValue, got %s", r.Header.Get("X-Custom-Header"))
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var req ChatRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			t.Fatalf("failed to unmarshal request: %v", err)
		}

		if req.Model != "mock-model" {
			t.Errorf("expected mock-model, got %s", req.Model)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
			t.Errorf("unexpected messages: %v", req.Messages)
		}

		resp := ChatResponse{
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role:    "assistant",
						Content: "mocked assistant response",
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &mockProvider{
		endpoint: server.URL,
		apiKey:   "test-api-key",
		model:    "mock-model",
		headers: map[string]string{
			"X-Custom-Header": "CustomValue",
		},
	}

	c := New(provider)
	ctx := context.Background()
	messages := []Message{
		{
			Role:    "user",
			Content: "hello",
		},
	}

	choice, usage, err := c.Send(ctx, messages, nil)
	if err != nil {
		t.Fatalf("client.Send failed: %v", err)
	}

	if choice.Message.Content != "mocked assistant response" {
		t.Errorf("expected 'mocked assistant response', got '%s'", choice.Message.Content)
	}
	if usage.TotalTokens != 30 {
		t.Errorf("expected 30 total tokens, got %d", usage.TotalTokens)
	}
}


func TestSendStream_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no flusher")
		}

		w.Header().Set("Content-Type", "text/event-stream")

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		flusher.Flush()

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n")
		flusher.Flush()

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"World\"}}]}\n\n")
		flusher.Flush()

		stop := "stop"
		fmt.Fprintf(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()

		_ = stop
	}))
	defer server.Close()

	provider := &mockProvider{
		endpoint: server.URL,
		apiKey:   "test-key",
		model:    "test-model",
	}

	c := New(provider)
	sc, err := c.SendStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("SendStream failed: %v", err)
	}

	result := AccumulateStream(sc)

	if result.Content != "Hello World" {
		t.Errorf("expected 'Hello World', got '%q'", result.Content)
	}
	if result.FinishReason != "stop" {
		t.Errorf("expected 'stop', got '%q'", result.FinishReason)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(result.ToolCalls))
	}
}

func TestSendStream_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no flusher")
		}

		w.Header().Set("Content-Type", "text/event-stream")

		chunk1 := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":""}}]}}]}` + "\n\n"
		fmt.Fprint(w, chunk1)
		flusher.Flush()

		chunk2 := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}` + "\n\n"
		fmt.Fprint(w, chunk2)
		flusher.Flush()

		chunk3 := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"main.go\"}"}}]}}]}` + "\n\n"
		fmt.Fprint(w, chunk3)
		flusher.Flush()

		chunk4 := `data: {"choices":[{"finish_reason":"tool_calls"}]}` + "\n\n"
		fmt.Fprint(w, chunk4)
		flusher.Flush()
	}))
	defer server.Close()

	provider := &mockProvider{
		endpoint: server.URL,
		apiKey:   "test-key",
		model:    "test-model",
	}

	c := New(provider)
	sc, err := c.SendStream(context.Background(), []Message{{Role: "user", Content: "read file"}}, nil)
	if err != nil {
		t.Fatalf("SendStream failed: %v", err)
	}

	result := AccumulateStream(sc)

	if result.FinishReason != "tool_calls" {
		t.Errorf("expected 'tool_calls', got '%q'", result.FinishReason)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("expected 'read_file', got '%q'", result.ToolCalls[0].Function.Name)
	}
	expectedArgs := `{"path":"main.go"}`
	if result.ToolCalls[0].Function.Arguments != expectedArgs {
		t.Errorf("expected %q, got %q", expectedArgs, result.ToolCalls[0].Function.Arguments)
	}
}

func TestSendStream_Cancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		<-r.Context().Done()
	}))
	defer server.Close()

	provider := &mockProvider{
		endpoint: server.URL,
		apiKey:   "test-key",
		model:    "test-model",
	}

	c := New(provider)
	ctx, cancel := context.WithCancel(context.Background())
	sc, err := c.SendStream(ctx, []Message{{Role: "user", Content: "test"}}, nil)
	if err != nil {
		t.Fatalf("SendStream failed: %v", err)
	}

	<-sc.Ch

	cancel()

	select {
	case <-sc.done:
	case <-time.After(2 * time.Second):
		t.Error("goroutine leak: cleanup did not complete within 2s")
	}
}

func TestToolCallAccumulator(t *testing.T) {
	acc := NewToolCallAccumulator()

	name1 := "read_file"
	args1 := "{\"path\":"
	acc.Apply([]ToolCallDelta{{
		Index: 0,
		ID:    strPtr("call_1"),
		Type:  strPtr("function"),
		Function: &FunctionDelta{
			Name:      &name1,
			Arguments: &args1,
		},
	}})

	args2 := "\"main.go\"}"
	acc.Apply([]ToolCallDelta{{
		Index: 0,
		Function: &FunctionDelta{
			Arguments: &args2,
		},
	}})

	name2 := "search_code"
	args3 := "{\"pattern\":\"func\"}"
	acc.Apply([]ToolCallDelta{{
		Index: 1,
		ID:    strPtr("call_2"),
		Type:  strPtr("function"),
		Function: &FunctionDelta{
			Name:      &name2,
			Arguments: &args3,
		},
	}})

	calls := acc.GetCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	if calls[0].Function.Name != "read_file" {
		t.Errorf("expected 'read_file', got '%q'", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != "{\"path\":\"main.go\"}" {
		t.Errorf("expected '{\"path\":\"main.go\"}', got '%q'", calls[0].Function.Arguments)
	}
	if calls[1].Function.Name != "search_code" {
		t.Errorf("expected 'search_code', got '%q'", calls[1].Function.Name)
	}
}

func TestToolCallAccumulator_ValidateArguments(t *testing.T) {
	acc := NewToolCallAccumulator()

	validName := "read_file"
	validArgs := "{\"path\":\"main.go\"}"
	acc.Apply([]ToolCallDelta{{
		Index: 0,
		Function: &FunctionDelta{
			Name:      &validName,
			Arguments: &validArgs,
		},
	}})

	errors := acc.ValidateArguments()
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %v", errors)
	}

	acc2 := NewToolCallAccumulator()
	badName := "read_file"
	badArgs := "{invalid json}"
	acc2.Apply([]ToolCallDelta{{
		Index: 0,
		Function: &FunctionDelta{
			Name:      &badName,
			Arguments: &badArgs,
		},
	}})

	errors2 := acc2.ValidateArguments()
	if len(errors2) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(errors2), errors2)
	}
}

func strPtr(s string) *string {
	return &s
}
