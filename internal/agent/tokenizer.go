package agent

import (
	"sync"

	"awas/internal/client"
	"github.com/pkoukk/tiktoken-go"
)

var (
	defaultTokenizer *Tokenizer
	tokenizerOnce    sync.Once
)

func getDefaultTokenizer() *Tokenizer {
	tokenizerOnce.Do(func() {
		enc, err := tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return
		}
		defaultTokenizer = &Tokenizer{encoding: enc}
	})
	return defaultTokenizer
}

type Tokenizer struct {
	encoding *tiktoken.Tiktoken
}

func NewTokenizer(model string) *Tokenizer {
	enc, _ := tiktoken.EncodingForModel(model)
	return &Tokenizer{encoding: enc}
}

func (t *Tokenizer) Count(text string) int {
	if t.encoding == nil {
		return len(text) / 4
	}
	return len(t.encoding.Encode(text, nil, nil))
}

func (t *Tokenizer) CountMessage(msg client.Message) int {
	tokens := 4 
	tokens += t.Count(msg.Role)
	if msg.Content != "" {
		tokens += t.Count(msg.Content)
	}
	for _, tc := range msg.ToolCalls {
		tokens += t.Count(tc.Function.Name)
		tokens += t.Count(tc.Function.Arguments)
	}
	if msg.ToolCallID != "" {
		tokens += t.Count(msg.ToolCallID)
	}
	tokens += 2 
	return tokens
}

func (t *Tokenizer) CountMessages(messages []client.Message) int {
	total := 0
	for _, msg := range messages {
		total += t.CountMessage(msg)
	}
	return total
}

func (t *Tokenizer) CountSystemPrompt(prompt string, skills string) int {
	tokens := 4
	tokens += t.Count(prompt)
	if skills != "" {
		tokens += t.Count(skills)
	}
	tokens += 2
	return tokens
}

func (t *Tokenizer) CountToolDefinitions(tools []client.Tool) int {
	total := 0
	for _, tool := range tools {
		total += t.Count(tool.Function.Name)
		total += t.Count(tool.Function.Description)
		total += 20
	}
	return total
}

func estimateTokens(msg client.Message) int {
	t := getDefaultTokenizer()
	if t != nil {
		return t.CountMessage(msg)
	}

	tokens := 4
	if msg.Role != "" {
		tokens += len(msg.Role)
	}
	if msg.Content != "" {
		tokens += len(msg.Content) / 3
	}
	for _, tc := range msg.ToolCalls {
		tokens += len(tc.Function.Name) / 2
		tokens += len(tc.Function.Arguments) / 3
		tokens += 10
	}
	if msg.ToolCallID != "" {
		tokens += len(msg.ToolCallID)
	}
	tokens += 2
	return tokens
}
