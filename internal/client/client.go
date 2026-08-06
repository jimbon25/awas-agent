package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type LLMProvider interface {
	GetEndpoint() string
	GetAPIKey() string
	GetModel() string
	GetHeaders() map[string]string
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string             `json:"type"`
	Function ToolDefinitionInfo `json:"function"`
}

type ToolDefinitionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatResponse struct {
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type StreamChoice struct {
	Index        int          `json:"index"`
	Delta        MessageDelta `json:"delta"`
	FinishReason *string      `json:"finish_reason"`
}

type MessageDelta struct {
	Role      string          `json:"role,omitempty"`
	Content   *string         `json:"content,omitempty"`
	ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"`
}

type ToolCallDelta struct {
	Index    int            `json:"index"`
	ID       *string        `json:"id,omitempty"`
	Type     *string        `json:"type,omitempty"`
	Function *FunctionDelta `json:"function,omitempty"`
}

type FunctionDelta struct {
	Name      *string `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

type StreamResponse struct {
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage         `json:"usage,omitempty"`
}

type StreamEvent struct {
	Type         EventType
	Content      string
	Role         string
	ToolCalls    []ToolCallDelta
	FinishReason string
	Usage        *Usage
	Error        error
}

type EventType int

const (
	EventDelta EventType = iota
	EventDone
	EventError
)

type StreamController struct {
	Ch       chan StreamEvent
	done     chan struct{}
	cancel   context.CancelFunc
	once     sync.Once
	bodyOnce sync.Once
	body     io.ReadCloser
	ctx      context.Context
}

func NewStreamController(ctx context.Context) (*StreamController, context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	return &StreamController{
		Ch:     make(chan StreamEvent, 64),
		done:   make(chan struct{}),
		cancel: cancel,
		ctx:    ctx,
	}, ctx
}

func (sc *StreamController) emit(ev StreamEvent) bool {
	select {
	case sc.Ch <- ev:
		return true
	case <-sc.ctx.Done():
		return false
	}
}

func extractErrorMessage(body []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Message != "" {
		var nested struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal([]byte(envelope.Error.Message), &nested); jsonErr == nil && nested.Error.Message != "" {
			if nested.Error.Type != "" {
				return fmt.Sprintf("%s: %s", nested.Error.Type, nested.Error.Message)
			}
			return envelope.Error.Message
		}
		if envelope.Error.Type != "" {
			return fmt.Sprintf("%s: %s", envelope.Error.Type, envelope.Error.Message)
		}
		return envelope.Error.Message
	}
	s := strings.TrimSpace(string(body))
	if idx := strings.Index(s, ": "); idx != -1 && idx < 12 {
		s = s[idx+2:]
	}
	return s
}

func (sc *StreamController) Cancel() {
	sc.cancel()
	sc.closeBody()
	select {
	case <-sc.done:
	case <-time.After(3 * time.Second): 
	}
}

func (sc *StreamController) closeBody() {
	if sc.body != nil {
		sc.bodyOnce.Do(func() {
			sc.body.Close()
		})
	}
}

func (sc *StreamController) close() {
	sc.once.Do(func() {
		close(sc.Ch)
		close(sc.done)
	})
}

type Client struct {
	provider   LLMProvider
	httpClient *http.Client
}

func New(provider LLMProvider) *Client {
	return &Client{
		provider: provider,
		httpClient: &http.Client{
			Timeout: 0,
		},
	}
}

func (c *Client) resolveEndpoint() string {
	url := strings.TrimSpace(c.provider.GetEndpoint())
	url = strings.TrimSuffix(url, "/")
	if strings.HasSuffix(url, "/chat/completions") {
		return url
	}
	if strings.HasSuffix(url, "/v1") {
		return url + "/chat/completions"
	}
	return url + "/v1/chat/completions"
}

func (c *Client) Send(ctx context.Context, messages []Message, tools []Tool) (*Choice, *Usage, error) {
	reqBody := ChatRequest{
		Model:    c.provider.GetModel(),
		Messages: messages,
		Tools:    tools,
		Stream:   false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request failed: %w", err)
	}

	endpoint := c.resolveEndpoint()
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if key := c.provider.GetAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	for k, v := range c.provider.GetHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, extractErrorMessage(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response body failed: %w", err)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return nil, nil, fmt.Errorf("decode response failed: %w, raw response: %s", err, string(bodyBytes))
	}

	if len(chatResp.Choices) == 0 {
		return nil, nil, fmt.Errorf("no choices returned in response, raw response: %s", string(bodyBytes))
	}

	return &chatResp.Choices[0], &chatResp.Usage, nil
}

func (c *Client) SendStream(ctx context.Context, messages []Message, tools []Tool) (*StreamController, error) {
	reqBody := ChatRequest{
		Model:    c.provider.GetModel(),
		Messages: messages,
		Tools:    tools,
		Stream:   true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	endpoint := c.resolveEndpoint()
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if key := c.provider.GetAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	for k, v := range c.provider.GetHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, extractErrorMessage(bodyBytes))
	}

	sc, streamCtx := NewStreamController(ctx)
	sc.body = resp.Body

	go func() {
		defer resp.Body.Close()
		defer sc.close()

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for {
			if streamCtx.Err() != nil {
				select {
				case sc.Ch <- StreamEvent{Type: EventError, Error: streamCtx.Err()}:
				default:
				}
				return
			}

			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					select {
					case sc.Ch <- StreamEvent{Type: EventError, Error: fmt.Errorf("stream read error: %w", err)}:
					default:
					}
				}
				return
			}

			line := scanner.Text()
			if line == "" {
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				if !sc.emit(StreamEvent{Type: EventDone}) {
					return
				}
				return
			}

			var streamResp StreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				var errEnvelope struct {
					Error struct {
						Message string `json:"message"`
						Type    string `json:"type"`
					} `json:"error"`
				}
				if jsonErr := json.Unmarshal([]byte(data), &errEnvelope); jsonErr == nil && errEnvelope.Error.Message != "" {
					sc.emit(StreamEvent{Type: EventError, Error: fmt.Errorf("%s", extractErrorMessage([]byte(errEnvelope.Error.Message)))})
				}
				continue
			}

			if len(streamResp.Choices) == 0 {
				continue
			}

			choice := streamResp.Choices[0]
			event := StreamEvent{Type: EventDelta}

			if choice.Delta.Role != "" {
				event.Role = choice.Delta.Role
			}

			if choice.Delta.Content != nil {
				event.Content = *choice.Delta.Content
			}

			if len(choice.Delta.ToolCalls) > 0 {
				event.ToolCalls = choice.Delta.ToolCalls
			}

			if choice.FinishReason != nil {
				event.FinishReason = *choice.FinishReason
			}

			if streamResp.Usage != nil {
				event.Usage = streamResp.Usage
			}

			if !sc.emit(event) {
				return
			}
		}
	}()

	return sc, nil
}
