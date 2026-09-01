package vault_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func writeNote(t *testing.T, svc *vault.Service, relPath, content string) {
	t.Helper()
	abs := filepath.Join(svc.Root(), relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0644))
}

// --- GetNoteOutline ---

func TestService_GetNoteOutline(t *testing.T) {
	ctx := context.Background()

	t.Run("returns headings with level, text, and 1-indexed line", func(t *testing.T) {
		svc := newTempVault(t)
		writeNote(t, svc, "Outline.md", "# Title\n\nsome text\n\n## Section One\nbody\n\n### Sub\nmore\n\n## Section Two\n")

		headings, err := svc.GetNoteOutline(ctx, "Outline.md")
		require.NoError(t, err)
		require.Len(t, headings, 4)

		assert.Equal(t, vault.HeadingInfo{Level: 1, Text: "Title", Line: 1}, headings[0])
		assert.Equal(t, vault.HeadingInfo{Level: 2, Text: "Section One", Line: 5}, headings[1])
		assert.Equal(t, vault.HeadingInfo{Level: 3, Text: "Sub", Line: 8}, headings[2])
		assert.Equal(t, vault.HeadingInfo{Level: 2, Text: "Section Two", Line: 11}, headings[3])
	})

	t.Run("malformed heading without a space after # is not counted", func(t *testing.T) {
		svc := newTempVault(t)
		writeNote(t, svc, "Malformed.md", "#NoSpace\n# Real Heading\n")

		headings, err := svc.GetNoteOutline(ctx, "Malformed.md")
		require.NoError(t, err)
		require.Len(t, headings, 1)
		assert.Equal(t, "Real Heading", headings[0].Text)
	})

	t.Run("no headings returns an empty, non-nil slice", func(t *testing.T) {
		svc := newTempVault(t)
		writeNote(t, svc, "Plain.md", "just some text\nno headings here\n")

		headings, err := svc.GetNoteOutline(ctx, "Plain.md")
		require.NoError(t, err)
		assert.NotNil(t, headings)
		assert.Empty(t, headings)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newTempVault(t)
		_, err := svc.GetNoteOutline(ctx, "Ghost.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})
}

// --- ReadNoteLines ---

func TestService_ReadNoteLines(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the requested range, inclusive", func(t *testing.T) {
		svc := newTempVault(t)
		writeNote(t, svc, "Lines.md", "one\ntwo\nthree\nfour\nfive\n")

		result, err := svc.ReadNoteLines(ctx, "Lines.md", 2, 2)
		require.NoError(t, err)
		assert.Equal(t, 2, result.StartLine)
		assert.Equal(t, 3, result.EndLine)
		assert.Equal(t, 6, result.TotalLines) // trailing "" element after the final \n
		assert.Equal(t, "two\nthree", result.Content)
	})

	t.Run("lineCount beyond the end clamps to the last line", func(t *testing.T) {
		svc := newTempVault(t)
		writeNote(t, svc, "Lines.md", "one\ntwo\nthree\n")

		result, err := svc.ReadNoteLines(ctx, "Lines.md", 2, 100)
		require.NoError(t, err)
		assert.Equal(t, 2, result.StartLine)
		assert.Equal(t, 4, result.EndLine) // includes the trailing "" element after the final \n
		assert.Equal(t, "two\nthree\n", result.Content)
	})

	t.Run("startLine beyond the end returns an empty range, not an error", func(t *testing.T) {
		svc := newTempVault(t)
		writeNote(t, svc, "Lines.md", "one\ntwo\n")

		result, err := svc.ReadNoteLines(ctx, "Lines.md", 50, 10)
		require.NoError(t, err)
		assert.Equal(t, "", result.Content)
		assert.Less(t, result.EndLine, result.StartLine)
		assert.Equal(t, 3, result.TotalLines)
	})

	t.Run("startLine and lineCount below 1 are clamped rather than erroring", func(t *testing.T) {
		svc := newTempVault(t)
		writeNote(t, svc, "Lines.md", "one\ntwo\n")

		result, err := svc.ReadNoteLines(ctx, "Lines.md", 0, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, result.StartLine)
		assert.Equal(t, "one", result.Content)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newTempVault(t)
		_, err := svc.ReadNoteLines(ctx, "Ghost.md", 1, 10)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})
}
