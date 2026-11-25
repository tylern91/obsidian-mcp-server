package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
)

func TestHeadingLineArg_NegativeRejected(t *testing.T) {
	deps := mutableDeps(t)
	patch, err := tools.PatchNoteHandler(deps)(context.Background(), makeRequestMixed(
		"path", "Notes/simple.md", "heading", "H", "position", "after", "content", "x", "line", -1))
	require.NoError(t, err)
	assert.True(t, patch.IsError)
	assert.Contains(t, extractText(t, patch), "line")

	rename, err := tools.RenameHeadingHandler(deps)(context.Background(), makeRequestMixed(
		"path", "Notes/simple.md", "heading", "H", "newHeading", "X", "line", -3))
	require.NoError(t, err)
	assert.True(t, rename.IsError)
}

func TestRenameHeadingHandler_RequiredParams(t *testing.T) {
	result, err := tools.RenameHeadingHandler(testDeps(t))(context.Background(), makeRequest("path", "Notes/simple.md"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestRenameHeadingHandler_Traversal(t *testing.T) {
	result, err := tools.RenameHeadingHandler(testDeps(t))(context.Background(), makeRequest(
		"path", "../escape.md", "heading", "H", "newHeading", "X"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
