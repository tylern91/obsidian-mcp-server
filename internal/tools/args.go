package tools

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// optStringSlice unmarshals a string tool argument as a JSON array into a
// []string. An absent or empty argument returns a nil slice, not an error.
// Returns a tool error result if the argument is present but invalid JSON.
func optStringSlice(req mcp.CallToolRequest, name string) ([]string, *mcp.CallToolResult) {
	raw := req.GetString(name, "")
	if raw == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("invalid %s: %v", name, err))
	}
	return out, nil
}

// optStringMap unmarshals a string tool argument as a JSON object into a
// map[string]any. An absent or empty argument returns a nil map, not an
// error. Returns a tool error result if the argument is present but invalid
// JSON.
func optStringMap(req mcp.CallToolRequest, name string) (map[string]any, *mcp.CallToolResult) {
	raw := req.GetString(name, "")
	if raw == "" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("invalid %s: %v", name, err))
	}
	return out, nil
}

// effectiveLimit resolves the limit a caller should get: requested if
// positive, else fallback; the result is then capped to ceiling. A ceiling
// <= 0 means uncapped.
func effectiveLimit(requested, fallback, ceiling int) int {
	limit := requested
	if limit <= 0 {
		limit = fallback
	}
	if ceiling > 0 && limit > ceiling {
		limit = ceiling
	}
	return limit
}

// clampBatch truncates paths to at most max entries (max <= 0 means
// unlimited) and reports whether truncation occurred.
func clampBatch(paths []string, max int) ([]string, bool) {
	if max > 0 && len(paths) > max {
		return paths[:max], true
	}
	return paths, false
}

// ifMatchOpt returns a vault.WriteOpt slice conditioning the call on
// ifMatch, or nil if ifMatch is empty.
func ifMatchOpt(ifMatch string) []vault.WriteOpt {
	if ifMatch == "" {
		return nil
	}
	return []vault.WriteOpt{vault.WithIfMatch(ifMatch)}
}

// vaultWriteError translates a vault write error into a tool result,
// surfacing a stable REVISION_CONFLICT code for an if_match mismatch rather
// than letting the raw Go error reach the caller as a protocol error.
func vaultWriteError(err error) *mcp.CallToolResult {
	if errors.Is(err, vault.ErrRevisionConflict) {
		return mcp.NewToolResultError("REVISION_CONFLICT: " + err.Error())
	}
	return mcp.NewToolResultError(err.Error())
}
