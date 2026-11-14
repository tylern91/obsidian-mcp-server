package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func TestReadMultipleNotesHandler_NativeArrayArgument(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadMultipleNotesHandler(deps)

	result, err := handler(context.Background(), makeRequestKV("paths", []any{"Notes/simple.md"}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal([]byte(extractText(t, result)), &resp))
	assert.Equal(t, 1, resp.Count)
}

func TestReadMultipleNotesHandler_WrongTypeArgument(t *testing.T) {
	deps := testDeps(t)
	handler := tools.ReadMultipleNotesHandler(deps)

	result, err := handler(context.Background(), makeRequestKV("paths", 42))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestUpdateFrontmatterHandler_NativeObjectArgument(t *testing.T) {
	deps := mutableDeps(t)
	handler := tools.UpdateFrontmatterHandler(deps)

	result, err := handler(context.Background(), makeRequestKV(
		"path", "Notes/simple.md",
		"updates", map[string]any{"k": "v"},
	))
	require.NoError(t, err)
	require.False(t, result.IsError, extractText(t, result))

	fm, _, err := deps.Vault.GetFrontmatter(context.Background(), "Notes/simple.md")
	require.NoError(t, err)
	assert.Equal(t, "v", fm["k"])
}

// moveStub lets a test script MoveNote's return while every other
// VaultService method is left unimplemented (nil embedded interface).
type moveStub struct {
	tools.VaultService
	result *vault.MoveResult
	err    error
}

func (m moveStub) MoveNote(context.Context, string, string, string, bool, bool, ...vault.WriteOpt) (*vault.MoveResult, error) {
	return m.result, m.err
}

func TestMoveNoteHandler_PartialLinkRewriteFailure(t *testing.T) {
	deps := tools.Deps{Vault: moveStub{
		result: &vault.MoveResult{Src: "a.md", Dst: "b.md", Moved: true},
		err:    errors.New("walk failed"),
	}}
	handler := tools.MoveNoteHandler(deps)

	result, err := handler(context.Background(), makeRequest("src", "a.md", "dst", "b.md", "confirm", "a.md"))
	require.NoError(t, err)
	require.False(t, result.IsError, "file already moved: must not report failure")

	var resp struct {
		Moved            bool   `json:"moved"`
		LinksComplete    *bool  `json:"linksComplete"`
		LinkRewriteError string `json:"linkRewriteError"`
	}
	require.NoError(t, json.Unmarshal([]byte(extractText(t, result)), &resp))
	assert.True(t, resp.Moved)
	require.NotNil(t, resp.LinksComplete)
	assert.False(t, *resp.LinksComplete)
	assert.Contains(t, resp.LinkRewriteError, "walk failed")
}
