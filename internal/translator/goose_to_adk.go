package translator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/innomon/adk2goose/internal/gooseclient"
	"google.golang.org/genai"
)

// ADKEvent represents an event in the ADK REST API SSE stream.
type ADKEvent struct {
	ID            string                                      `json:"id"`
	Time          int64                                       `json:"time"`
	InvocationID  string                                      `json:"invocationId"`
	Branch        string                                      `json:"branch"`
	Author        string                                      `json:"author"`
	Partial       bool                                        `json:"partial"`
	Content       *genai.Content                              `json:"content,omitempty"`
	TurnComplete  bool                                        `json:"turnComplete"`
	Interrupted   bool                                        `json:"interrupted"`
	ErrorCode     string                                      `json:"errorCode,omitempty"`
	ErrorMessage  string                                      `json:"errorMessage,omitempty"`
	Actions       *ADKEventActions                            `json:"actions,omitempty"`
	UsageMetadata *genai.GenerateContentResponseUsageMetadata `json:"usageMetadata,omitempty"`
}

// ADKEventActions holds state changes associated with an ADK event.
type ADKEventActions struct {
	StateDelta map[string]any `json:"stateDelta,omitempty"`
}

// GooseSSEEventToADKEvent converts a Goose SSE event into an ADK REST event.
func GooseSSEEventToADKEvent(sse *gooseclient.SSEEvent, invocationID string) (*ADKEvent, error) {
	switch sse.Type {
	case "Message":
		content := GooseMessageToADKContent(sse.Message)
		return &ADKEvent{
			ID:           fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			Time:         time.Now().Unix(),
			InvocationID: invocationID,
			Author:       "goose",
			Content:      content,
		}, nil

	case "Finish":
		evt := &ADKEvent{
			ID:           fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			Time:         time.Now().Unix(),
			InvocationID: invocationID,
			Author:       "goose",
			TurnComplete: true,
		}
		if sse.TokenState != nil {
			evt.UsageMetadata = GooseTokenStateToUsageMetadata(sse.TokenState)
		}
		return evt, nil

	case "Error":
		return &ADKEvent{
			ID:           fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			Time:         time.Now().Unix(),
			InvocationID: invocationID,
			Author:       "goose",
			ErrorCode:    "GOOSE_ERROR",
			ErrorMessage: sse.Error,
		}, nil

	case "Ping":
		return nil, nil

	default:
		return nil, nil
	}
}

// GooseMessageToADKContent converts a Goose message into a genai Content.
func GooseMessageToADKContent(msg *gooseclient.GooseMessage) *genai.Content {
	role := msg.Role
	if role == "assistant" {
		role = "model"
	}

	var parts []*genai.Part
	for _, mc := range msg.Content {
		switch mc.Type {
		case "text":
			parts = append(parts, genai.NewPartFromText(mc.Text))

		case "toolRequest":
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   mc.ID,
					Name: mc.ToolCall.Name,
					Args: mc.ToolCall.Arguments,
				},
			}
			parts = append(parts, part)

		case "toolResponse":
			resultText := extractToolResultText(mc.ToolResult)
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       mc.ID,
					Name:     "",
					Response: map[string]any{"result": resultText},
				},
			}
			parts = append(parts, part)

		case "thinking", "reasoning":
			text := mc.Thinking
			if text == "" {
				text = mc.Text
			}
			part := genai.NewPartFromText(text)
			part.Thought = true
			parts = append(parts, part)
		}
	}

	return &genai.Content{Parts: parts, Role: role}
}

// GooseTokenStateToUsageMetadata converts Goose token state into genai usage metadata.
func GooseTokenStateToUsageMetadata(ts *gooseclient.TokenState) *genai.GenerateContentResponseUsageMetadata {
	return &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     ts.InputTokens,
		CandidatesTokenCount: ts.OutputTokens,
		TotalTokenCount:      ts.TotalTokens,
	}
}

// extractToolResultText extracts a text representation from a ToolResult.
func extractToolResultText(tr *gooseclient.ToolResult) string {
	if tr == nil {
		return ""
	}
	for _, c := range tr.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text
		}
	}
	if tr.StructuredContent != nil {
		b, err := json.Marshal(tr.StructuredContent)
		if err == nil {
			return string(b)
		}
	}
	return ""
}
