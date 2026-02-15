package translator

import (
	"testing"

	"github.com/innomon/adk2goose/internal/gooseclient"
	"google.golang.org/genai"
)

func TestADKContentToGooseMessage_Text(t *testing.T) {
	content := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{genai.NewPartFromText("hello")},
	}

	msg := ADKContentToGooseMessage(content)

	if msg.Role != "user" {
		t.Errorf("expected role %q, got %q", "user", msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != "text" {
		t.Errorf("expected type %q, got %q", "text", msg.Content[0].Type)
	}
	if msg.Content[0].Text != "hello" {
		t.Errorf("expected text %q, got %q", "hello", msg.Content[0].Text)
	}
}

func TestADKContentToGooseMessage_FunctionCall(t *testing.T) {
	content := &genai.Content{
		Role: "model",
		Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{
				ID:   "call1",
				Name: "search",
				Args: map[string]any{"query": "test"},
			}},
		},
	}

	msg := ADKContentToGooseMessage(content)

	if msg.Role != "assistant" {
		t.Errorf("expected role %q, got %q", "assistant", msg.Role)
	}
	if msg.Content[0].Type != "toolRequest" {
		t.Errorf("expected type %q, got %q", "toolRequest", msg.Content[0].Type)
	}
	if msg.Content[0].ToolCall.Name != "search" {
		t.Errorf("expected tool name %q, got %q", "search", msg.Content[0].ToolCall.Name)
	}
	if msg.Content[0].ToolCall.Arguments["query"] != "test" {
		t.Errorf("expected argument query=%q, got %v", "test", msg.Content[0].ToolCall.Arguments["query"])
	}
}

func TestGooseMessageToADKContent_Text(t *testing.T) {
	msg := &gooseclient.GooseMessage{
		Role: "assistant",
		Content: []gooseclient.MessageContent{
			{Type: "text", Text: "hello world"},
		},
	}

	content := GooseMessageToADKContent(msg)

	if content.Role != "model" {
		t.Errorf("expected role %q, got %q", "model", content.Role)
	}
	if len(content.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(content.Parts))
	}
	if content.Parts[0].Text != "hello world" {
		t.Errorf("expected text %q, got %q", "hello world", content.Parts[0].Text)
	}
}

func TestGooseSSEEventToADKEvent_Message(t *testing.T) {
	sse := &gooseclient.SSEEvent{
		Type: "Message",
		Message: &gooseclient.GooseMessage{
			Role: "assistant",
			Content: []gooseclient.MessageContent{
				{Type: "text", Text: "response text"},
			},
		},
	}

	evt, err := GooseSSEEventToADKEvent(sse, "inv-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt == nil {
		t.Fatal("expected non-nil event")
	}
	if evt.Content == nil {
		t.Fatal("expected non-nil content")
	}
	if evt.Content.Parts[0].Text != "response text" {
		t.Errorf("expected text %q, got %q", "response text", evt.Content.Parts[0].Text)
	}
}

func TestGooseSSEEventToADKEvent_Finish(t *testing.T) {
	sse := &gooseclient.SSEEvent{
		Type: "Finish",
		TokenState: &gooseclient.TokenState{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}

	evt, err := GooseSSEEventToADKEvent(sse, "inv-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !evt.TurnComplete {
		t.Error("expected TurnComplete to be true")
	}
	if evt.UsageMetadata == nil {
		t.Fatal("expected non-nil UsageMetadata")
	}
}

func TestGooseSSEEventToADKEvent_Error(t *testing.T) {
	sse := &gooseclient.SSEEvent{
		Type:  "Error",
		Error: "something failed",
	}

	evt, err := GooseSSEEventToADKEvent(sse, "inv-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.ErrorCode != "GOOSE_ERROR" {
		t.Errorf("expected error code %q, got %q", "GOOSE_ERROR", evt.ErrorCode)
	}
	if evt.ErrorMessage != "something failed" {
		t.Errorf("expected error message %q, got %q", "something failed", evt.ErrorMessage)
	}
}

func TestGooseSSEEventToADKEvent_Ping(t *testing.T) {
	sse := &gooseclient.SSEEvent{
		Type: "Ping",
	}

	evt, err := GooseSSEEventToADKEvent(sse, "inv-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt != nil {
		t.Errorf("expected nil event for Ping, got %+v", evt)
	}
}

func TestGooseToolCallToADKFunctionCall(t *testing.T) {
	tc := &gooseclient.ToolCall{
		Name:      "read_file",
		Arguments: map[string]any{"path": "/tmp/test"},
	}

	result := GooseToolCallToADKFunctionCall("tc1", tc)

	if result.ID != "tc1" {
		t.Errorf("expected ID %q, got %q", "tc1", result.ID)
	}
	if result.Name != "read_file" {
		t.Errorf("expected name %q, got %q", "read_file", result.Name)
	}
	if result.Args["path"] != "/tmp/test" {
		t.Errorf("expected path %q, got %v", "/tmp/test", result.Args["path"])
	}
}
