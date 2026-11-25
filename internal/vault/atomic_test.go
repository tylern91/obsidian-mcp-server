package vault_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// noTempFiles fails the test if any atomic temp file remains under dir.
func noTempFiles(t *testing.T, dir string) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".tmp-") {
			t.Errorf("leftover temp file: %s", p)
		}
		return nil
	})
}

func TestAtomicWrite_FailureBeforeRenameKeepsOriginal(t *testing.T) {
	root := t.TempDir()
	note := filepath.Join(root, "a.md")
	if err := os.WriteFile(note, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := newService(root)

	boom := errors.New("simulated crash")
	restore := vault.SetBeforeRenameHook(func() error {
		// The temp file must be fully written but not yet visible as the note.
		got, _ := os.ReadFile(note)
		if string(got) != "original" {
			t.Errorf("target changed before rename: %q", got)
		}
		return boom
	})
	defer restore()

	err := svc.WriteNote(context.Background(), "a.md", "replacement", vault.WriteModeOverwrite)
	if !errors.Is(err, boom) {
		t.Fatalf("want simulated failure, got %v", err)
	}
	got, _ := os.ReadFile(note)
	if string(got) != "original" {
		t.Errorf("content = %q, want original", got)
	}
	noTempFiles(t, root)
}

func TestAtomicWrite_SymlinkedNoteKeepsLink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real"), 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "real", "t.md")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	svc := newService(root)

	if err := svc.WriteNote(context.Background(), "link.md", "new", vault.WriteModeOverwrite); err != nil {
		t.Fatalf("WriteNote: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was replaced: %v %v", info, err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Errorf("target = %q, want new", got)
	}
	noTempFiles(t, root)
}

func TestAtomicWrite_PreservesMode(t *testing.T) {
	root := t.TempDir()
	note := filepath.Join(root, "m.md")
	if err := os.WriteFile(note, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(note, 0600); err != nil {
		t.Fatal(err)
	}
	svc := newService(root)

	for _, mode := range []vault.WriteMode{vault.WriteModeOverwrite, vault.WriteModeAppend} {
		if err := svc.WriteNote(context.Background(), "m.md", "y", mode); err != nil {
			t.Fatalf("WriteNote(%s): %v", mode, err)
		}
		info, _ := os.Stat(note)
		if info.Mode().Perm() != 0600 {
			t.Errorf("mode after %s = %v, want 0600", mode, info.Mode().Perm())
		}
	}
	noTempFiles(t, root)
}

func TestAtomicWrite_ReadersNeverSeePartialContent(t *testing.T) {
	root := t.TempDir()
	a := strings.Repeat("a", 1<<18)
	b := strings.Repeat("b", 1<<18)
	if err := os.WriteFile(filepath.Join(root, "c.md"), []byte(a), 0644); err != nil {
		t.Fatal(err)
	}
	svc := newService(root)

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			data, err := os.ReadFile(filepath.Join(root, "c.md"))
			if err != nil {
				t.Errorf("read: %v", err)
				return
			}
			if s := string(data); s != a && s != b {
				t.Errorf("partial content of %d bytes", len(data))
				return
			}
		}
	}()
	for i := 0; i < 200; i++ {
		c := a
		if i%2 == 0 {
			c = b
		}
		if err := svc.WriteNote(context.Background(), "c.md", c, vault.WriteModeOverwrite); err != nil {
			t.Fatal(err)
		}
	}
	close(done)
	wg.Wait()
	noTempFiles(t, root)
}

func TestAtomicWrite_TempFileNeverListed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// A filter-less service has no extension allowlist to hide a temp name.
	svc := vault.New(root, nil)

	restore := vault.SetBeforeRenameHook(func() error {
		var walked []string
		_ = svc.WalkNotes(context.Background(), func(rel, _ string) error {
			walked = append(walked, rel)
			return nil
		})
		for _, w := range walked {
			if strings.Contains(w, ".tmp-") {
				t.Errorf("walk listed temp file %q", w)
			}
		}
		entries, err := svc.ListDirectory(context.Background(), "")
		if err != nil {
			t.Errorf("ListDirectory: %v", err)
		}
		for _, e := range entries {
			if strings.Contains(e.Name, ".tmp-") {
				t.Errorf("listing showed temp file %q", e.Name)
			}
		}
		return nil
	})
	defer restore()

	if err := svc.WriteNote(context.Background(), "a.md", "y", vault.WriteModeOverwrite); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicWrite_ReadOnlyNoteIsNotReplaced(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only file does not block writes for root")
	}
	root := t.TempDir()
	note := filepath.Join(root, "ro.md")
	if err := os.WriteFile(note, []byte("keep"), 0o444); err != nil {
		t.Fatal(err)
	}
	svc := newService(root)

	err := svc.WriteNote(context.Background(), "ro.md", "changed", vault.WriteModeOverwrite)
	if err == nil {
		t.Fatal("WriteNote replaced a read-only note, want a permission error")
	}
	got, _ := os.ReadFile(note)
	if string(got) != "keep" {
		t.Errorf("read-only note content = %q, want it unchanged", got)
	}
	noTempFiles(t, root)
}
