package gooseclient

// GooseMessage represents a message in a Goose conversation.
type GooseMessage struct {
	ID       string           `json:"id,omitempty"`
	Role     string           `json:"role"`
	Created  int64            `json:"created"`
	Content  []MessageContent `json:"content"`
	Metadata *MessageMetadata `json:"metadata,omitempty"`
}

// MessageMetadata controls visibility of a message.
type MessageMetadata struct {
	UserVisible  bool `json:"user_visible"`
	AgentVisible bool `json:"agent_visible"`
}

// MessageContent is a discriminated union over the Type field.
type MessageContent struct {
	Type string `json:"type"`

	// Text / Reasoning
	Text string `json:"text,omitempty"`

	// Image
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`

	// ToolRequest
	ID           string         `json:"id,omitempty"`
	ToolCall     *ToolCall      `json:"toolCall,omitempty"`
	ToolMetadata map[string]any `json:"metadata,omitempty"`

	// ToolResponse
	ToolResult *ToolResult `json:"toolResult,omitempty"`

	// ToolConfirmationRequest
	ToolName  string         `json:"toolName,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Prompt    string         `json:"prompt,omitempty"`

	// Thinking / RedactedThinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// ToolCall describes a tool invocation within a tool request.
type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolResult carries the output of a tool execution.
type ToolResult struct {
	Content           []MessageContent `json:"content,omitempty"`
	IsError           bool             `json:"is_error"`
	StructuredContent map[string]any   `json:"structured_content,omitempty"`
}

// SSEEvent represents a server-sent event from the Goose streaming API.
type SSEEvent struct {
	Type       string        `json:"type"`
	Message    *GooseMessage `json:"message,omitempty"`
	Error      string        `json:"error,omitempty"`
	Reason     string        `json:"reason,omitempty"`
	TokenState *TokenState   `json:"token_state,omitempty"`
	Model      string        `json:"model,omitempty"`
	Mode       string        `json:"mode,omitempty"`
}

// TokenState tracks token usage for a streaming response.
type TokenState struct {
	InputTokens              int32 `json:"input_tokens"`
	OutputTokens             int32 `json:"output_tokens"`
	TotalTokens              int32 `json:"total_tokens"`
	AccumulatedInputTokens   int32 `json:"accumulated_input_tokens"`
	AccumulatedOutputTokens  int32 `json:"accumulated_output_tokens"`
	AccumulatedTotalTokens   int32 `json:"accumulated_total_tokens"`
}

// StartAgentRequest is the payload sent to start a new Goose agent session.
type StartAgentRequest struct {
	WorkingDir string `json:"working_dir"`
	RecipeID   string `json:"recipe_id,omitempty"`
}

// StartAgentResponse is the session object returned after starting an agent.
type StartAgentResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	WorkingDir string `json:"working_dir"`
}

// StopAgentRequest is the payload sent to stop a running agent session.
type StopAgentRequest struct {
	SessionID string `json:"session_id"`
}

// ResumeAgentRequest is the payload sent to resume a previously stopped session.
type ResumeAgentRequest struct {
	SessionID              string `json:"session_id"`
	LoadModelAndExtensions bool   `json:"load_model_and_extensions"`
}

// ReplyRequest is the payload sent to submit a user message to a session.
type ReplyRequest struct {
	UserMessage       *GooseMessage  `json:"user_message"`
	SessionID         string         `json:"session_id"`
	ConversationSoFar []GooseMessage `json:"conversation_so_far,omitempty"`
}

// SessionListResponse wraps the list of known sessions.
type SessionListResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}

// SessionInfo describes a single session in a listing.
type SessionInfo struct {
	ID       string           `json:"id"`
	Path     string           `json:"path"`
	Modified string           `json:"modified"`
	Metadata *SessionMetadata `json:"metadata,omitempty"`
}

// SessionMetadata carries additional details about a session.
type SessionMetadata struct {
	WorkingDir   string `json:"working_dir"`
	Description  string `json:"description"`
	MessageCount int    `json:"message_count"`
}

// SessionHistoryResponse is the full history of a session.
type SessionHistoryResponse struct {
	SessionID string           `json:"sessionId"`
	Metadata  *SessionMetadata `json:"metadata,omitempty"`
	Messages  []GooseMessage   `json:"messages"`
}

// ToolConfirmationRequest is the payload sent to approve or deny a tool call.
type ToolConfirmationRequest struct {
	SessionID string `json:"session_id"`
	RequestID string `json:"request_id"`
	Approved  bool   `json:"approved"`
}
