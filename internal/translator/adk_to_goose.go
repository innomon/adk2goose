package translator

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/innomon/adk2goose/internal/gooseclient"
	"google.golang.org/genai"
)

// ADKContentToGooseMessage converts an ADK genai.Content into a Goose message.
func ADKContentToGooseMessage(content *genai.Content) *gooseclient.GooseMessage {
	role := "user"
	if content.Role == "model" {
		role = "assistant"
	}

	var parts []gooseclient.MessageContent
	for _, part := range content.Parts {
		if part.Text != "" {
			parts = append(parts, gooseclient.MessageContent{
				Type: "text",
				Text: part.Text,
			})
		}
		if part.FunctionCall != nil {
			parts = append(parts, gooseclient.MessageContent{
				Type: "toolRequest",
				ID:   part.FunctionCall.ID,
				ToolCall: &gooseclient.ToolCall{
					Name:      part.FunctionCall.Name,
					Arguments: part.FunctionCall.Args,
				},
			})
		}
		if part.FunctionResponse != nil {
			respText, _ := json.Marshal(part.FunctionResponse.Response)
			parts = append(parts, gooseclient.MessageContent{
				Type: "toolResponse",
				ID:   part.FunctionResponse.ID,
				ToolResult: &gooseclient.ToolResult{
					Content: []gooseclient.MessageContent{
						{Type: "text", Text: string(respText)},
					},
					IsError: false,
				},
			})
		}
		if part.InlineData != nil {
			parts = append(parts, gooseclient.MessageContent{
				Type:     "image",
				Data:     base64.StdEncoding.EncodeToString(part.InlineData.Data),
				MimeType: part.InlineData.MIMEType,
			})
		}
	}

	return &gooseclient.GooseMessage{
		Role:    role,
		Created: time.Now().Unix(),
		Content: parts,
		Metadata: &gooseclient.MessageMetadata{
			UserVisible:  true,
			AgentVisible: true,
		},
	}
}

// ADKRunSSERequestToReplyRequest converts a session ID and ADK content into a
// Goose ReplyRequest suitable for the streaming reply endpoint.
func ADKRunSSERequestToReplyRequest(sessionID string, content *genai.Content) *gooseclient.ReplyRequest {
	msg := ADKContentToGooseMessage(content)
	return &gooseclient.ReplyRequest{
		UserMessage: msg,
		SessionID:   sessionID,
	}
}
