package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
)

// readEtag reads a note via read_note and returns its etag field.
func readEtag(t *testing.T, deps tools.Deps, path string) string {
	t.Helper()
	handler := tools.ReadNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", path))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)

	var resp struct {
		Etag string `json:"etag"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	require.NotEmpty(t, resp.Etag)
	return resp.Etag
}

func TestReadNoteHandler_EmitsEtag(t *testing.T) {
	deps := testDeps(t)
	readEtag(t, deps, "Notes/simple.md")
}

func TestWriteNoteHandler_IfMatchConflict(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.WriteNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"content", "new content",
		"if_match", "stale-etag",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, extractText(t, result), "REVISION_CONFLICT")
}

func TestWriteNoteHandler_IfMatchSucceeds(t *testing.T) {
	deps := mutableDeps(t)
	etag := readEtag(t, deps, "Notes/simple.md")

	handler := tools.WriteNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"content", "new content",
		"if_match", etag,
	))
	require.NoError(t, err)
	require.False(t, result.IsError)
}

func TestPatchNoteHandler_IfMatchConflict(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.PatchNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"heading", "Simple Note",
		"position", "after",
		"content", "x",
		"if_match", "stale-etag",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, extractText(t, result), "REVISION_CONFLICT")
}

func TestUpdateFrontmatterHandler_IfMatchConflict(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.UpdateFrontmatterHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/with-fm.md",
		"updates", `{"title":"x"}`,
		"if_match", "stale-etag",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, extractText(t, result), "REVISION_CONFLICT")
}

func TestManageTagsHandler_IfMatchConflict(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.ManageTagsHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"action", "add",
		"tag", "newtag",
		"if_match", "stale-etag",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, extractText(t, result), "REVISION_CONFLICT")
}

func TestDeleteNoteHandler_IfMatchConflict(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.DeleteNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"path", "Notes/simple.md",
		"confirm", "Notes/simple.md",
		"if_match", "stale-etag",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, extractText(t, result), "REVISION_CONFLICT")
}

func TestMoveNoteHandler_IfMatchConflict(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.MoveNoteHandler(deps)
	result, err := handler(context.Background(), makeRequest(
		"src", "Notes/simple.md",
		"dst", "Archive/simple.md",
		"confirm", "Notes/simple.md",
		"if_match", "stale-etag",
	))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, extractText(t, result), "REVISION_CONFLICT")
}

func TestGetNotesInfoHandler_EmitsEtag(t *testing.T) {
	deps := testDeps(t)
	paths, err := json.Marshal([]string{"Notes/simple.md"})
	require.NoError(t, err)

	handler := tools.GetNotesInfoHandler(deps)
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, extractText(t, result), `"etag"`)
}

func TestReadMultipleNotesHandler_EmitsEtag(t *testing.T) {
	deps := testDeps(t)
	paths, err := json.Marshal([]string{"Notes/simple.md"})
	require.NoError(t, err)

	handler := tools.ReadMultipleNotesHandler(deps)
	result, err := handler(context.Background(), makeRequest("paths", string(paths)))
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, extractText(t, result), `"etag"`)
}
