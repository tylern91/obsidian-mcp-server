package tools_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// testDeps returns a Deps wired to the committed testdata fixture vault.
func testDeps(t *testing.T) tools.Deps {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "vault")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return tools.Deps{
		Vault:       vault.New(abs, filter),
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

	// Extract text content from result
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var resp struct {
		Path    string `json:"path"`
		Content string `json:"content"`
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

	// Read back via vault
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

// copyDir recursively copies src directory to dst.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("copyDir ReadDir: %v", err)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("copyDir MkdirAll: %v", err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
		} else {
			copyFile(t, srcPath, dstPath)
		}
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("copyFile Open: %v", err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("copyFile Create: %v", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copyFile Copy: %v", err)
	}
}
