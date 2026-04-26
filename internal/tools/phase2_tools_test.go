package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// mutableDeps copies the testdata fixture vault to a temp dir for write/delete/move tests.
func mutableDeps(t *testing.T) tools.Deps {
	t.Helper()
	src := "../../testdata/vault"
	dst := t.TempDir()
	require.NoError(t, copyDirForTools(src, dst))
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return tools.Deps{
		Vault:       vault.New(dst, filter),
		PrettyPrint: false,
	}
}

func copyDirForTools(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// --- get_frontmatter ---

func TestGetFrontmatterHandler_WithFM(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Notes/with-fm.md"))
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success")
	text := extractText(t, result)
	assert.Contains(t, text, `"frontmatter"`)
	assert.Contains(t, text, `"body"`)
}

func TestGetFrontmatterHandler_NoFM(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Notes/simple.md"))
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success for note without FM")
	text := extractText(t, result)
	// No frontmatter but still returns the structure
	assert.Contains(t, text, `"frontmatter"`)
}

func TestGetFrontmatterHandler_PathRequired(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest())
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestGetFrontmatterHandler_NotFound(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Notes/ghost.md"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- update_frontmatter ---

func TestUpdateFrontmatterHandler_UpdateKey(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.UpdateFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/with-fm.md",
		"updates", `{"title":"Updated Title"}`,
	))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"success":true`)
}

func TestUpdateFrontmatterHandler_PathRequired(t *testing.T) {
	deps := testDeps(t)
	handler := tools.UpdateFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest("updates", `{"k":"v"}`))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestUpdateFrontmatterHandler_Traversal(t *testing.T) {
	deps := testDeps(t)
	handler := tools.UpdateFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "../escape.md", "updates", `{"k":"v"}`))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "traversal")
}

func TestUpdateFrontmatterHandler_InvalidJSON(t *testing.T) {
	deps := testDeps(t)
	handler := tools.UpdateFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Notes/with-fm.md", "updates", "not-json"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- manage_tags ---

func TestManageTagsHandler_AddFrontmatter(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.ManageTagsHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"action", "add",
		"tag", "test-tag",
		"location", "frontmatter",
	))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"success":true`)
}

func TestManageTagsHandler_RemoveTag(t *testing.T) {
	deps := mutableDeps(t)
	// Add then remove to verify idempotency.
	addHandler := tools.ManageTagsHandler(deps)
	_, err := addHandler(context.Background(), makeRequest(
		"path", "Notes/tagged.md",
		"action", "add",
		"tag", "removeme",
	))
	require.NoError(t, err)

	removeHandler := tools.ManageTagsHandler(deps)
	result, err := removeHandler(context.Background(), makeRequest(
		"path", "Notes/tagged.md",
		"action", "remove",
		"tag", "removeme",
	))
	require.NoError(t, err)
	require.False(t, result.IsError)
}

func TestManageTagsHandler_RequiredParams(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ManageTagsHandler(deps)

	// Missing path
	result, err := handler(context.Background(), makeRequest("action", "add", "tag", "x"))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Missing action
	result, err = handler(context.Background(), makeRequest("path", "Notes/simple.md", "tag", "x"))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Missing tag
	result, err = handler(context.Background(), makeRequest("path", "Notes/simple.md", "action", "add"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- list_all_tags ---

func TestListAllTagsHandler_ReturnsTagsWithCounts(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ListAllTagsHandler(deps)
	result, err := handler(context.Background(), makeRequest())
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)

	var resp struct {
		Tags  []map[string]any `json:"tags"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.True(t, resp.Total > 0, "fixture vault has tags")
}

// --- get_backlinks ---

func TestGetBacklinksHandler_ReturnsBacklinks(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetBacklinksHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Notes/simple.md"))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"backlinks"`)
	assert.Contains(t, text, `"total"`)
}

func TestGetBacklinksHandler_PathRequired(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetBacklinksHandler(deps)
	result, err := handler(context.Background(), makeRequest())
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestGetBacklinksHandler_NotFound(t *testing.T) {
	deps := testDeps(t)
	handler := tools.GetBacklinksHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Notes/ghost.md"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- patch_note ---

func TestPatchNoteHandler_ReplaceBody(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.PatchNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"heading", "Simple Note",
		"position", "replace_body",
		"content", "Patched body.",
	))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"success":true`)
}

func TestPatchNoteHandler_HeadingNotFound(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.PatchNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"heading", "No Such Heading",
		"position", "after",
		"content", "x",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, strings.ToLower(text), "heading")
}

func TestPatchNoteHandler_Traversal(t *testing.T) {
	deps := testDeps(t)
	handler := tools.PatchNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "../escape.md",
		"heading", "H",
		"position", "after",
		"content", "x",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestPatchNoteHandler_RequiredParams(t *testing.T) {
	deps := testDeps(t)
	handler := tools.PatchNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Notes/simple.md"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- delete_note ---

func TestDeleteNoteHandler_Success(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.DeleteNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"confirm", "Notes/simple.md",
	))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"success":true`)
}

func TestDeleteNoteHandler_ConfirmMismatch(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.DeleteNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"confirm", "wrong.md",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, strings.ToLower(text), "confirm")
}

func TestDeleteNoteHandler_Traversal(t *testing.T) {
	deps := testDeps(t)
	handler := tools.DeleteNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "../escape.md",
		"confirm", "../escape.md",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- move_note ---

func TestMoveNoteHandler_Success(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.MoveNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"src", "Notes/simple.md",
		"dst", "Archive/simple.md",
		"confirm", "Notes/simple.md",
	))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"success":true`)
	assert.Contains(t, text, `"src":"Notes/simple.md"`)
}

func TestMoveNoteHandler_ConfirmMismatch(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.MoveNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"src", "Notes/simple.md",
		"dst", "Archive/simple.md",
		"confirm", "wrong.md",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestMoveNoteHandler_DstExists(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.MoveNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"src", "Notes/simple.md",
		"dst", "Notes/with-fm.md",
		"confirm", "Notes/simple.md",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, strings.ToLower(text), "exist")
}

func TestMoveNoteHandler_Traversal(t *testing.T) {
	deps := testDeps(t)
	handler := tools.MoveNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"src", "../escape.md",
		"dst", "Notes/escape.md",
		"confirm", "../escape.md",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
