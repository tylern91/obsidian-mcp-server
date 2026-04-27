package prompts

import "github.com/mark3labs/mcp-go/mcp"

// errorPrompt returns a GetPromptResult with a single user message containing the error text.
// Prompt handlers never return a Go error for vault errors — they surface them in the prompt body
// so the LLM can acknowledge or report the failure.
func errorPrompt(msg string) *mcp.GetPromptResult {
	return mcp.NewGetPromptResult(
		"Error",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent("Error: "+msg)),
		},
	)
}
