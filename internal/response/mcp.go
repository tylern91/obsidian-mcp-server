package response

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolResult formats v as JSON — honoring req's "prettyPrint" argument when the
// tool declares one, falling back to defaultPretty otherwise — and wraps it as
// a tool result. Collapses the FormatJSON+error-check+NewToolResultText
// boilerplate repeated across every tool handler.
func ToolResult(req mcp.CallToolRequest, defaultPretty bool, v any) (*mcp.CallToolResult, error) {
	prettyPrint := req.GetBool("prettyPrint", defaultPretty)
	text, err := FormatJSON(v, prettyPrint)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(text), nil
}

// JSONResourceContents formats v as JSON and wraps it as resource contents at
// uri. On marshal failure it returns the same {"error","uri"} envelope as
// ErrorResourceContents.
func JSONResourceContents(uri, mimeType string, v any, prettyPrint bool) []mcp.ResourceContents {
	text, err := FormatJSON(v, prettyPrint)
	if err != nil {
		return ErrorResourceContents(uri, err.Error())
	}
	return []mcp.ResourceContents{mcp.TextResourceContents{
		URI:      uri,
		MIMEType: mimeType,
		Text:     text,
	}}
}

// ErrorResourceContents wraps msg as a JSON error payload for a resource read.
func ErrorResourceContents(uri, msg string) []mcp.ResourceContents {
	b, _ := json.Marshal(map[string]string{"error": msg, "uri": uri})
	return []mcp.ResourceContents{mcp.TextResourceContents{
		URI:      uri,
		MIMEType: "application/json",
		Text:     string(b),
	}}
}
