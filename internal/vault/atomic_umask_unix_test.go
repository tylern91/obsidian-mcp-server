//go:build unix

package vault_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// Not parallel: the umask is process-wide, and parallel tests only resume once
// every serial test has finished.
func TestAtomicWrite_NewFileHonorsUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)

	root := t.TempDir()
	svc := newService(root)
	if err := svc.WriteNote(context.Background(), "new.md", "x", vault.WriteModeOverwrite); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "new.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("new note mode = %v under umask 077, want 0600", got)
	}
}
