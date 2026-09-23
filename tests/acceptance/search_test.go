package acceptance_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// newSearchService copies the fixture vault into a fresh temp dir and returns
// a *search.Service backed by a raw *vault.Service.
func newSearchService(t *testing.T) *search.Service {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir("../../testdata/vault", dir); err != nil {
		t.Fatalf("copy fixture vault: %v", err)
	}
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return search.New(vault.New(dir, filter))
}

// newSearchServiceFromDir builds a search.Service pointing at an arbitrary directory.
func newSearchServiceFromDir(t *testing.T, dir string) *search.Service {
	t.Helper()
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return search.New(vault.New(dir, filter))
}

func searchResultsContain(results []search.BM25Result, suffix string) bool {
	for _, r := range results {
		if strings.HasSuffix(filepath.ToSlash(r.Path), suffix) {
			return true
		}
	}
	return false
}

func findSearchResult(results []search.BM25Result, suffix string) (search.BM25Result, bool) {
	for _, r := range results {
		if strings.HasSuffix(filepath.ToSlash(r.Path), suffix) {
			return r, true
		}
	}
	return search.BM25Result{}, false
}

func TestSearch_Acceptance(t *testing.T) {
	ctx := context.Background()

	t.Run("[AC-1] a note dense in query terms outranks one with sparser term density", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			SearchContent:     true,
			SearchFrontmatter: true,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)
		require.NotEmpty(t, results, "expected results for 'machine learning'")

		intro, ok1 := findSearchResult(results, "Search/ml-intro.md")
		basics, ok2 := findSearchResult(results, "Search/ml-basics.md")
		require.True(t, ok1, "ml-intro.md should appear in results")
		require.True(t, ok2, "ml-basics.md should appear in results")

		assert.Greater(t, intro.Score, basics.Score,
			"ml-intro (high density) should outrank ml-basics (medium density)")
	})

	t.Run("[AC-2] unrelated notes are excluded from top results for an unrelated query", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			Limit:             3,
			SearchContent:     true,
			SearchFrontmatter: true,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)

		for _, r := range results {
			assert.NotContains(t, r.Path, "cooking.md",
				"cooking.md should not appear in top-3 ML results")
			assert.NotContains(t, r.Path, "recipes.md",
				"recipes.md should not appear in top-3 ML results")
		}
	})

	t.Run("[AC-3] SearchFrontmatter=false excludes notes whose only match is in frontmatter", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			SearchContent:     true,
			SearchFrontmatter: false,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)

		for _, r := range results {
			assert.NotContains(t, r.Path, "fm-only.md",
				"fm-only.md should not appear when SearchFrontmatter=false")
		}
	})

	t.Run("[AC-4] SearchFrontmatter=true includes notes whose only match is in frontmatter", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			SearchContent:     true,
			SearchFrontmatter: true,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)

		assert.True(t, searchResultsContain(results, "Search/fm-only.md"),
			"fm-only.md should appear in results when SearchFrontmatter=true")
	})

	t.Run("[AC-5] terms inside code fences are not counted toward BM25 score", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			SearchContent:     true,
			SearchFrontmatter: false,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)

		_, hasFenced := findSearchResult(results, "Search/fenced-tags.md")
		assert.False(t, hasFenced,
			"fenced-tags.md should not appear because its ML terms are inside a code fence")
	})

	t.Run("[AC-6] PathScope restricts results to matching notes only", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			SearchContent:     true,
			SearchFrontmatter: true,
			PathScope:         "Search/ml-*",
		})
		require.NoError(t, err)
		require.NotEmpty(t, results)

		for _, r := range results {
			base := filepath.Base(r.Path)
			assert.True(t, strings.HasPrefix(base, "ml-"),
				"expected only ml-* files but got %q", r.Path)
		}
	})

	t.Run("[AC-7] CaseSensitive=true narrows results relative to case-insensitive search", func(t *testing.T) {
		svc := newSearchService(t)

		resultsSensitive, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "Machine",
			SearchContent:     true,
			SearchFrontmatter: false,
			CaseSensitive:     true,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)

		resultsCI, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine",
			SearchContent:     true,
			SearchFrontmatter: false,
			CaseSensitive:     false,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)

		assert.LessOrEqual(t, len(resultsSensitive), len(resultsCI),
			"case-sensitive 'Machine' should return fewer or equal results than case-insensitive 'machine'")
	})

	t.Run("[AC-8] Limit caps the number of BM25 results returned", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			Limit:             2,
			SearchContent:     true,
			SearchFrontmatter: true,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)

		assert.LessOrEqual(t, len(results), 2, "Limit=2 should return at most 2 results")
	})

	t.Run("[AC-9] an empty query returns no results", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "",
			SearchContent:     true,
			SearchFrontmatter: true,
		})
		require.NoError(t, err)
		assert.Empty(t, results, "empty query should return no results")
	})

	t.Run("[AC-10] consecutive term co-occurrence outranks the same terms scattered apart", func(t *testing.T) {
		tmpDir := t.TempDir()

		dense := `---
title: Dense Co-occurrence
---
# Dense

Machine learning is great. Machine learning rocks. Machine learning all day.
Machine learning never stops. Machine learning for the win.
`
		sparse := `---
title: Sparse Terms
---
# Sparse

I have a machine. It is a very good machine. People enjoy using the machine.
Learning is important. Keep learning every day. Learning never stops for anyone.
`
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "dense.md"), []byte(dense), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "sparse.md"), []byte(sparse), 0644))

		svc := newSearchServiceFromDir(t, tmpDir)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			SearchContent:     true,
			SearchFrontmatter: true,
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2, "expected at least 2 results")

		denseRes, ok1 := findSearchResult(results, "dense.md")
		sparseRes, ok2 := findSearchResult(results, "sparse.md")
		require.True(t, ok1, "dense.md should appear in results")
		require.True(t, ok2, "sparse.md should appear in results")

		assert.Greater(t, denseRes.Score, sparseRes.Score,
			"dense co-occurrence doc (consecutive bigrams) should outrank sparse term doc (no bigrams)")
	})

	t.Run("[AC-11] disabling both SearchContent and SearchFrontmatter returns nil without error", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine",
			SearchContent:     false,
			SearchFrontmatter: false,
		})
		require.NoError(t, err)
		assert.Nil(t, results)
	})

	t.Run("[AC-12] BM25 results are sorted by descending score", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			SearchContent:     true,
			SearchFrontmatter: true,
			PathScope:         "Search/*",
		})
		require.NoError(t, err)
		require.NotEmpty(t, results)

		for i := 1; i < len(results); i++ {
			assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score,
				"results should be sorted descending by score at index %d", i)
		}
	})

	t.Run("[AC-13] matching results carry snippet matches and a positive token count", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchBM25(ctx, search.BM25Options{
			Query:             "machine learning",
			SearchContent:     true,
			SearchFrontmatter: true,
			PathScope:         "Search/ml-intro.md",
		})
		require.NoError(t, err)
		require.NotEmpty(t, results)

		intro := results[0]
		assert.NotEmpty(t, intro.Matches, "ml-intro.md should have snippet matches")
		assert.Greater(t, intro.TokenCount, 0, "TokenCount should be positive")
	})

	t.Run("[AC-14] regex content scope finds the pattern and returns the matching line", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchRegex(ctx, search.RegexOptions{
			Pattern: "simple note with no frontmatter",
			Scope:   "content",
		})
		require.NoError(t, err)
		require.NotEmpty(t, results, "expected at least one result")

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

	t.Run("[AC-15] regex path scope matches by relative path with no content snippets", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchRegex(ctx, search.RegexOptions{
			Pattern: `Daily Notes/.*`,
			Scope:   "path",
		})
		require.NoError(t, err)
		require.NotEmpty(t, results, "expected at least one result")

		assert.Equal(t, "Daily Notes/2024-01-15.md", results[0].Path)
		assert.Empty(t, results[0].Matches, "path-scope match should have no line snippets")
	})

	t.Run("[AC-16] regex both scope returns both path-only and content hits", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchRegex(ctx, search.RegexOptions{
			Pattern: "simple",
			Scope:   "both",
			Limit:   50,
		})
		require.NoError(t, err)
		require.NotEmpty(t, results)

		var foundPath, foundContent bool
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

	t.Run("[AC-17] a glob pattern is honored when IsGlob is set", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchRegex(ctx, search.RegexOptions{
			Pattern: "Daily Notes/*.md",
			IsGlob:  true,
			Scope:   "path",
		})
		require.NoError(t, err)
		require.Len(t, results, 1, "expected exactly 1 result for glob Daily Notes/*.md")
		assert.Equal(t, "Daily Notes/2024-01-15.md", results[0].Path)
	})

	t.Run("[AC-18] Limit caps the total number of regex results", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchRegex(ctx, search.RegexOptions{
			Pattern: ".",
			Scope:   "content",
			Limit:   1,
		})
		require.NoError(t, err)
		assert.Len(t, results, 1, "Limit=1 should return exactly 1 result")
	})

	t.Run("[AC-19] an invalid regex pattern returns an error", func(t *testing.T) {
		svc := newSearchService(t)

		_, err := svc.SearchRegex(ctx, search.RegexOptions{
			Pattern: "[invalid",
			Scope:   "content",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pattern")
	})

	t.Run("[AC-20] a cancelled context aborts the search with an error", func(t *testing.T) {
		svc := newSearchService(t)

		cctx, cancel := context.WithCancel(ctx)
		cancel()

		_, err := svc.SearchRegex(cctx, search.RegexOptions{
			Pattern: "simple",
			Scope:   "content",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("[AC-21] MaxMatchesPerFile caps the number of matches collected per file", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchRegex(ctx, search.RegexOptions{
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

	t.Run("[AC-22] zero-value Limit and MaxMatchesPerFile fall back to their defaults", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchRegex(ctx, search.RegexOptions{
			Pattern:           `\S`,
			Scope:             "content",
			Limit:             0,
			MaxMatchesPerFile: 0,
		})
		require.NoError(t, err)
		assert.Greater(t, len(results), 0, "zero Limit should use default (20), not cap at 0")

		for _, r := range results {
			assert.LessOrEqual(t, len(r.Matches), 5,
				"file %q has %d matches, expected <= 5 (default MaxMatchesPerFile)", r.Path, len(r.Matches))
		}
	})

	t.Run("[AC-23] a both-scope hit that only matches on path carries no content matches", func(t *testing.T) {
		svc := newSearchService(t)

		results, err := svc.SearchRegex(ctx, search.RegexOptions{
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

	t.Run("[AC-24] glob patterns are anchored to full path segments, not substrings", func(t *testing.T) {
		cases := []struct {
			name    string
			glob    string
			wantAny []string // at least one result path must have this suffix
			wantNot []string // no result path may have this suffix
		}{
			{
				name:    "single-star matches within one directory level only",
				glob:    "Daily Notes/*.md",
				wantAny: []string{"Daily Notes/2024-01-15.md"},
			},
			{
				name:    "double-star matches across directory levels",
				glob:    "**/duplicate.md",
				wantAny: []string{"Notes/duplicate.md", "Notes/Deep/duplicate.md"},
			},
			{
				name:    "top-level star does not descend into subdirectories",
				glob:    "*.md",
				wantAny: nil,
				wantNot: []string{"Notes/simple.md", "Daily Notes/2024-01-15.md"},
			},
			{
				name:    "question mark matches exactly one character",
				glob:    "Notes/simple.md",
				wantAny: []string{"Notes/simple.md"},
				wantNot: []string{"Notes/simpler.md"},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc := newSearchService(t)
				results, err := svc.SearchRegex(ctx, search.RegexOptions{
					Pattern: tc.glob,
					IsGlob:  true,
					Scope:   "path",
					Limit:   50,
				})
				require.NoError(t, err)

				for _, want := range tc.wantAny {
					assert.True(t, searchRegexResultsContain(results, want),
						"expected glob %q to match %q", tc.glob, want)
				}
				for _, notWant := range tc.wantNot {
					assert.False(t, searchRegexResultsContain(results, notWant),
						"expected glob %q NOT to match %q", tc.glob, notWant)
				}
			})
		}
	})
}

func searchRegexResultsContain(results []search.RegexResult, path string) bool {
	for _, r := range results {
		if r.Path == path {
			return true
		}
	}
	return false
}
