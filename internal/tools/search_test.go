package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// searchDeps returns a Deps wired to the committed testdata fixture vault,
// including a real search.Service.
func searchDeps(t *testing.T) tools.Deps {
	t.Helper()
	root := "../../testdata/vault"
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	vaultSvc := vault.New(root, filter)
	return tools.Deps{
		Vault:       vaultSvc,
		Search:      search.New(vaultSvc),
		PrettyPrint: false,
	}
}

// makeRequestMixed builds a CallToolRequest from alternating key, value pairs
// where values may be any type (string, bool, int, float64, etc.).
func makeRequestMixed(kvs ...any) mcp.CallToolRequest {
	args := make(map[string]any)
	for i := 0; i+1 < len(kvs); i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			continue
		}
		args[key] = kvs[i+1]
	}
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: args},
	}
}

// --- search_notes tests ---

func TestSearchNotesHandler_BasicQuery(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchNotesHandler(deps)

	// "machine learning" appears in Search/ml-intro.md and Search/ml-basics.md
	result, err := handler(context.Background(), makeRequest("query", "machine learning"))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError=true: %v", result.Content)
	}

	text := extractText(t, result)

	var resp struct {
		Query   string `json:"query"`
		Total   int    `json:"total"`
		Results []struct {
			Path       string  `json:"path"`
			Score      float64 `json:"score"`
			MatchCount int     `json:"matchCount"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp.Query != "machine learning" {
		t.Errorf("query = %q, want %q", resp.Query, "machine learning")
	}
	if resp.Total == 0 {
		t.Error("expected at least one result for 'machine learning'")
	}
	if resp.Total != len(resp.Results) {
		t.Errorf("total=%d but len(results)=%d", resp.Total, len(resp.Results))
	}
	// Verify each result has a positive score.
	for _, r := range resp.Results {
		if r.Score <= 0 {
			t.Errorf("result %q has non-positive score %f", r.Path, r.Score)
		}
	}
}

func TestSearchNotesHandler_QueryRequired(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchNotesHandler(deps)

	result, err := handler(context.Background(), makeRequest())
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when query is missing")
	}
}

func TestSearchNotesHandler_PrettyPrint(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchNotesHandler(deps)

	req := makeRequestMixed("query", "machine", "prettyPrint", true)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	text := extractText(t, result)
	if len(text) == 0 {
		t.Fatal("empty response text")
	}
	var js json.RawMessage
	if err := json.Unmarshal([]byte(text), &js); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	// Pretty-printed JSON contains newlines.
	foundIndent := false
	for _, c := range text {
		if c == '\n' {
			foundIndent = true
			break
		}
	}
	if !foundIndent {
		t.Error("expected indented JSON (newlines) for prettyPrint=true")
	}
}

func TestSearchNotesHandler_PathScope(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchNotesHandler(deps)

	req := makeRequestMixed("query", "machine", "pathScope", "Search/*")
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	text := extractText(t, result)
	var resp struct {
		Results []struct {
			Path string `json:"path"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	// All results should be under Search/
	for _, r := range resp.Results {
		if len(r.Path) < 7 || r.Path[:7] != "Search/" {
			t.Errorf("result path %q is outside Search/ scope", r.Path)
		}
	}
}

// --- search_regex tests ---

func TestSearchRegexHandler_ContentMatch(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchRegexHandler(deps)

	// "machine" should match inside ml-intro.md content
	result, err := handler(context.Background(), makeRequest("pattern", "machine"))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError=true: %v", result.Content)
	}

	text := extractText(t, result)
	var resp struct {
		Pattern string `json:"pattern"`
		Scope   string `json:"scope"`
		Total   int    `json:"total"`
		Results []struct {
			Path    string `json:"path"`
			Matches []struct {
				Line    int    `json:"line"`
				Snippet string `json:"snippet"`
			} `json:"matches"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp.Pattern != "machine" {
		t.Errorf("pattern = %q, want %q", resp.Pattern, "machine")
	}
	if resp.Scope != "content" {
		t.Errorf("scope = %q, want %q", resp.Scope, "content")
	}
	if resp.Total == 0 {
		t.Error("expected at least one result for pattern 'machine'")
	}
	if resp.Total != len(resp.Results) {
		t.Errorf("total=%d but len(results)=%d", resp.Total, len(resp.Results))
	}
	// Each result should have at least one match snippet.
	for _, r := range resp.Results {
		if len(r.Matches) == 0 {
			t.Errorf("result %q has no match snippets", r.Path)
		}
	}
}

func TestSearchRegexHandler_PatternRequired(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchRegexHandler(deps)

	result, err := handler(context.Background(), makeRequest())
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when pattern is missing")
	}
}

func TestSearchRegexHandler_InvalidPattern(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchRegexHandler(deps)

	// An invalid regex should return an error result, not a Go-level error.
	result, err := handler(context.Background(), makeRequest("pattern", "[invalid"))
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for invalid regex pattern")
	}
}

func TestSearchRegexHandler_GlobPattern(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchRegexHandler(deps)

	req := makeRequestMixed("pattern", "Search/*", "isGlob", true, "scope", "path")
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	text := extractText(t, result)
	var resp struct {
		Total   int `json:"total"`
		Results []struct {
			Path string `json:"path"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp.Total == 0 {
		t.Error("expected at least one result for glob 'Search/*' on path scope")
	}
	for _, r := range resp.Results {
		if len(r.Path) < 7 || r.Path[:7] != "Search/" {
			t.Errorf("result path %q is outside Search/ scope", r.Path)
		}
	}
}

func TestSearchRegexHandler_PrettyPrint(t *testing.T) {
	deps := searchDeps(t)
	handler := tools.SearchRegexHandler(deps)

	req := makeRequestMixed("pattern", "machine", "prettyPrint", true)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	text := extractText(t, result)
	foundIndent := false
	for _, c := range text {
		if c == '\n' {
			foundIndent = true
			break
		}
	}
	if !foundIndent {
		t.Error("expected indented JSON (newlines) for prettyPrint=true")
	}
}
