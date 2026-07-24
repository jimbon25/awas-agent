package client

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ToolCallAccumulator struct {
	calls map[int]*toolCallState
}

type toolCallState struct {
	id        string
	name      string
	arguments strings.Builder
	hasID     bool
	hasName   bool
}

func NewToolCallAccumulator() *ToolCallAccumulator {
	return &ToolCallAccumulator{
		calls: make(map[int]*toolCallState),
	}
}

func (acc *ToolCallAccumulator) Apply(deltas []ToolCallDelta) {
	for _, delta := range deltas {
		state, ok := acc.calls[delta.Index]
		if !ok {
			state = &toolCallState{}
			acc.calls[delta.Index] = state
		}

		if delta.ID != nil {
			state.id = *delta.ID
			state.hasID = true
		}

		if delta.Type != nil {
		}

		if delta.Function != nil {
			if delta.Function.Name != nil {
				state.name = *delta.Function.Name
				state.hasName = true
			}
			if delta.Function.Arguments != nil {
				state.arguments.WriteString(*delta.Function.Arguments)
			}
		}
	}
}

func (acc *ToolCallAccumulator) GetCalls() []ToolCall {
	if len(acc.calls) == 0 {
		return nil
	}

	var result []ToolCall
	for i := 0; i <= maxIndex(acc.calls); i++ {
		state, ok := acc.calls[i]
		if !ok {
			continue
		}
		if !state.hasName {
			continue 
		}

		args := state.arguments.String()
		if args == "" {
			args = "{}"
		}

		result = append(result, ToolCall{
			ID:   state.id,
			Type: "function",
			Function: ToolFunction{
				Name:      state.name,
				Arguments: args,
			},
		})
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func (acc *ToolCallAccumulator) ValidateArguments() []string {
	var errors []string
	for i := 0; i <= maxIndex(acc.calls); i++ {
		state, ok := acc.calls[i]
		if !ok || !state.hasName {
			continue
		}
		args := state.arguments.String()
		if args == "" {
			args = "{}"
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(args), &parsed); err != nil {
			errors = append(errors, fmt.Sprintf("tool %q index %d: invalid JSON arguments: %v", state.name, i, err))
		}
	}
	return errors
}

func (acc *ToolCallAccumulator) Count() int {
	return len(acc.calls)
}

func (acc *ToolCallAccumulator) Reset() {
	acc.calls = make(map[int]*toolCallState)
}

func maxIndex(m map[int]*toolCallState) int {
	max := -1
	for i := range m {
		if i > max {
			max = i
		}
	}
	return max
}

type StreamResult struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        *Usage
}

func AccumulateStream(sc *StreamController) StreamResult {
	acc := NewToolCallAccumulator()
	var content strings.Builder
	var finishReason string
	var usage *Usage

	for event := range sc.Ch {
		switch event.Type {
		case EventDelta:
			if event.Content != "" {
				content.WriteString(event.Content)
			}
			if len(event.ToolCalls) > 0 {
				acc.Apply(event.ToolCalls)
			}
			if event.FinishReason != "" {
				finishReason = event.FinishReason
			}
			if event.Usage != nil {
				usage = event.Usage
			}

		case EventError:
			return StreamResult{
				Content:      content.String(),
				ToolCalls:    acc.GetCalls(),
				FinishReason: finishReason,
				Usage:        usage,
			}

		case EventDone:
		}
	}

	return StreamResult{
		Content:      content.String(),
		ToolCalls:    acc.GetCalls(),
		FinishReason: finishReason,
		Usage:        usage,
	}
}
