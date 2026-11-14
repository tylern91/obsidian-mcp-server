package tools

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// optStringSlice reads a tool argument as a list of strings. It accepts a
// JSON-array string or a native JSON array. An absent or empty argument
// returns a nil slice, not an error. Returns a tool error result if the
// argument is present but of another type or invalid.
func optStringSlice(req mcp.CallToolRequest, name string) ([]string, *mcp.CallToolResult) {
	val, present := req.GetArguments()[name]
	if !present || val == nil {
		return nil, nil
	}
	var out []string
	switch v := val.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, mcp.NewToolResultError(fmt.Sprintf("invalid %s: %v", name, err))
		}
	case []string:
		out = v
	case []any:
		out = make([]string, len(v))
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, mcp.NewToolResultError(fmt.Sprintf("invalid %s: element %d is %T, want string", name, i, item))
			}
			out[i] = str
		}
	default:
		return nil, mcp.NewToolResultError(fmt.Sprintf("invalid %s: got %T, want a JSON array", name, val))
	}
	return out, nil
}

// optStringMap reads a tool argument as a JSON object. It accepts a
// JSON-object string or a native JSON object. An absent or empty argument
// returns a nil map, not an error. Returns a tool error result if the
// argument is present but of another type or invalid.
func optStringMap(req mcp.CallToolRequest, name string) (map[string]any, *mcp.CallToolResult) {
	val, present := req.GetArguments()[name]
	if !present || val == nil {
		return nil, nil
	}
	switch v := val.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, mcp.NewToolResultError(fmt.Sprintf("invalid %s: %v", name, err))
		}
		return out, nil
	case map[string]any:
		return v, nil
	default:
		return nil, mcp.NewToolResultError(fmt.Sprintf("invalid %s: got %T, want a JSON object", name, val))
	}
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
	if errors.Is(err, vault.ErrNotANote) {
		return mcp.NewToolResultError("NOT_A_NOTE: " + err.Error())
	}
	return mcp.NewToolResultError(err.Error())
}
