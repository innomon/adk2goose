package translator

import (
	"encoding/json"

	"github.com/innomon/adk2goose/internal/gooseclient"
	"google.golang.org/genai"
)

// ADKToolToGooseToolInfo converts an ADK tool declaration to a description string
// suitable for logging/display. Goose manages its own tools via extensions,
// so this is primarily informational.
func ADKToolToGooseToolInfo(decl *genai.FunctionDeclaration) map[string]any {
	info := map[string]any{
		"name":        decl.Name,
		"description": decl.Description,
	}
	if decl.Parameters != nil {
		info["parameters"] = decl.Parameters
	}
	return info
}

// GooseToolCallToADKFunctionCall converts a Goose ToolCall to an ADK FunctionCall.
func GooseToolCallToADKFunctionCall(id string, tc *gooseclient.ToolCall) *genai.FunctionCall {
	return &genai.FunctionCall{
		ID:   id,
		Name: tc.Name,
		Args: tc.Arguments,
	}
}

// ADKFunctionResponseToGooseToolResult converts an ADK FunctionResponse to a Goose ToolResult.
func ADKFunctionResponseToGooseToolResult(fr *genai.FunctionResponse) *gooseclient.ToolResult {
	text := ""
	if fr.Response != nil {
		data, err := json.Marshal(fr.Response)
		if err == nil {
			text = string(data)
		}
	}
	return &gooseclient.ToolResult{
		Content: []gooseclient.MessageContent{
			{Type: "text", Text: text},
		},
		IsError: false,
	}
}
