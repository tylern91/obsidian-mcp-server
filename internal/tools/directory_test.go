package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
)

// dirResp is the full response envelope returned by list_directory.
type dirResp struct {
	Path      string     `json:"path"`
	Entries   []dirEntry `json:"entries"`
	Count     int        `json:"count"`
	Total     int        `json:"total"`
	Truncated bool       `json:"truncated"`
}

// dirEntry is the per-entry shape. size/modTime are pointers so they are nil
// when the field is absent (concise mode).
type dirEntry struct {
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	IsDir   bool    `json:"isDir"`
	Size    *int64  `json:"size"`
	ModTime *string `json:"modTime"`
}

// parseDirResp unmarshals the first text result into a dirResp.
func parseDirResp(t *testing.T, result *mcp.CallToolResult) dirResp {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}
	text := extractText(t, result)
	var r dirResp
	if err := json.Unmarshal([]byte(text), &r); err != nil {
		t.Fatalf("json unmarshal: %v\nraw: %s", err, text)
	}
	return r
}

// ─── existing tests (updated to use new envelope fields) ───────────────────

func TestListDirectoryHandler_Root(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	// Pass a high limit so the fixture vault is never truncated in this test.
	result, err := handler(context.Background(), makeRequestKV("path", "", "limit", 200))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)
	if len(resp.Entries) == 0 {
		t.Fatal("expected at least one entry in vault root")
	}
	if resp.Count != len(resp.Entries) {
		t.Errorf("count=%d but len(entries)=%d", resp.Count, len(resp.Entries))
	}
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

	result, err := handler(context.Background(), makeRequestKV("path", "Notes", "limit", 200))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)

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

// ─── limit / truncation (RED: fails until limit is implemented) ───────────

func TestListDirectoryHandler_LimitCap(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	// Notes/ has many entries; limit=2 must force truncation.
	result, err := handler(context.Background(), makeRequestKV("path", "Notes", "limit", 2))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)

	if len(resp.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Entries))
	}
	if resp.Count != 2 {
		t.Errorf("count should be 2, got %d", resp.Count)
	}
	if !resp.Truncated {
		t.Error("expected Truncated=true when limit < total")
	}
	if resp.Total <= 2 {
		t.Errorf("Total should be > 2, got %d", resp.Total)
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

func TestListDirectoryHandler_LimitZeroUsesDefault(t *testing.T) {
	deps := testDeps(t)
	deps.MaxResults = 3
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequestKV("path", "Notes", "limit", 0))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)

	if len(resp.Entries) > 3 {
		t.Errorf("expected at most 3 entries when MaxResults=3, got %d", len(resp.Entries))
	}
}

func TestListDirectoryHandler_SortedOutput(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequestKV("path", "Notes", "limit", 200))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)

	for i := 1; i < len(resp.Entries); i++ {
		if resp.Entries[i].Name < resp.Entries[i-1].Name {
			t.Errorf("entries not sorted at index %d: %q > %q",
				i, resp.Entries[i-1].Name, resp.Entries[i].Name)
		}
	}
}

func TestListDirectoryHandler_FilterGlob(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequestKV("path", "Notes", "filter", "*.md", "limit", 200))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)

	if len(resp.Entries) == 0 {
		t.Fatal("expected at least one *.md entry")
	}
	for _, e := range resp.Entries {
		name := e.Name
		if len(name) < 3 || name[len(name)-3:] != ".md" {
			t.Errorf("filter=*.md returned non-.md entry: %q", name)
		}
	}
}

func TestListDirectoryHandler_TypeFiles(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequestKV("path", "Notes", "type", "files", "limit", 200))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)

	if len(resp.Entries) == 0 {
		t.Fatal("expected file entries")
	}
	for _, e := range resp.Entries {
		if e.IsDir {
			t.Errorf("type=files returned directory entry: %q", e.Name)
		}
	}
}

func TestListDirectoryHandler_OffsetPagination(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	r1, err := handler(context.Background(), makeRequestKV("path", "Notes", "type", "files", "limit", 2, "offset", 0))
	if err != nil {
		t.Fatalf("handler error p1: %v", err)
	}
	resp1 := parseDirResp(t, r1)

	r2, err := handler(context.Background(), makeRequestKV("path", "Notes", "type", "files", "limit", 2, "offset", 2))
	if err != nil {
		t.Fatalf("handler error p2: %v", err)
	}
	resp2 := parseDirResp(t, r2)

	p1Names := map[string]bool{}
	for _, e := range resp1.Entries {
		p1Names[e.Name] = true
	}
	for _, e := range resp2.Entries {
		if p1Names[e.Name] {
			t.Errorf("entry %q appeared on both page 1 and page 2", e.Name)
		}
	}
	if resp1.Total != resp2.Total {
		t.Errorf("Total should be stable across pages: p1=%d p2=%d", resp1.Total, resp2.Total)
	}
}

func TestListDirectoryHandler_ConciseTrueOmitsMetadata(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequestKV("path", "Notes", "concise", true, "limit", 200))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)

	for _, e := range resp.Entries {
		if e.Size != nil {
			t.Errorf("concise=true: expected no size field for %q", e.Name)
		}
		if e.ModTime != nil {
			t.Errorf("concise=true: expected no modTime field for %q", e.Name)
		}
	}
}

func TestListDirectoryHandler_ConciseFalseIncludesMetadata(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListDirectoryHandler(deps)

	result, err := handler(context.Background(), makeRequestKV("path", "Notes", "concise", false, "limit", 200))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	resp := parseDirResp(t, result)

	for _, e := range resp.Entries {
		if e.IsDir {
			continue
		}
		if e.Size == nil {
			t.Errorf("concise=false: expected size field for %q", e.Name)
		}
		if e.ModTime == nil {
			t.Errorf("concise=false: expected modTime field for %q", e.Name)
		}
	}
}
