// Package acceptance_test holds [AC-N]-named acceptance tests, one per acceptance criterion.
// See AGENTS.md § Critical conventions for the ATDD convention this package follows.
package acceptance_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// newVaultDeps copies the committed testdata/vault fixture into a fresh t.TempDir()
// and returns a tools.Deps wired to it. Never mutate testdata/vault directly.
func newVaultDeps(t *testing.T) tools.Deps {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir("../../testdata/vault", dir); err != nil {
		t.Fatalf("copy fixture vault: %v", err)
	}
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return tools.Deps{
		Vault:       vault.New(dir, filter),
		PrettyPrint: false,
	}
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
