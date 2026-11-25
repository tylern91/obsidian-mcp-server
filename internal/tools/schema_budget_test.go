package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

// schemaTokenCeiling is the maximum cl100k_base token count allowed for the
// full tools/list payload (all tools registered, not read-only).
//
// Measured 2026-11-25: 4101 tokens for 24 tools; 4621 tokens for 25 tools
// after rename_heading, the patch_note line parameter, search truncation and
// list_all_tags paging landed. The ceiling sits about 10% above the latest
// measurement to leave room for small additions. Every tool schema is paid for in every
// client's context window on every session. If a legitimate new tool or
// parameter pushes the total over the ceiling, re-measure with
// `go test ./internal/tools -run TestToolSchemaTokenBudget -v`, then raise
// this constant deliberately in the same PR and say why in the description.
const schemaTokenCeiling = 5100

func TestToolSchemaTokenBudget(t *testing.T) {
	s := server.NewMCPServer("budget", "0.0.0")
	RegisterAll(s, registrationTestDeps(t))

	raw := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}

	var env struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	if len(env.Result.Tools) == 0 {
		t.Fatalf("tools/list returned no tools: %s", payload)
	}

	for _, raw := range env.Result.Tools {
		var named struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &named); err != nil {
			t.Fatalf("decode tool: %v", err)
		}
		t.Logf("%-28s %5d tokens", named.Name, response.CountTokens(string(raw)))
	}

	total := response.CountTokens(string(payload))
	t.Logf("tools/list payload: %d tokens across %d tools (ceiling %d)", total, len(env.Result.Tools), schemaTokenCeiling)
	if total > schemaTokenCeiling {
		t.Errorf("tools/list payload is %d tokens, over the ceiling of %d; trim schemas or raise schemaTokenCeiling deliberately", total, schemaTokenCeiling)
	}
}
