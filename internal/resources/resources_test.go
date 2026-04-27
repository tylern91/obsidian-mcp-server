package resources_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/resources"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func vaultDeps(t *testing.T) resources.Deps {
	t.Helper()
	root := "../../testdata/vault"
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return resources.Deps{
		Vault:       vault.New(root, filter),
		PrettyPrint: false,
	}
}

func makeResourceRequest(uri string) mcp.ReadResourceRequest {
	return mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	}
}

// --- pathFromURI ---

func TestPathFromURI(t *testing.T) {
	cases := []struct {
		uri, prefix, want string
	}{
		{"obsidian://note/Notes/foo.md", "obsidian://note/", "Notes/foo.md"},
		{"obsidian://backlinks/Notes/bar.md", "obsidian://backlinks/", "Notes/bar.md"},
		{"obsidian://note/", "obsidian://note/", ""},
		{"obsidian://other/x", "obsidian://note/", ""},
	}
	for _, tc := range cases {
		got := resources.PathFromURI(tc.uri, tc.prefix)
		if got != tc.want {
			t.Errorf("PathFromURI(%q, %q) = %q, want %q", tc.uri, tc.prefix, got, tc.want)
		}
	}
}

// --- obsidian://vault/stats ---

func TestVaultStats_ReturnsJSON(t *testing.T) {
	deps := vaultDeps(t)
	handler := resources.VaultStatsHandler(deps)

	contents, err := handler(context.Background(), makeResourceRequest("obsidian://vault/stats"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) == 0 {
		t.Fatal("expected resource contents")
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}
	if tc.MIMEType != "application/json" {
		t.Errorf("expected application/json MIME, got %q", tc.MIMEType)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Errorf("response is not valid JSON: %v\ntext: %s", err, tc.Text)
	}
	if _, ok := m["noteCount"]; !ok {
		t.Error("expected noteCount in stats response")
	}
}

// --- obsidian://vault/tags ---

func TestVaultTags_ReturnsJSON(t *testing.T) {
	deps := vaultDeps(t)
	handler := resources.VaultTagsHandler(deps)

	contents, err := handler(context.Background(), makeResourceRequest("obsidian://vault/tags"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
	if _, ok := m["total"]; !ok {
		t.Error("expected total in tags response")
	}
}

// --- obsidian://note/{path} ---

func TestNoteResource_ExistingNote(t *testing.T) {
	deps := vaultDeps(t)
	handler := resources.NoteResourceHandler(deps)

	contents, err := handler(context.Background(), makeResourceRequest("obsidian://note/Notes/simple.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}
	if tc.MIMEType != "text/markdown" {
		t.Errorf("expected text/markdown, got %q", tc.MIMEType)
	}
	if tc.Text == "" {
		t.Error("expected non-empty note content")
	}
}

func TestNoteResource_MissingNote(t *testing.T) {
	deps := vaultDeps(t)
	handler := resources.NoteResourceHandler(deps)

	contents, err := handler(context.Background(), makeResourceRequest("obsidian://note/nonexistent.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}
	if !strings.Contains(tc.Text, "error") && !strings.Contains(tc.Text, "Error") {
		t.Errorf("expected error in response for missing note, got: %s", tc.Text)
	}
}

// --- obsidian://backlinks/{path} ---

func TestBacklinksResource_ExistingNote(t *testing.T) {
	deps := vaultDeps(t)
	handler := resources.BacklinksResourceHandler(deps)

	contents, err := handler(context.Background(), makeResourceRequest("obsidian://backlinks/Notes/simple.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}
	if tc.MIMEType != "application/json" {
		t.Errorf("expected application/json, got %q", tc.MIMEType)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
	if _, ok := m["target"]; !ok {
		t.Error("expected target field in backlinks response")
	}
}

// --- obsidian://periodic/{granularity} (stub periodic) ---

type stubPeriodic struct {
	path string
	err  error
}

func (s *stubPeriodic) Resolve(_ string, _ int) (string, error) { return s.path, s.err }
func (s *stubPeriodic) RecentDates(_ string, _ int) ([]time.Time, error) {
	return nil, nil
}

func TestPeriodicResource_NoteNotFound(t *testing.T) {
	deps := vaultDeps(t)
	deps.Periodic = &stubPeriodic{path: "Daily Notes/9999-99-99.md"}
	handler := resources.PeriodicResourceHandler(deps)

	contents, err := handler(context.Background(), makeResourceRequest("obsidian://periodic/daily"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}
	// Missing note should return an HTML comment, not an error payload.
	if !strings.Contains(tc.Text, "does not exist") {
		t.Errorf("expected 'does not exist' message, got: %s", tc.Text)
	}
}
