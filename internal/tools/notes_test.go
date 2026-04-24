package tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// testDeps returns a Deps wired to the committed testdata fixture vault.
func testDeps(t *testing.T) tools.Deps {
	t.Helper()
	root := "../../testdata/vault"
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return tools.Deps{
		Vault:       vault.New(root, filter),
		PrettyPrint: false,
	}
}

// writeDeps returns a Deps backed by a fresh temp directory.
func writeDeps(t *testing.T) tools.Deps {
	t.Helper()
	dir := t.TempDir()
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return tools.Deps{
		Vault:       vault.New(dir, filter),
		PrettyPrint: false,
	}
}

// makeRequest builds a CallToolRequest from key-value pairs.
func makeRequest(kvs ...string) mcp.CallToolRequest {
	args := make(map[string]any)
	for i := 0; i+1 < len(kvs); i += 2 {
		args[kvs[i]] = kvs[i+1]
	}
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: args},
	}
}

func TestReadNoteHandler_Success(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadNoteHandler(deps)

	result, err := handler(context.Background(), makeRequest("path", "Notes/simple.md"))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError=true: %v", result.Content)
	}

	// Extract text content from result.
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var resp struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		Size       int64  `json:"size"`
		ModTime    string `json:"modTime"`
		TokenCount int    `json:"tokenCount"`
	}
	if err := json.Unmarshal([]byte(text.Text), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp.Path != "Notes/simple.md" {
		t.Errorf("path = %q, want %q", resp.Path, "Notes/simple.md")
	}
	if resp.Content == "" {
		t.Error("expected non-empty content")
	}
	if resp.TokenCount <= 0 {
		t.Errorf("tokenCount = %d, want > 0", resp.TokenCount)
	}
	if resp.Size <= 0 {
		t.Errorf("size = %d, want > 0", resp.Size)
	}
	if _, parseErr := time.Parse(time.RFC3339, resp.ModTime); parseErr != nil {
		t.Errorf("modTime %q is not valid RFC3339: %v", resp.ModTime, parseErr)
	}
}

func TestReadNoteHandler_NotFound(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadNoteHandler(deps)

	result, err := handler(context.Background(), makeRequest("path", "Notes/nonexistent.md"))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for missing note")
	}
}

func TestReadNoteHandler_PathRequired(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadNoteHandler(deps)

	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{}},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when path is missing")
	}
}

func TestWriteNoteHandler_Overwrite(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.WriteNoteHandler(deps)

	req := makeRequest("path", "note.md", "content", "hello world", "mode", "overwrite")
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	// Read back via vault.
	note, readErr := deps.Vault.ReadNote(context.Background(), "note.md")
	if readErr != nil {
		t.Fatalf("read back failed: %v", readErr)
	}
	if note.Content != "hello world" {
		t.Errorf("content = %q, want %q", note.Content, "hello world")
	}
}

func TestWriteNoteHandler_Append(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.WriteNoteHandler(deps)

	// Write initial content.
	req1 := makeRequest("path", "note.md", "content", "first\n", "mode", "overwrite")
	if r, err := handler(context.Background(), req1); err != nil || r.IsError {
		t.Fatalf("initial write failed: err=%v isError=%v", err, r.IsError)
	}

	// Append.
	req2 := makeRequest("path", "note.md", "content", "second\n", "mode", "append")
	result, err := handler(context.Background(), req2)
	if err != nil {
		t.Fatalf("append error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error on append: %v", result.Content)
	}

	note, readErr := deps.Vault.ReadNote(context.Background(), "note.md")
	if readErr != nil {
		t.Fatalf("read back failed: %v", readErr)
	}
	const want = "first\nsecond\n"
	if note.Content != want {
		t.Errorf("content = %q, want %q", note.Content, want)
	}
}

func TestWriteNoteHandler_Prepend(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.WriteNoteHandler(deps)

	// Write initial content.
	req1 := makeRequest("path", "note.md", "content", "body\n", "mode", "overwrite")
	if r, err := handler(context.Background(), req1); err != nil || r.IsError {
		t.Fatalf("initial write failed: err=%v isError=%v", err, r.IsError)
	}

	// Prepend a prefix.
	req2 := makeRequest("path", "note.md", "content", "prefix\n", "mode", "prepend")
	result, err := handler(context.Background(), req2)
	if err != nil {
		t.Fatalf("prepend error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error on prepend: %v", result.Content)
	}

	note, readErr := deps.Vault.ReadNote(context.Background(), "note.md")
	if readErr != nil {
		t.Fatalf("read back failed: %v", readErr)
	}
	const want = "prefix\nbody\n"
	if note.Content != want {
		t.Errorf("content = %q, want %q", note.Content, want)
	}
	if note.Content[:len("prefix\n")] != "prefix\n" {
		t.Error("expected prepended content at start of file")
	}
}

func TestWriteNoteHandler_ContentRequired(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.WriteNoteHandler(deps)

	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{"path": "note.md"}},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when content is missing")
	}
}

func TestWriteNoteHandler_PathRequired(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.WriteNoteHandler(deps)

	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{"content": "hello"}},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when path is missing")
	}
}

func TestWriteNoteHandler_Traversal(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.WriteNoteHandler(deps)

	result, err := handler(context.Background(), makeRequest("path", "../escape.md", "content", "evil", "mode", "overwrite"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for path traversal attempt")
	}
}
