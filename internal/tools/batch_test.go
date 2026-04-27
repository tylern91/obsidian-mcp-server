package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
)

// batchDeps returns a Deps with MaxBatch=3 for truncation tests.
func batchDeps(t *testing.T) tools.Deps {
	t.Helper()
	d := testDeps(t)
	d.MaxBatch = 3
	return d
}

// extractResultText pulls the first TextContent.Text from a CallToolResult.
func extractResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return text.Text
}

// makeRequestKV is like makeRequest but accepts any value types (not just string).
func makeRequestKV(kvs ...any) mcp.CallToolRequest {
	args := make(map[string]any)
	for i := 0; i+1 < len(kvs); i += 2 {
		key, _ := kvs[i].(string)
		args[key] = kvs[i+1]
	}
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: args},
	}
}

// ─── read_multiple_notes ────────────────────────────────────────────────────

func TestReadMultipleNotesHandler_SinglePath(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadMultipleNotesHandler(deps)

	paths, _ := json.Marshal([]string{"Notes/simple.md"})
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []struct {
			Path       string  `json:"path"`
			Content    string  `json:"content"`
			Size       int64   `json:"size"`
			ModTime    string  `json:"modTime"`
			TokenCount int     `json:"tokenCount"`
			Error      *string `json:"error"`
		} `json:"notes"`
		Count     int  `json:"count"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
	if resp.Truncated {
		t.Error("truncated should be false for single path")
	}
	if len(resp.Notes) != 1 {
		t.Fatalf("notes len = %d, want 1", len(resp.Notes))
	}
	n := resp.Notes[0]
	if n.Path != "Notes/simple.md" {
		t.Errorf("path = %q, want Notes/simple.md", n.Path)
	}
	if n.Content == "" {
		t.Error("content should be non-empty")
	}
	if n.Size <= 0 {
		t.Errorf("size = %d, want > 0", n.Size)
	}
	if n.TokenCount <= 0 {
		t.Errorf("tokenCount = %d, want > 0", n.TokenCount)
	}
	if n.Error != nil {
		t.Errorf("error should be nil, got %v", *n.Error)
	}
}

func TestReadMultipleNotesHandler_MultiplePaths(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadMultipleNotesHandler(deps)

	paths, _ := json.Marshal([]string{"Notes/simple.md", "Notes/with-fm.md"})
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []struct {
			Path  string  `json:"path"`
			Error *string `json:"error"`
		} `json:"notes"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
	if len(resp.Notes) != 2 {
		t.Fatalf("notes len = %d, want 2", len(resp.Notes))
	}
	for _, n := range resp.Notes {
		if n.Error != nil {
			t.Errorf("note %q has unexpected error: %v", n.Path, *n.Error)
		}
	}
}

func TestReadMultipleNotesHandler_SummaryMode(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadMultipleNotesHandler(deps)

	paths, _ := json.Marshal([]string{"Notes/simple.md"})
	req := makeRequestKV("paths", string(paths), "summary", true, "headChars", "50")

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	text := extractResultText(t, result)

	var resp struct {
		Notes []json.RawMessage `json:"notes"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if len(resp.Notes) == 0 {
		t.Fatal("expected at least one note")
	}

	var note map[string]json.RawMessage
	if err := json.Unmarshal(resp.Notes[0], &note); err != nil {
		t.Fatalf("unmarshal note: %v", err)
	}

	headOfRaw, hasHeadOf := note["headOf"]
	_, hasContent := note["content"]

	if !hasHeadOf {
		t.Error("expected headOf field in summary mode")
	}
	if hasContent {
		t.Error("content field should not be present in summary mode")
	}

	if hasHeadOf {
		var headOf string
		if err := json.Unmarshal(headOfRaw, &headOf); err != nil {
			t.Fatalf("unmarshal headOf: %v", err)
		}
		if headOf == "" {
			t.Error("headOf should be non-empty")
		}
		if len([]rune(headOf)) > 50 {
			t.Errorf("headOf too long: %d runes, want <= 50", len([]rune(headOf)))
		}
	}
}

func TestReadMultipleNotesHandler_BatchCap(t *testing.T) {
	deps := batchDeps(t) // MaxBatch = 3
	handler := tools.ReadMultipleNotesHandler(deps)

	// 5 paths, but MaxBatch = 3 → truncated
	p := []string{
		"Notes/simple.md",
		"Notes/with-fm.md",
		"Notes/tagged.md",
		"Notes/simple.md",
		"Notes/with-fm.md",
	}
	paths, _ := json.Marshal(p)
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []struct {
			Path string `json:"path"`
		} `json:"notes"`
		Count     int  `json:"count"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if !resp.Truncated {
		t.Error("expected truncated=true when more paths than MaxBatch")
	}
	if resp.Count != 3 {
		t.Errorf("count = %d, want 3", resp.Count)
	}
	if len(resp.Notes) != 3 {
		t.Errorf("notes len = %d, want 3", len(resp.Notes))
	}
}

func TestReadMultipleNotesHandler_NonExistentPath(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadMultipleNotesHandler(deps)

	paths, _ := json.Marshal([]string{"Notes/simple.md", "Notes/does-not-exist.md"})
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatal("overall result should not be an error when some paths fail")
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []struct {
			Path  string  `json:"path"`
			Error *string `json:"error"`
		} `json:"notes"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
	found := map[string]bool{}
	for _, n := range resp.Notes {
		found[n.Path] = (n.Error == nil)
	}
	if !found["Notes/simple.md"] {
		t.Error("Notes/simple.md should succeed (error == nil)")
	}
	// "does-not-exist.md" should have error set => found value is false
	if found["Notes/does-not-exist.md"] {
		t.Error("Notes/does-not-exist.md should have error field set")
	}
}

func TestReadMultipleNotesHandler_EmptyPaths(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadMultipleNotesHandler(deps)

	paths, _ := json.Marshal([]string{})
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []json.RawMessage `json:"notes"`
		Count int               `json:"count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if resp.Count != 0 {
		t.Errorf("count = %d, want 0", resp.Count)
	}
	if len(resp.Notes) != 0 {
		t.Errorf("notes len = %d, want 0", len(resp.Notes))
	}
}

// ─── get_notes_info ─────────────────────────────────────────────────────────

func TestGetNotesInfoHandler_SinglePath(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetNotesInfoHandler(deps)

	paths, _ := json.Marshal([]string{"Notes/simple.md"})
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []struct {
			Path      string  `json:"path"`
			Size      int64   `json:"size"`
			ModTime   string  `json:"modTime"`
			Title     string  `json:"title"`
			TagCount  int     `json:"tagCount"`
			LinkCount int     `json:"linkCount"`
			Error     *string `json:"error"`
		} `json:"notes"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
	if len(resp.Notes) != 1 {
		t.Fatalf("notes len = %d, want 1", len(resp.Notes))
	}
	n := resp.Notes[0]
	if n.Path != "Notes/simple.md" {
		t.Errorf("path = %q, want Notes/simple.md", n.Path)
	}
	if n.Size <= 0 {
		t.Errorf("size = %d, want > 0", n.Size)
	}
	if n.Error != nil {
		t.Errorf("error should be nil, got %v", *n.Error)
	}
}

func TestGetNotesInfoHandler_TitleFromFrontmatter(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetNotesInfoHandler(deps)

	// with-fm.md has frontmatter: title: "Note With Frontmatter", tags: [research, ideas]
	paths, _ := json.Marshal([]string{"Notes/with-fm.md"})
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []struct {
			Title    string `json:"title"`
			TagCount int    `json:"tagCount"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if len(resp.Notes) == 0 {
		t.Fatal("expected one note")
	}
	n := resp.Notes[0]
	if n.Title != "Note With Frontmatter" {
		t.Errorf("title = %q, want %q", n.Title, "Note With Frontmatter")
	}
	if n.TagCount < 2 {
		t.Errorf("tagCount = %d, want >= 2 (research, ideas)", n.TagCount)
	}
}

func TestGetNotesInfoHandler_TitleFromFilename(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetNotesInfoHandler(deps)

	// simple.md has no frontmatter, so title should be filename stem "simple"
	paths, _ := json.Marshal([]string{"Notes/simple.md"})
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []struct {
			Title string `json:"title"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if len(resp.Notes) == 0 {
		t.Fatal("expected one note")
	}
	if resp.Notes[0].Title != "simple" {
		t.Errorf("title = %q, want %q", resp.Notes[0].Title, "simple")
	}
}

func TestGetNotesInfoHandler_NonExistentPath(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetNotesInfoHandler(deps)

	paths, _ := json.Marshal([]string{"Notes/does-not-exist.md"})
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatal("overall result should not be an error for missing paths")
	}

	text := extractResultText(t, result)
	var resp struct {
		Notes []struct {
			Path  string  `json:"path"`
			Error *string `json:"error"`
		} `json:"notes"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, text)
	}
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
	if len(resp.Notes) == 0 {
		t.Fatal("expected one note entry")
	}
	n := resp.Notes[0]
	if n.Error == nil {
		t.Error("expected error field to be set for missing note")
	}
}
