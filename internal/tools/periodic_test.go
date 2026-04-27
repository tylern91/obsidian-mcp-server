package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/tylern91/obsidian-mcp-server/internal/periodic"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// mockPeriodic is a fake PeriodicService for tool handler tests.
type mockPeriodic struct {
	resolvePath string
	resolveErr  error
	dates       []time.Time
	datesErr    error
}

func (m *mockPeriodic) Resolve(granularity string, offset int) (string, error) {
	return m.resolvePath, m.resolveErr
}

func (m *mockPeriodic) RecentDates(granularity string, count int) ([]time.Time, error) {
	return m.dates, m.datesErr
}

// periodicTestDeps wires a real vault against testdata plus a mock periodic service.
func periodicTestDeps(t *testing.T, p tools.PeriodicService) tools.Deps {
	t.Helper()
	root := "../../testdata/vault"
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return tools.Deps{
		Vault:       vault.New(root, filter),
		Periodic:    p,
		PrettyPrint: false,
		MaxResults:  20,
	}
}

// periodicWriteDeps wires a temp-dir vault plus a real periodic service
// pointing at that same temp dir.
func periodicWriteDeps(t *testing.T) tools.Deps {
	t.Helper()
	dir := t.TempDir()
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	// Wire a real periodic service with a fixed clock
	svc := periodic.New(dir).WithClock(func() time.Time {
		return time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	})
	return tools.Deps{
		Vault:       vault.New(dir, filter),
		Periodic:    svc,
		PrettyPrint: false,
		MaxResults:  20,
	}
}

// ─── get_periodic_note ───────────────────────────────────────────────────────

func TestGetPeriodicNoteHandler_Exists(t *testing.T) {
	// The daily note 2024-01-15 exists in testdata/vault/Daily Notes/
	mock := &mockPeriodic{resolvePath: "Daily Notes/2024-01-15.md"}
	deps := periodicTestDeps(t, mock)
	handler := tools.GetPeriodicNoteHandler(deps)

	result, err := handler(context.Background(), makeRequest("granularity", "daily", "offset", "0"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractResultText(t, result)
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, text)
	}

	if resp["exists"] != true {
		t.Errorf("expected exists=true, got %v", resp["exists"])
	}
	if resp["path"] != "Daily Notes/2024-01-15.md" {
		t.Errorf("expected path='Daily Notes/2024-01-15.md', got %v", resp["path"])
	}
	if resp["content"] == nil {
		t.Error("expected content field to be present")
	}
}

func TestGetPeriodicNoteHandler_NotExists(t *testing.T) {
	mock := &mockPeriodic{resolvePath: "Daily Notes/2099-12-31.md"}
	deps := periodicTestDeps(t, mock)
	handler := tools.GetPeriodicNoteHandler(deps)

	result, err := handler(context.Background(), makeRequest("granularity", "daily", "offset", "0"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractResultText(t, result)
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, text)
	}

	if resp["exists"] != false {
		t.Errorf("expected exists=false, got %v", resp["exists"])
	}
	if resp["path"] != "Daily Notes/2099-12-31.md" {
		t.Errorf("expected path in response, got %v", resp["path"])
	}
}

func TestGetPeriodicNoteHandler_CreateIfMissing(t *testing.T) {
	// Use a fresh temp vault; the daily note won't exist yet.
	deps := periodicWriteDeps(t)
	handler := tools.GetPeriodicNoteHandler(deps)

	result, err := handler(context.Background(), makeRequest(
		"granularity", "daily",
		"offset", "0",
		"createIfMissing", "true",
	))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractResultText(t, result)
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, text)
	}

	if resp["exists"] != true {
		t.Errorf("expected exists=true after create, got %v", resp["exists"])
	}
}

// ─── get_recent_periodic_notes ───────────────────────────────────────────────

func TestGetRecentPeriodicNotes_Summary(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	mock := &mockPeriodic{
		resolvePath: "Daily Notes/2024-01-15.md",
		dates:       dates,
	}
	deps := periodicTestDeps(t, mock)
	handler := tools.GetRecentPeriodicNotesHandler(deps)

	result, err := handler(context.Background(), makeRequest(
		"granularity", "daily",
		"count", "1",
		"summary", "true",
	))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractResultText(t, result)
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, text)
	}

	notes, ok := resp["notes"].([]any)
	if !ok || len(notes) == 0 {
		t.Fatalf("expected non-empty notes array, got %v", resp["notes"])
	}

	entry, ok := notes[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map entry, got %T", notes[0])
	}

	// summary=true means headOf should be present, content absent
	if entry["headOf"] == nil {
		t.Error("expected headOf field with summary=true")
	}
	if entry["content"] != nil {
		t.Error("expected no content field with summary=true")
	}
}

func TestGetRecentPeriodicNotes_Full(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	mock := &mockPeriodic{
		resolvePath: "Daily Notes/2024-01-15.md",
		dates:       dates,
	}
	deps := periodicTestDeps(t, mock)
	handler := tools.GetRecentPeriodicNotesHandler(deps)

	result, err := handler(context.Background(), makeRequest(
		"granularity", "daily",
		"count", "1",
		"summary", "false",
	))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractResultText(t, result)
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, text)
	}

	notes, ok := resp["notes"].([]any)
	if !ok || len(notes) == 0 {
		t.Fatalf("expected non-empty notes array, got %v", resp["notes"])
	}

	entry, ok := notes[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map entry, got %T", notes[0])
	}

	// summary=false means content should be present, headOf absent
	if entry["content"] == nil {
		t.Error("expected content field with summary=false")
	}
	if entry["headOf"] != nil {
		t.Error("expected no headOf field with summary=false")
	}
}

// Test that the real periodic service resolves via the testdata vault path
func TestGetPeriodicNote_RealService(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "vault")
	svc := periodic.New(root).WithClock(func() time.Time {
		return time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	})
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	deps := tools.Deps{
		Vault:       vault.New(root, filter),
		Periodic:    svc,
		PrettyPrint: false,
		MaxResults:  20,
	}
	handler := tools.GetPeriodicNoteHandler(deps)

	result, err := handler(context.Background(), makeRequest("granularity", "daily", "offset", "0"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractResultText(t, result)
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, text)
	}

	if resp["exists"] != true {
		t.Errorf("expected exists=true for known daily note, got %v", resp["exists"])
	}
}
