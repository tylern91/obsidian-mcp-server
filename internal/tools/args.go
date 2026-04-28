package tools

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// parseJSONArg unmarshals a string tool argument as JSON into dst.
// Returns a tool error result if the argument is missing or invalid JSON.
func parseJSONArg[T any](req mcp.CallToolRequest, name string, dst *T) *mcp.CallToolResult {
	raw := req.GetString(name, "")
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid %s: %v", name, err))
	}
	return nil
}
