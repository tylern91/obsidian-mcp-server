package vault_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// FuzzSanitizePath exercises the path-security invariants of sanitizePath via
// the public ResolvePath method. The test uses a temp directory as the vault
// root so that ResolvePath does not need the fixture vault to exist on disk.
//
// Invariants checked:
//  1. If a path string contains ".." segments the call must return an error
//     (path-traversal attempts are never allowed).
//  2. The function must never panic.
func FuzzSanitizePath(f *testing.F) {
	// Seed corpus: path-traversal attempts and normal paths.
	f.Add("../secret")
	f.Add("../../etc/passwd")
	f.Add("Notes/../../../tmp")
	f.Add("..")
	f.Add("../")
	f.Add("Notes/foo.md")
	f.Add("Notes/Sub/bar.md")
	f.Add("simple.md")
	f.Add("")
	f.Add("a/b/c/d.md")
	f.Add("Notes/\x00null.md")
	f.Add("/absolute/path")
	f.Add("normal/../still-normal/foo.md")

	// Use a temp dir so we don't need real vault files on disk.
	root := f.TempDir()
	svc := vault.New(root, nil)

	f.Fuzz(func(t *testing.T, p string) {
		// Must not panic.
		_, err := svc.ResolvePath(p)

		// If the cleaned form of p still escapes the root, an error is required.
		cleaned := filepath.Clean(p)
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			if err == nil {
				t.Fatalf("ResolvePath(%q): expected error for traversal path, got nil", p)
			}
		}
	})
}
