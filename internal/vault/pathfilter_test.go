package vault_test

import (
	"errors"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// ----------------------------------------------------------------------------
// IsIgnored
// ----------------------------------------------------------------------------

func TestPathFilter_IsIgnored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ignorePatterns []string
		path           string
		want           bool
	}{
		{
			name:           "matching component in the middle of path",
			ignorePatterns: []string{".git"},
			path:           "Daily Notes/.git/config",
			want:           true,
		},
		{
			name:           "matching root component",
			ignorePatterns: []string{".obsidian"},
			path:           ".obsidian/workspace.json",
			want:           true,
		},
		{
			name:           "no matching component",
			ignorePatterns: []string{".git", ".obsidian"},
			path:           "Notes/my-note.md",
			want:           false,
		},
		{
			name:           "partial component name does not match",
			ignorePatterns: []string{".git"},
			path:           "Notes/.git-backup/file",
			want:           false,
		},
		{
			name:           "empty ignore patterns always returns false",
			ignorePatterns: []string{},
			path:           ".git/config",
			want:           false,
		},
		{
			name:           "nil ignore patterns always returns false",
			ignorePatterns: nil,
			path:           ".git/config",
			want:           false,
		},
		{
			name:           "exact filename match at leaf",
			ignorePatterns: []string{"node_modules"},
			path:           "project/node_modules",
			want:           true,
		},
		{
			name:           "multiple patterns, second matches",
			ignorePatterns: []string{".git", "node_modules"},
			path:           "project/node_modules/dep/index.js",
			want:           true,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := vault.NewPathFilter(tc.ignorePatterns, nil)
			got := f.IsIgnored(tc.path)
			if got != tc.want {
				t.Errorf("IsIgnored(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// IsAllowedExtension
// ----------------------------------------------------------------------------

func TestPathFilter_IsAllowedExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		allowedExts []string
		path        string
		want        bool
	}{
		{
			name:        ".md file with .md in allowed list",
			allowedExts: []string{".md", ".txt"},
			path:        "Notes/my-note.md",
			want:        true,
		},
		{
			name:        ".MD uppercase file with .md in allowed list (case-insensitive)",
			allowedExts: []string{".md"},
			path:        "Notes/my-note.MD",
			want:        true,
		},
		{
			name:        ".pdf not in allowed list",
			allowedExts: []string{".md", ".txt"},
			path:        "Attachments/document.pdf",
			want:        false,
		},
		{
			name:        "file with no extension returns false when list is non-empty",
			allowedExts: []string{".md"},
			path:        "Notes/README",
			want:        false,
		},
		{
			name:        "empty allowed list allows any extension",
			allowedExts: []string{},
			path:        "Notes/my-note.pdf",
			want:        true,
		},
		{
			name:        "nil allowed list allows any extension",
			allowedExts: nil,
			path:        "Notes/my-note.pdf",
			want:        true,
		},
		{
			name:        "empty allowed list with extensionless file",
			allowedExts: []string{},
			path:        "Notes/README",
			want:        true,
		},
		{
			name:        ".canvas extension allowed",
			allowedExts: []string{".md", ".canvas"},
			path:        "Maps/graph.canvas",
			want:        true,
		},
		{
			name:        "mixed-case allowed ext matches lower-case file ext",
			allowedExts: []string{".MD"},
			path:        "Notes/note.md",
			want:        true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := vault.NewPathFilter(nil, tc.allowedExts)
			got := f.IsAllowedExtension(tc.path)
			if got != tc.want {
				t.Errorf("IsAllowedExtension(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// PathError
// ----------------------------------------------------------------------------

func TestPathError_Error(t *testing.T) {
	t.Parallel()

	err := &vault.PathError{
		Op:   "resolve",
		Path: "path/to/note.md",
		Err:  vault.ErrPathTraversal,
	}

	want := "resolve path/to/note.md: path traversal attempt"
	if got := err.Error(); got != want {
		t.Errorf("PathError.Error() = %q, want %q", got, want)
	}
}

func TestPathError_IsUnwrap(t *testing.T) {
	t.Parallel()

	wrapped := &vault.PathError{
		Op:   "write",
		Path: "secret/../etc/passwd",
		Err:  vault.ErrPathTraversal,
	}

	if !errors.Is(wrapped, vault.ErrPathTraversal) {
		t.Error("errors.Is(wrapped, ErrPathTraversal) = false, want true")
	}
}

func TestPathError_AsUnwrap(t *testing.T) {
	t.Parallel()

	original := &vault.PathError{
		Op:   "read",
		Path: "missing-note.md",
		Err:  vault.ErrNotFound,
	}

	// Wrap in a plain fmt.Errorf-style wrapping to simulate call-site wrapping.
	var target *vault.PathError
	if !errors.As(original, &target) {
		t.Fatal("errors.As did not find *PathError in the chain")
	}
	if target.Op != "read" {
		t.Errorf("target.Op = %q, want %q", target.Op, "read")
	}
	if target.Path != "missing-note.md" {
		t.Errorf("target.Path = %q, want %q", target.Path, "missing-note.md")
	}
}

func TestPathError_WrapsSentinels(t *testing.T) {
	t.Parallel()

	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrPathTraversal", vault.ErrPathTraversal},
		{"ErrSymlinkEscape", vault.ErrSymlinkEscape},
		{"ErrNotFound", vault.ErrNotFound},
		{"ErrConfirmMismatch", vault.ErrConfirmMismatch},
		{"ErrAlreadyExists", vault.ErrAlreadyExists},
		{"ErrAmbiguousPath", vault.ErrAmbiguousPath},
		{"ErrPathRestricted", vault.ErrPathRestricted},
	}

	for _, s := range sentinels {
		s := s
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			wrapped := &vault.PathError{Op: "op", Path: "p", Err: s.err}
			if !errors.Is(wrapped, s.err) {
				t.Errorf("errors.Is(*PathError{Err: %v}, %v) = false, want true", s.err, s.err)
			}
		})
	}
}
