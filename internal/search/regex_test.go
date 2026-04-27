package search

import (
	"context"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// testVaultPath resolves the testdata/vault path relative to this source file.
func testVaultPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// file is .../internal/search/regex_test.go
	// testdata/vault is two directories up: .../testdata/vault
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "vault")
}

// newSearchService creates a search.Service backed by the fixture vault.
func newSearchService(t *testing.T) *Service {
	t.Helper()
	root := testVaultPath(t)
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	v := vault.New(root, filter)
	return New(v)
}

func TestSearchRegex(t *testing.T) {
	t.Parallel()

	svc := newSearchService(t)

	t.Run("content scope finds pattern in known file", func(t *testing.T) {
		t.Parallel()

		results, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern: "simple note with no frontmatter",
			Scope:   "content",
		})
		require.NoError(t, err)
		require.NotEmpty(t, results, "expected at least one result")

		// Notes/simple.md should appear in the results.
		var found bool
		for _, r := range results {
			if r.Path == "Notes/simple.md" {
				found = true
				require.NotEmpty(t, r.Matches, "expected line matches in Notes/simple.md")
				assert.Equal(t, "This is a simple note with no frontmatter.", r.Matches[0].Snippet)
				break
			}
		}
		assert.True(t, found, "Notes/simple.md not found in results")
	})

	t.Run("path scope finds file by path pattern", func(t *testing.T) {
		t.Parallel()

		results, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern: `Daily Notes/.*`,
			Scope:   "path",
		})
		require.NoError(t, err)
		require.NotEmpty(t, results, "expected at least one result")

		assert.Equal(t, "Daily Notes/2024-01-15.md", results[0].Path)
		// Path matches must have no line snippets.
		assert.Empty(t, results[0].Matches, "path-scope match should have no line snippets")
	})

	t.Run("both scope returns path and content results", func(t *testing.T) {
		t.Parallel()

		// "simple" appears in the content of multiple files and in the path of
		// Notes/simple.md.
		results, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern: "simple",
			Scope:   "both",
			Limit:   50,
		})
		require.NoError(t, err)
		require.NotEmpty(t, results)

		var foundPath bool
		var foundContent bool
		for _, r := range results {
			if r.Path == "Notes/simple.md" {
				foundPath = true
			}
			if len(r.Matches) > 0 {
				foundContent = true
			}
		}
		assert.True(t, foundPath, "Notes/simple.md should appear via path match in both scope")
		assert.True(t, foundContent, "some file should have content matches in both scope")
	})

	t.Run("glob pattern matches daily note", func(t *testing.T) {
		t.Parallel()

		results, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern: "Daily Notes/*.md",
			IsGlob:  true,
			Scope:   "path",
		})
		require.NoError(t, err)
		require.Len(t, results, 1, "expected exactly 1 result for glob Daily Notes/*.md")
		assert.Equal(t, "Daily Notes/2024-01-15.md", results[0].Path)
	})

	t.Run("limit caps total results", func(t *testing.T) {
		t.Parallel()

		results, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern: ".",    // matches every non-empty line; many files will have content hits
			Scope:   "content",
			Limit:   1,
		})
		require.NoError(t, err)
		assert.Len(t, results, 1, "Limit=1 should return exactly 1 result")
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		t.Parallel()

		_, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern: "[invalid",
			Scope:   "content",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pattern")
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately before the walk starts

		_, err := svc.SearchRegex(ctx, RegexOptions{
			Pattern: "simple",
			Scope:   "content",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("MaxMatchesPerFile caps per-file matches", func(t *testing.T) {
		t.Parallel()

		// Match any line with non-whitespace content; most files have many such lines.
		results, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern:           `\S`,
			Scope:             "content",
			Limit:             50,
			MaxMatchesPerFile: 2,
		})
		require.NoError(t, err)

		for _, r := range results {
			assert.LessOrEqual(t, len(r.Matches), 2,
				"file %q has %d matches, expected <= 2", r.Path, len(r.Matches))
		}
	})

	t.Run("zero-value Limit and MaxMatchesPerFile use defaults", func(t *testing.T) {
		t.Parallel()

		// Limit=0 and MaxMatchesPerFile=0 should apply defaults (20 and 5),
		// not cap results at zero.
		results, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern:           `\S`, // matches any non-whitespace line — many files qualify
			Scope:             "content",
			Limit:             0,
			MaxMatchesPerFile: 0,
		})
		require.NoError(t, err)
		assert.Greater(t, len(results), 0, "zero Limit should use default (20), not cap at 0")

		for _, r := range results {
			assert.LessOrEqual(t, len(r.Matches), defaultMaxMatchesPerFile,
				"file %q has %d matches, expected <= %d (default MaxMatchesPerFile)",
				r.Path, len(r.Matches), defaultMaxMatchesPerFile)
		}
	})

	t.Run("both scope path-only hit has empty matches", func(t *testing.T) {
		t.Parallel()

		// "Daily" (capital D) appears in the path "Daily Notes/2024-01-15.md"
		// but not in the file content (content has lowercase "daily").
		// In scope=both the file should appear with an empty Matches slice.
		results, err := svc.SearchRegex(context.Background(), RegexOptions{
			Pattern: `Daily`,
			Scope:   "both",
			Limit:   50,
		})
		require.NoError(t, err)

		var found bool
		for _, r := range results {
			if r.Path == "Daily Notes/2024-01-15.md" {
				found = true
				assert.Empty(t, r.Matches,
					"path-only hit in both scope should have no content matches")
				break
			}
		}
		assert.True(t, found, "Daily Notes/2024-01-15.md should appear via path hit in both scope")
	})
}

// TestGlobToRegex verifies the glob-to-regex conversion rules.
func TestGlobToRegex(t *testing.T) {
	t.Parallel()

	cases := []struct {
		glob    string
		pattern string   // expected regex string
		matches []string // paths that should match
		noMatch []string // paths that must NOT match
	}{
		{
			glob:    "Daily Notes/*.md",
			pattern: `^Daily Notes/[^/]*\.md$`,
			matches: []string{"Daily Notes/2024-01-15.md", "Daily Notes/foo.md"},
			noMatch: []string{"Daily Notes/sub/nested.md", "Notes/simple.md"},
		},
		{
			glob:    "**/*.md",
			pattern: `^(.*/)?[^/]*\.md$`,
			matches: []string{"simple.md", "Notes/simple.md", "a/b/c/foo.md"},
			noMatch: []string{"Notes/simple.txt"},
		},
		{
			glob:    "*.md",
			pattern: `^[^/]*\.md$`,
			matches: []string{"simple.md", "README.md"},
			noMatch: []string{"Notes/simple.md"},
		},
		{
			glob:    "Notes/?.md",
			pattern: `^Notes/[^/]\.md$`,
			matches: []string{"Notes/a.md"},
			noMatch: []string{"Notes/ab.md", "Notes/simple.md"},
		},
		{
			glob:    "**",
			pattern: `^.*$`,
			matches: []string{"anything", "Notes/simple.md", "a/b/c/d.md"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.glob, func(t *testing.T) {
			t.Parallel()

			got := globToRegex(tc.glob)
			assert.Equal(t, tc.pattern, got)

			re, err := regexp.Compile(got)
			require.NoError(t, err)

			for _, m := range tc.matches {
				assert.True(t, re.MatchString(m),
					"expected %q to match glob %q (regex %q)", m, tc.glob, got)
			}
			for _, nm := range tc.noMatch {
				assert.False(t, re.MatchString(nm),
					"expected %q NOT to match glob %q (regex %q)", nm, tc.glob, got)
			}
		})
	}
}
