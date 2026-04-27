package prompts_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/prompts"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// vaultDeps returns Deps wired to the committed testdata fixture vault.
func vaultDeps(t *testing.T) prompts.Deps {
	t.Helper()
	root := "../../testdata/vault"
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return prompts.Deps{
		Vault: vault.New(root, filter),
	}
}

func makePromptRequest(args map[string]string) mcp.GetPromptRequest {
	return mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: args,
		},
	}
}

// --- summarize_note ---

func TestSummarizeNote_Success(t *testing.T) {
	deps := vaultDeps(t)
	handler := prompts.SummarizeNoteHandler(deps)

	result, err := handler(context.Background(), makePromptRequest(map[string]string{
		"path": "Notes/simple.md",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one prompt message")
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "simple.md") && !strings.Contains(text, "Notes/simple") {
		t.Errorf("expected note path in prompt text, got: %s", text[:min(200, len(text))])
	}
}

func TestSummarizeNote_MissingPath(t *testing.T) {
	deps := vaultDeps(t)
	handler := prompts.SummarizeNoteHandler(deps)

	result, err := handler(context.Background(), makePromptRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Errorf("expected error message, got: %s", text)
	}
}

func TestSummarizeNote_NotFound(t *testing.T) {
	deps := vaultDeps(t)
	handler := prompts.SummarizeNoteHandler(deps)

	result, err := handler(context.Background(), makePromptRequest(map[string]string{
		"path": "nonexistent/note.md",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Errorf("expected error message for missing note, got: %s", text)
	}
}

// --- find_related ---

func TestFindRelated_Success(t *testing.T) {
	deps := vaultDeps(t)
	handler := prompts.FindRelatedHandler(deps)

	result, err := handler(context.Background(), makePromptRequest(map[string]string{
		"path": "Notes/tagged.md",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected prompt message")
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "tagged.md") && !strings.Contains(text, "Notes/tagged") {
		t.Errorf("expected note path in prompt, got: %s", text[:min(200, len(text))])
	}
}

func TestFindRelated_MissingPath(t *testing.T) {
	deps := vaultDeps(t)
	handler := prompts.FindRelatedHandler(deps)

	result, err := handler(context.Background(), makePromptRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Errorf("expected error message, got: %s", text)
	}
}

// --- vault_health_check ---

func TestVaultHealthCheck_ReturnsReport(t *testing.T) {
	deps := vaultDeps(t)
	handler := prompts.VaultHealthCheckHandler(deps)

	result, err := handler(context.Background(), makePromptRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected prompt message")
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	// Should contain at least the report header
	if !strings.Contains(text, "notes total") {
		t.Errorf("expected vault health report, got: %s", text[:min(300, len(text))])
	}
	// testdata vault has orphan.md, untagged.md — should appear in report
	if !strings.Contains(text, "Orphaned") && !strings.Contains(text, "Untagged") {
		t.Errorf("expected orphan/untagged sections in report")
	}
}

// --- extractWikilinks ---

func TestExtractWikilinks(t *testing.T) {
	cases := []struct {
		content string
		want    []string
	}{
		{"[[Foo]] and [[Bar]]", []string{"Foo", "Bar"}},
		{"[[Note|Alias]]", []string{"Note"}},
		{"no links here", nil},
		{"[[Dup]] and [[Dup]]", []string{"Dup"}}, // deduped
	}
	for _, tc := range cases {
		got := prompts.ExtractWikilinks(tc.content)
		if len(got) != len(tc.want) {
			t.Errorf("content=%q: got %v, want %v", tc.content, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("content=%q index %d: got %q, want %q", tc.content, i, got[i], tc.want[i])
			}
		}
	}
}

// --- daily_note_review (with stub periodic service) ---

type stubPeriodic struct {
	resolve func(granularity string, offset int) (string, error)
	recent  func(granularity string, count int) ([]time.Time, error)
}

func (s *stubPeriodic) Resolve(granularity string, offset int) (string, error) {
	return s.resolve(granularity, offset)
}
func (s *stubPeriodic) RecentDates(granularity string, count int) ([]time.Time, error) {
	return s.recent(granularity, count)
}

func TestDailyNoteReview_InvalidOffset(t *testing.T) {
	deps := vaultDeps(t)
	deps.Periodic = &stubPeriodic{
		resolve: func(string, int) (string, error) { return "", nil },
		recent:  func(string, int) ([]time.Time, error) { return nil, nil },
	}
	handler := prompts.DailyNoteReviewHandler(deps)

	result, err := handler(context.Background(), makePromptRequest(map[string]string{
		"offset": "not-a-number",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "Error") {
		t.Errorf("expected error for invalid offset, got: %s", text)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
