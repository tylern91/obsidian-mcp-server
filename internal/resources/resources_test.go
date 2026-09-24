package resources_test

import (
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/resources"
)

// TestPathFromURI covers pathFromURI edge cases (empty path suffix,
// non-matching prefix) that tests/acceptance/resources_test.go's ACs don't
// reach directly since every AC URI carries a matching prefix and a
// non-empty path.
func TestPathFromURI(t *testing.T) {
	cases := []struct {
		uri, prefix, want string
	}{
		{"obsidian://note/Notes/foo.md", "obsidian://note/", "Notes/foo.md"},
		{"obsidian://backlinks/Notes/bar.md", "obsidian://backlinks/", "Notes/bar.md"},
		{"obsidian://note/", "obsidian://note/", ""},
		{"obsidian://other/x", "obsidian://note/", ""},
	}
	for _, tc := range cases {
		got := resources.PathFromURI(tc.uri, tc.prefix)
		if got != tc.want {
			t.Errorf("PathFromURI(%q, %q) = %q, want %q", tc.uri, tc.prefix, got, tc.want)
		}
	}
}
