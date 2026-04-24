package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
)

func TestListDirectoryHandler_Root(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	// Empty path lists vault root.
	result, err := handler(context.Background(), makeRequest("path", ""))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := extractText(t, result)
	var resp struct {
		Path    string `json:"path"`
		Entries []struct {
			Name  string `json:"name"`
			IsDir bool   `json:"isDir"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if len(resp.Entries) == 0 {
		t.Fatal("expected at least one entry in vault root")
	}

	// Filtered directories must not appear.
	for _, e := range resp.Entries {
		switch e.Name {
		case ".obsidian", ".git":
			t.Errorf("filtered entry %q appeared in listing", e.Name)
		}
	}
}

func TestListDirectoryHandler_Notes(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequest("path", "Notes"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := extractText(t, result)
	var resp struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	found := false
	for _, e := range resp.Entries {
		if e.Name == "simple.md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected simple.md to appear in Notes listing")
	}
}

func TestListDirectoryHandler_Traversal(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequest("path", "../"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for path traversal attempt")
	}
}

func TestListDirectoryHandler_NonexistentDirectory(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequest("path", "NoSuchDir"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for nonexistent directory")
	}
}

// extractText pulls the first TextContent from a CallToolResult.
func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}
