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

func newHeadingVault(t *testing.T, files map[string]string) (*vault.Service, string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return vault.New(dir, nil), dir
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(b)
}

func TestPatchNote_HeadingAmbiguityAndFences(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		note    string
		op      vault.PatchOp
		want    string
		wantErr error
		errHas  []string
	}{
		{
			name:    "nested and top-level duplicates are ambiguous",
			note:    "# A\n## Notes\nnested\n# Notes\ntop\n",
			op:      vault.PatchOp{Heading: "Notes", Position: "replace_body", Content: "X"},
			wantErr: vault.ErrAmbiguousHeading,
			errHas:  []string{"line 2 (level 2)", "line 4 (level 1)"},
		},
		{
			name:    "same-level repeats are ambiguous",
			note:    "# Notes\none\n# Notes\ntwo\n",
			op:      vault.PatchOp{Heading: "Notes", Position: "replace_body", Content: "X"},
			wantErr: vault.ErrAmbiguousHeading,
			errHas:  []string{"line 1 (level 1)", "line 3 (level 1)"},
		},
		{
			name: "line selects the nested heading",
			note: "# A\n## Notes\nnested\n# Notes\ntop\n",
			op:   vault.PatchOp{Heading: "Notes", Position: "replace_body", Content: "X", Line: 2},
			want: "# A\n## Notes\nX\n# Notes\ntop\n",
		},
		{
			name: "line selects the top-level heading",
			note: "# A\n## Notes\nnested\n# Notes\ntop\n",
			op:   vault.PatchOp{Heading: "Notes", Position: "replace_body", Content: "X", Line: 4},
			want: "# A\n## Notes\nnested\n# Notes\nX",
		},
		{
			name:    "line pointing at a non-matching heading is not found",
			note:    "# A\n## Notes\nnested\n# Notes\ntop\n",
			op:      vault.PatchOp{Heading: "Notes", Position: "replace_body", Content: "X", Line: 1},
			wantErr: vault.ErrHeadingNotFound,
		},
		{
			name:    "line pointing at a non-heading line is not found",
			note:    "# A\n## Notes\nnested\n",
			op:      vault.PatchOp{Heading: "Notes", Position: "replace_body", Content: "X", Line: 3},
			wantErr: vault.ErrHeadingNotFound,
		},
		{
			name: "hash line inside a fence is not a heading",
			note: "# Real\n```\n# Real\n```\nbody\n",
			op:   vault.PatchOp{Heading: "Real", Position: "replace_body", Content: "X"},
			want: "# Real\nX",
		},
		{
			name:    "heading that only exists inside a fence is not found",
			note:    "# A\n~~~\n# Hidden\n~~~\n",
			op:      vault.PatchOp{Heading: "Hidden", Position: "before", Content: "X"},
			wantErr: vault.ErrHeadingNotFound,
		},
		{
			name: "hash line inside a fence does not end the section",
			note: "# A\nintro\n```\n# not a heading\n```\ntail\n# B\nb\n",
			op:   vault.PatchOp{Heading: "A", Position: "replace_body", Content: "X"},
			want: "# A\nX\n# B\nb\n",
		},
		{
			name: "CRLF fence is honoured",
			note: "# A\r\n```\r\n# A\r\n```\r\n",
			op:   vault.PatchOp{Heading: "A", Position: "after", Content: "X"},
			want: "# A\r\n```\r\n# A\r\n```\r\n\nX",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, dir := newHeadingVault(t, map[string]string{"n.md": tc.note})
			err := svc.PatchNote(ctx, "n.md", tc.op)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr), "got %v", err)
				for _, s := range tc.errHas {
					assert.Contains(t, err.Error(), s)
				}
				assert.Equal(t, tc.note, readFile(t, dir, "n.md"), "note must be unchanged on error")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, readFile(t, dir, "n.md"))
		})
	}
}

func TestGetNoteOutline_SkipsFencedHeadings(t *testing.T) {
	svc, _ := newHeadingVault(t, map[string]string{
		"n.md": "# A\n```bash\n# comment\n```\n## B\n~~~\n### C\n~~~\n",
	})
	got, err := svc.GetNoteOutline(context.Background(), "n.md")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, vault.HeadingInfo{Level: 1, Text: "A", Line: 1}, got[0])
	assert.Equal(t, vault.HeadingInfo{Level: 2, Text: "B", Line: 5}, got[1])
}

func TestRenameHeading(t *testing.T) {
	ctx := context.Background()

	t.Run("renames and keeps the level", func(t *testing.T) {
		svc, dir := newHeadingVault(t, map[string]string{"n.md": "# A\n### Old\nbody\n"})
		res, err := svc.RenameHeading(ctx, "n.md", "Old", "New", 0, true)
		require.NoError(t, err)
		assert.Equal(t, 3, res.Level)
		assert.Equal(t, 2, res.Line)
		assert.Equal(t, "# A\n### New\nbody\n", readFile(t, dir, "n.md"))
	})

	t.Run("ambiguous without line, selected with line", func(t *testing.T) {
		note := "# A\n## Notes\nnested\n# Notes\ntop\n"
		svc, dir := newHeadingVault(t, map[string]string{"n.md": note})
		_, err := svc.RenameHeading(ctx, "n.md", "Notes", "Renamed", 0, false)
		require.ErrorIs(t, err, vault.ErrAmbiguousHeading)
		assert.Contains(t, err.Error(), "line 2 (level 2)")
		assert.Equal(t, note, readFile(t, dir, "n.md"))

		_, err = svc.RenameHeading(ctx, "n.md", "Notes", "Renamed", 4, false)
		require.NoError(t, err)
		assert.Equal(t, "# A\n## Notes\nnested\n# Renamed\ntop\n", readFile(t, dir, "n.md"))
	})

	t.Run("fenced hash line is not renamed", func(t *testing.T) {
		svc, dir := newHeadingVault(t, map[string]string{"n.md": "```\n# Old\n```\n# Old\n"})
		_, err := svc.RenameHeading(ctx, "n.md", "Old", "New", 0, false)
		require.NoError(t, err)
		assert.Equal(t, "```\n# Old\n```\n# New\n", readFile(t, dir, "n.md"))
	})

	t.Run("heading only inside a fence is not found", func(t *testing.T) {
		svc, _ := newHeadingVault(t, map[string]string{"n.md": "```\n# Old\n```\n"})
		_, err := svc.RenameHeading(ctx, "n.md", "Old", "New", 0, false)
		require.ErrorIs(t, err, vault.ErrHeadingNotFound)
	})

	t.Run("empty or multi-line new heading is rejected", func(t *testing.T) {
		svc, dir := newHeadingVault(t, map[string]string{"n.md": "# Old\n"})
		for _, nh := range []string{"", "  ", "a\nb"} {
			_, err := svc.RenameHeading(ctx, "n.md", "Old", nh, 0, false)
			require.Error(t, err, "newHeading %q", nh)
		}
		assert.Equal(t, "# Old\n", readFile(t, dir, "n.md"))
	})

	t.Run("if_match conflict leaves note unchanged", func(t *testing.T) {
		svc, dir := newHeadingVault(t, map[string]string{"n.md": "# Old\n"})
		_, err := svc.RenameHeading(ctx, "n.md", "Old", "New", 0, false, vault.WithIfMatch("stale"))
		require.ErrorIs(t, err, vault.ErrRevisionConflict)
		assert.Equal(t, "# Old\n", readFile(t, dir, "n.md"))
	})

	t.Run("path traversal is rejected", func(t *testing.T) {
		svc, _ := newHeadingVault(t, map[string]string{"n.md": "# Old\n"})
		_, err := svc.RenameHeading(ctx, "../x.md", "Old", "New", 0, false)
		require.ErrorIs(t, err, vault.ErrPathTraversal)
	})

	t.Run("missing note", func(t *testing.T) {
		svc, _ := newHeadingVault(t, map[string]string{"n.md": "# Old\n"})
		_, err := svc.RenameHeading(ctx, "nope.md", "Old", "New", 0, false)
		require.ErrorIs(t, err, vault.ErrNotFound)
	})

	t.Run("updateLinks rewrites wiki and markdown anchors vault-wide", func(t *testing.T) {
		svc, dir := newHeadingVault(t, map[string]string{
			"Target.md": "# Top\n## Old Name\nbody\nsee [[#Old Name]] and [[Target#Old Name|me]]\n```\n[[#Old Name]]\n```\n",
			"a.md":      "[[Target#Old Name]] [[Target#Old Name|alias]] ![[Target#Old Name]] [[Target#Other]] [[Target#^blk]] [[Target]]\n",
			"b.md":      "[t](Target.md#old-name) [t](Target.md#Old%20Name) [t](Target.md#other) [x](Other.md#old-name)\n",
			"sub/c.md":  "[[Target#Top#Old Name]] [[sub/Target#Old Name]]\n",
		})
		res, err := svc.RenameHeading(ctx, "Target.md", "Old Name", "New Name", 0, true)
		require.NoError(t, err)
		assert.Empty(t, res.LinksSkipped)

		assert.Equal(t, "# Top\n## New Name\nbody\nsee [[#New Name]] and [[Target#New Name|me]]\n```\n[[#Old Name]]\n```\n", readFile(t, dir, "Target.md"))
		assert.Equal(t, "[[Target#New Name]] [[Target#New Name|alias]] ![[Target#New Name]] [[Target#Other]] [[Target#^blk]] [[Target]]\n", readFile(t, dir, "a.md"))
		assert.Equal(t, "[t](Target.md#new-name) [t](Target.md#New%20Name) [t](Target.md#other) [x](Other.md#old-name)\n", readFile(t, dir, "b.md"))
		assert.Equal(t, "[[Target#Top#New Name]] [[sub/Target#Old Name]]\n", readFile(t, dir, "sub/c.md"))
		assert.NotEmpty(t, res.Links)
	})

	t.Run("updateLinks false leaves links alone", func(t *testing.T) {
		svc, dir := newHeadingVault(t, map[string]string{
			"Target.md": "## Old\n",
			"a.md":      "[[Target#Old]]\n",
		})
		res, err := svc.RenameHeading(ctx, "Target.md", "Old", "New", 0, false)
		require.NoError(t, err)
		assert.Empty(t, res.Links)
		assert.Equal(t, "[[Target#Old]]\n", readFile(t, dir, "a.md"))
	})

	t.Run("same-note anchors follow updateLinks", func(t *testing.T) {
		cases := []struct {
			name        string
			updateLinks bool
			want        string
		}{
			{"false leaves [[#Old]] unchanged", false, "## New\nsee [[#Old]] and [[#Top#Old|alias]]\n"},
			{"true rewrites [[#Old]]", true, "## New\nsee [[#New]] and [[#Top#New|alias]]\n"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, dir := newHeadingVault(t, map[string]string{"n.md": "## Old\nsee [[#Old]] and [[#Top#Old|alias]]\n"})
				_, err := svc.RenameHeading(ctx, "n.md", "Old", "New", 0, tc.updateLinks)
				require.NoError(t, err)
				assert.Equal(t, tc.want, readFile(t, dir, "n.md"))
			})
		}
	})

	t.Run("ambiguous link target is reported and not rewritten", func(t *testing.T) {
		svc, dir := newHeadingVault(t, map[string]string{
			"x/Target.md": "## Old\n",
			"y/Target.md": "## Old\n",
			"a.md":        "[[Target#Old]] [[x/Target#Old]]\n",
		})
		res, err := svc.RenameHeading(ctx, "x/Target.md", "Old", "New", 0, true)
		require.NoError(t, err)
		assert.Equal(t, "[[Target#Old]] [[x/Target#New]]\n", readFile(t, dir, "a.md"))
		var ambiguous int
		for _, l := range res.Links {
			if l.Ambiguous {
				ambiguous++
			}
		}
		assert.Equal(t, 1, ambiguous)
	})

	t.Run("links are left alone when an earlier duplicate remains the target", func(t *testing.T) {
		svc, dir := newHeadingVault(t, map[string]string{
			"Target.md": "# Notes\n## Notes\n",
			"a.md":      "[[Target#Notes]]\n",
		})
		res, err := svc.RenameHeading(ctx, "Target.md", "Notes", "Renamed", 2, true)
		require.NoError(t, err)
		assert.NotEmpty(t, res.LinksSkipped)
		assert.Equal(t, "[[Target#Notes]]\n", readFile(t, dir, "a.md"))
	})
}
