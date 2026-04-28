package search

import (
	"regexp"
	"strings"
	"testing"
)

// FuzzGlobToRegex exercises the glob-to-regex converter.
//
// Invariants checked:
//  1. The function must never panic.
//  2. The returned string must always compile as a valid RE2 regex.
//  3. The returned pattern must be anchored (starts with "^", ends with "$").
func FuzzGlobToRegex(f *testing.F) {
	// Seed corpus: normal globs, edge cases, and potentially invalid inputs.
	f.Add("*.md")
	f.Add("**/*.md")
	f.Add("Notes/*")
	f.Add("**")
	f.Add("[invalid")
	f.Add("")
	f.Add("?")
	f.Add("*")
	f.Add("a/b/c/*.txt")
	f.Add("**/**/*.md")
	f.Add("foo.md")
	f.Add("[")
	f.Add("]]")
	f.Add("Notes/../secret")
	f.Add("\x00")
	f.Add("(unbalanced")

	f.Fuzz(func(t *testing.T, glob string) {
		// Must not panic.
		result := globToRegex(glob)

		// Result must be a valid RE2 regex.
		if _, err := regexp.Compile(result); err != nil {
			t.Fatalf("globToRegex(%q) produced invalid regex %q: %v", glob, result, err)
		}

		// Result must be anchored.
		if !strings.HasPrefix(result, "^") {
			t.Fatalf("globToRegex(%q) = %q: missing leading '^' anchor", glob, result)
		}
		if !strings.HasSuffix(result, "$") {
			t.Fatalf("globToRegex(%q) = %q: missing trailing '$' anchor", glob, result)
		}
	})
}
