package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// --- get_note_outline ---

func TestGetNoteOutlineHandler_Success(t *testing.T) {
	deps := writeDeps(t)
	require.NoError(t, deps.Vault.WriteNote(context.Background(), "Doc.md",
		"# Title\n\n## Section\nbody\n", vault.WriteModeOverwrite))

	handler := tools.GetNoteOutlineHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Doc.md"))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"level":1`)
	assert.Contains(t, text, `"text":"Title"`)
	assert.Contains(t, text, `"text":"Section"`)
}

func TestGetNoteOutlineHandler_NotFound(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.GetNoteOutlineHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Ghost.md"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestGetNoteOutlineHandler_RequiredParams(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.GetNoteOutlineHandler(deps)
	result, err := handler(context.Background(), makeRequest())
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- read_note_lines ---

func TestReadNoteLinesHandler_Success(t *testing.T) {
	deps := writeDeps(t)
	require.NoError(t, deps.Vault.WriteNote(context.Background(), "Doc.md",
		"one\ntwo\nthree\nfour\n", vault.WriteModeOverwrite))

	handler := tools.ReadNoteLinesHandler(deps)
	result, err := handler(context.Background(), makeRequestMixed(
		"path", "Doc.md",
		"startLine", 2,
		"lineCount", 2,
	))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"startLine":2`)
	assert.Contains(t, text, `"endLine":3`)
	assert.Contains(t, text, `two\nthree`)
}

func TestReadNoteLinesHandler_DefaultLineCount(t *testing.T) {
	deps := writeDeps(t)
	require.NoError(t, deps.Vault.WriteNote(context.Background(), "Doc.md", "one\ntwo\n", vault.WriteModeOverwrite))

	handler := tools.ReadNoteLinesHandler(deps)
	result, err := handler(context.Background(), makeRequestMixed("path", "Doc.md", "startLine", 1))
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, `"startLine":1`)
}

func TestReadNoteLinesHandler_RequiredParams(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.ReadNoteLinesHandler(deps)
	result, err := handler(context.Background(), makeRequest("path", "Doc.md"))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestReadNoteLinesHandler_NotFound(t *testing.T) {
	deps := writeDeps(t)
	handler := tools.ReadNoteLinesHandler(deps)
	result, err := handler(context.Background(), makeRequestMixed("path", "Ghost.md", "startLine", 1))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
