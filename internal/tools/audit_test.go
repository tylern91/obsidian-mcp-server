package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
)

// extractAuditText is a helper to get the text payload from a handler result.
func extractAuditText(t *testing.T, result *mcp.CallToolResult) string {
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

// auditResponse is the shape returned by audit_notes.
type auditResponse struct {
	Orphans         []auditEntry `json:"orphans"`
	DanglingLinks   []auditEntry `json:"dangling-links"`
	Untagged        []auditEntry `json:"untagged"`
	DuplicateTitles []auditEntry `json:"duplicate-titles"`
	Truncated       bool         `json:"truncated"`
}

type auditEntry struct {
	Path   string `json:"path"`
	Detail string `json:"detail"`
}

func TestAuditNotesHandler_Orphans(t *testing.T) {
	deps := testDeps(t)
	handler := tools.AuditNotesHandler(deps)

	req := makeRequest("classes", `["orphans"]`)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", extractAuditText(t, result))
	}

	raw := extractAuditText(t, result)
	var resp auditResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("json unmarshal: %v\n  raw: %s", err, raw)
	}

	// orphan.md has no tags and no incoming links.
	found := false
	for _, e := range resp.Orphans {
		if e.Path == "Notes/orphan.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Notes/orphan.md in orphans, got: %v", resp.Orphans)
	}

	// DuplicateTitles should be absent (only orphans requested).
	if resp.DuplicateTitles != nil {
		t.Errorf("expected no duplicate-titles key when not requested, got: %v", resp.DuplicateTitles)
	}
}

func TestAuditNotesHandler_DanglingLinks(t *testing.T) {
	deps := testDeps(t)
	handler := tools.AuditNotesHandler(deps)

	req := makeRequest("classes", `["dangling-links"]`)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", extractAuditText(t, result))
	}

	raw := extractAuditText(t, result)
	var resp auditResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("json unmarshal: %v\n  raw: %s", err, raw)
	}

	found := false
	for _, e := range resp.DanglingLinks {
		if e.Path == "Notes/dangling.md" && strings.Contains(e.Detail, "NonExistentNote") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Notes/dangling.md with NonExistentNote in dangling-links, got: %v", resp.DanglingLinks)
	}
}

func TestAuditNotesHandler_Untagged(t *testing.T) {
	deps := testDeps(t)
	handler := tools.AuditNotesHandler(deps)

	req := makeRequest("classes", `["untagged"]`)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", extractAuditText(t, result))
	}

	raw := extractAuditText(t, result)
	var resp auditResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("json unmarshal: %v\n  raw: %s", err, raw)
	}

	// Both Notes/untagged.md and Notes/orphan.md have no tags.
	found := false
	for _, e := range resp.Untagged {
		if e.Path == "Notes/untagged.md" || e.Path == "Notes/orphan.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one untagged note (Notes/untagged.md or Notes/orphan.md), got: %v", resp.Untagged)
	}
}

func TestAuditNotesHandler_DuplicateTitles(t *testing.T) {
	deps := testDeps(t)
	handler := tools.AuditNotesHandler(deps)

	req := makeRequest("classes", `["duplicate-titles"]`)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", extractAuditText(t, result))
	}

	raw := extractAuditText(t, result)
	var resp auditResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("json unmarshal: %v\n  raw: %s", err, raw)
	}

	// Both Notes/duplicate.md and Notes/Deep/duplicate.md should appear.
	foundRoot, foundDeep := false, false
	for _, e := range resp.DuplicateTitles {
		if e.Path == "Notes/duplicate.md" {
			foundRoot = true
		}
		if e.Path == "Notes/Deep/duplicate.md" {
			foundDeep = true
		}
	}
	if !foundRoot {
		t.Errorf("expected Notes/duplicate.md in duplicate-titles, got: %v", resp.DuplicateTitles)
	}
	if !foundDeep {
		t.Errorf("expected Notes/Deep/duplicate.md in duplicate-titles, got: %v", resp.DuplicateTitles)
	}
}

func TestAuditNotesHandler_SpecificClass(t *testing.T) {
	deps := testDeps(t)
	handler := tools.AuditNotesHandler(deps)

	// Request only orphans — other keys should be absent in the JSON.
	req := makeRequest("classes", `["orphans"]`)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", extractAuditText(t, result))
	}

	raw := extractAuditText(t, result)

	// Verify the JSON only has the orphans key (plus truncated).
	var raw2 map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &raw2); err != nil {
		t.Fatalf("json unmarshal raw2: %v", err)
	}
	if _, ok := raw2["dangling-links"]; ok {
		t.Error("unexpected key 'dangling-links' in response when not requested")
	}
	if _, ok := raw2["untagged"]; ok {
		t.Error("unexpected key 'untagged' in response when not requested")
	}
	if _, ok := raw2["duplicate-titles"]; ok {
		t.Error("unexpected key 'duplicate-titles' in response when not requested")
	}
	if _, ok := raw2["orphans"]; !ok {
		t.Error("expected key 'orphans' in response")
	}
}

func TestAuditNotesHandler_Limit(t *testing.T) {
	deps := testDeps(t)
	handler := tools.AuditNotesHandler(deps)

	// The vault has multiple untagged notes (orphan.md, untagged.md, dangling.md, duplicate.md, Deep/duplicate.md, simple.md ...).
	// Limit to 1 and verify truncated=true.
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"classes": `["untagged"]`,
			"limit":   float64(1),
		}},
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", extractAuditText(t, result))
	}

	raw := extractAuditText(t, result)
	var resp auditResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("json unmarshal: %v\n  raw: %s", err, raw)
	}

	if !resp.Truncated {
		t.Error("expected truncated=true when limit is hit")
	}
	if len(resp.Untagged) > 1 {
		t.Errorf("expected at most 1 untagged result, got %d", len(resp.Untagged))
	}
}

func TestAuditNotesHandler_AllClasses_Default(t *testing.T) {
	deps := testDeps(t)
	handler := tools.AuditNotesHandler(deps)

	// No classes param → all four classes should be present.
	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{}},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", extractAuditText(t, result))
	}

	raw := extractAuditText(t, result)
	var raw2 map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &raw2); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	for _, key := range []string{"orphans", "dangling-links", "untagged", "duplicate-titles"} {
		if _, ok := raw2[key]; !ok {
			t.Errorf("expected key %q in default (all classes) response", key)
		}
	}
}
