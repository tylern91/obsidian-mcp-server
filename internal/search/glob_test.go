package search

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlobToRegex asserts on globToRegex's exact generated regex string and
// the resulting match behaviour, including the bare "**" case not exercised
// by any acceptance test (see tests/acceptance/search_test.go [AC-24]).
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
