package vault_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

var (
	testVaultRoot string

	defaultIgnore = []string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"}
	defaultExts   = []string{".md", ".markdown", ".txt", ".canvas"}
)

func TestMain(m *testing.M) {
	// Resolve the absolute path to the fixture vault.
	abs, err := filepath.Abs("../../testdata/vault")
	if err != nil {
		panic("failed to resolve testdata/vault: " + err.Error())
	}
	testVaultRoot = abs

	// Create the .git directory that git can't commit.
	gitDir := filepath.Join(testVaultRoot, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0644)

	os.Exit(m.Run())
}

func newService(root string) *vault.Service {
	return vault.New(root, vault.NewPathFilter(defaultIgnore, defaultExts))
}

// ----------------------------------------------------------------------------
// ResolvePath
// ----------------------------------------------------------------------------

func TestService_ResolvePath(t *testing.T) {
	t.Parallel()

	svc := newService(testVaultRoot)

	tests := []struct {
		name       string
		inputPath  string
		wantSuffix string // non-empty: resolved path should have this suffix
		wantErr    error  // non-nil: errors.Is check
	}{
		{
			name:       "happy path notes/simple.md",
			inputPath:  "Notes/simple.md",
			wantSuffix: "Notes/simple.md",
		},
		{
			name:      "path traversal with double dot",
			inputPath: "../etc/passwd",
			wantErr:   vault.ErrPathTraversal,
		},
		{
			name:      "ignored path .git/config",
			inputPath: ".git/config",
			wantErr:   vault.ErrPathRestricted,
		},
		{
			name:      "ignored path .obsidian/workspace.json",
			inputPath: ".obsidian/workspace.json",
			wantErr:   vault.ErrPathRestricted,
		},
		{
			name:      "not found",
			inputPath: "Notes/nonexistent.md",
			wantErr:   vault.ErrNotFound,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.ResolvePath(tc.inputPath)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("ResolvePath(%q) = %q, want error wrapping %v", tc.inputPath, got, tc.wantErr)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("ResolvePath(%q) error = %v, want errors.Is(%v)", tc.inputPath, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePath(%q) unexpected error: %v", tc.inputPath, err)
			}
			if tc.wantSuffix != "" && !strings.HasSuffix(got, tc.wantSuffix) {
				t.Errorf("ResolvePath(%q) = %q, want suffix %q", tc.inputPath, got, tc.wantSuffix)
			}
		})
	}
}

func TestService_ResolvePath_CaseInsensitiveFallback(t *testing.T) {
	t.Parallel()

	// Use a temp dir to control the filesystem state exactly.
	// Create a file "Notes/simple.md" and look it up as "Notes/SIMPLE.MD"
	// to exercise the case-insensitive fallback code path.
	dir := t.TempDir()
	notesDir := filepath.Join(dir, "Notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "simple.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

	// On a case-insensitive FS (macOS default), direct stat succeeds — fine.
	// On a case-sensitive FS, the fallback must find "simple.md" for "SIMPLE.MD".
	resolved, err := svc.ResolvePath("Notes/SIMPLE.MD")
	if err != nil {
		t.Fatalf("ResolvePath(Notes/SIMPLE.MD) unexpected error: %v", err)
	}
	// Either the OS matched directly or the fallback matched — both are correct.
	base := strings.ToLower(filepath.Base(resolved))
	if base != "simple.md" {
		t.Errorf("resolved base = %q, want \"simple.md\"", base)
	}
}

func TestService_ResolvePath_AmbiguousCase(t *testing.T) {
	t.Parallel()

	// Create two files whose names only differ by case to force ambiguity.
	// On case-insensitive filesystems (e.g. macOS default HFS+/APFS),
	// both writes succeed but only one directory entry exists — detect and skip.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Note.md"), []byte("a"), 0644); err != nil {
		t.Skipf("filesystem doesn't support first write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "NOTE.md"), []byte("b"), 0644); err != nil {
		t.Skipf("filesystem doesn't support second write: %v", err)
	}

	// Verify that two distinct directory entries actually exist.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var mdCount int
	for _, e := range entries {
		if strings.ToLower(e.Name()) == "note.md" {
			mdCount++
		}
	}
	if mdCount < 2 {
		t.Skip("filesystem is case-insensitive; cannot create two entries differing only in case")
	}

	svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

	_, err = svc.ResolvePath("note.md")
	if err == nil {
		t.Fatal("expected error for ambiguous case match, got nil")
	}
	if !errors.Is(err, vault.ErrAmbiguousPath) {
		t.Errorf("want ErrAmbiguousPath, got %v", err)
	}
}

// ----------------------------------------------------------------------------
// ReadNote
// ----------------------------------------------------------------------------

func TestService_ReadNote(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService(testVaultRoot)

	tests := []struct {
		name           string
		path           string
		wantSubstring  string
		wantErr        error
		wantSizeGtZero bool
		wantModTime    bool
	}{
		{
			name:           "read simple.md",
			path:           "Notes/simple.md",
			wantSubstring:  "Simple Note",
			wantSizeGtZero: true,
			wantModTime:    true,
		},
		{
			name:           "read with-fm.md contains frontmatter delimiters",
			path:           "Notes/with-fm.md",
			wantSubstring:  "---",
			wantSizeGtZero: true,
			wantModTime:    true,
		},
		{
			name:    "read nonexistent wraps ErrNotFound",
			path:    "Notes/nonexistent.md",
			wantErr: vault.ErrNotFound,
		},
		{
			name:    "read ignored path wraps ErrPathRestricted",
			path:    ".obsidian/workspace.json",
			wantErr: vault.ErrPathRestricted,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			note, err := svc.ReadNote(ctx, tc.path)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("ReadNote(%q) = %v, want error wrapping %v", tc.path, note, tc.wantErr)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("ReadNote(%q) error = %v, want errors.Is(%v)", tc.path, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadNote(%q) unexpected error: %v", tc.path, err)
			}
			if tc.wantSubstring != "" && !strings.Contains(note.Content, tc.wantSubstring) {
				t.Errorf("ReadNote(%q) content = %q, want substring %q", tc.path, note.Content, tc.wantSubstring)
			}
			if tc.wantSizeGtZero && note.Size <= 0 {
				t.Errorf("ReadNote(%q) Size = %d, want > 0", tc.path, note.Size)
			}
			if tc.wantModTime && note.ModTime.IsZero() {
				t.Errorf("ReadNote(%q) ModTime is zero", tc.path)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// WriteNote
// ----------------------------------------------------------------------------

func TestService_WriteNote(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("overwrite creates and replaces content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		path := "Notes/new-note.md"
		initial := "# Initial Content\n"
		if err := svc.WriteNote(ctx, path, initial, vault.WriteModeOverwrite); err != nil {
			t.Fatalf("WriteNote overwrite (create): %v", err)
		}

		note, err := svc.ReadNote(ctx, path)
		if err != nil {
			t.Fatalf("ReadNote after overwrite: %v", err)
		}
		if note.Content != initial {
			t.Errorf("content = %q, want %q", note.Content, initial)
		}

		replacement := "# Replaced Content\n"
		if err := svc.WriteNote(ctx, path, replacement, vault.WriteModeOverwrite); err != nil {
			t.Fatalf("WriteNote overwrite (replace): %v", err)
		}
		note, err = svc.ReadNote(ctx, path)
		if err != nil {
			t.Fatalf("ReadNote after replacement: %v", err)
		}
		if note.Content != replacement {
			t.Errorf("content = %q, want %q", note.Content, replacement)
		}
	})

	t.Run("append adds to end of existing content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		path := "Notes/append-note.md"
		if err := svc.WriteNote(ctx, path, "first\n", vault.WriteModeOverwrite); err != nil {
			t.Fatal(err)
		}
		if err := svc.WriteNote(ctx, path, "second\n", vault.WriteModeAppend); err != nil {
			t.Fatal(err)
		}

		note, err := svc.ReadNote(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		want := "first\nsecond\n"
		if note.Content != want {
			t.Errorf("content = %q, want %q", note.Content, want)
		}
	})

	t.Run("prepend adds to start of existing content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		path := "Notes/prepend-note.md"
		if err := svc.WriteNote(ctx, path, "body\n", vault.WriteModeOverwrite); err != nil {
			t.Fatal(err)
		}
		if err := svc.WriteNote(ctx, path, "header\n", vault.WriteModePrepend); err != nil {
			t.Fatal(err)
		}

		note, err := svc.ReadNote(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		want := "header\nbody\n"
		if note.Content != want {
			t.Errorf("content = %q, want %q", note.Content, want)
		}
	})

	t.Run("create new file in non-existent directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		path := "Deep/Nested/Dir/new-note.md"
		if err := svc.WriteNote(ctx, path, "content\n", vault.WriteModeOverwrite); err != nil {
			t.Fatalf("WriteNote for new nested path: %v", err)
		}

		note, err := svc.ReadNote(ctx, path)
		if err != nil {
			t.Fatalf("ReadNote after deep create: %v", err)
		}
		if note.Content != "content\n" {
			t.Errorf("content = %q, want %q", note.Content, "content\n")
		}
	})

	t.Run("append to non-existent file creates it", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		path := "Notes/brand-new.md"
		if err := svc.WriteNote(ctx, path, "appended\n", vault.WriteModeAppend); err != nil {
			t.Fatalf("WriteNote append to new file: %v", err)
		}

		note, err := svc.ReadNote(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if note.Content != "appended\n" {
			t.Errorf("content = %q, want %q", note.Content, "appended\n")
		}
	})

	t.Run("unknown mode returns error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		err := svc.WriteNote(ctx, "Notes/x.md", "content", vault.WriteMode("bogus"))
		if err == nil {
			t.Fatal("expected error for unknown write mode, got nil")
		}
		if !strings.Contains(err.Error(), "bogus") {
			t.Errorf("error = %v, want message containing \"bogus\"", err)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		err := svc.WriteNote(ctx, "../outside.md", "bad", vault.WriteModeOverwrite)
		if err == nil {
			t.Fatal("expected ErrPathTraversal, got nil")
		}
		if !errors.Is(err, vault.ErrPathTraversal) {
			t.Errorf("error = %v, want errors.Is(ErrPathTraversal)", err)
		}
	})

	t.Run("ignored path rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		err := svc.WriteNote(ctx, ".obsidian/config.md", "bad", vault.WriteModeOverwrite)
		if err == nil {
			t.Fatal("expected ErrPathRestricted, got nil")
		}
		if !errors.Is(err, vault.ErrPathRestricted) {
			t.Errorf("error = %v, want errors.Is(ErrPathRestricted)", err)
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		err := svc.WriteNote(ctx, "/etc/passwd", "bad", vault.WriteModeOverwrite)
		if err == nil {
			t.Fatal("expected ErrPathTraversal, got nil")
		}
		if !errors.Is(err, vault.ErrPathTraversal) {
			t.Errorf("error = %v, want errors.Is(ErrPathTraversal)", err)
		}
	})

	t.Run("symlink escape rejected", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()
		outsideDir := t.TempDir()

		symlinkDir := filepath.Join(vaultDir, "EscapeDir")
		if err := os.Symlink(outsideDir, symlinkDir); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}

		svc := vault.New(vaultDir, vault.NewPathFilter(defaultIgnore, defaultExts))

		err := svc.WriteNote(ctx, "EscapeDir/secret.md", "bad", vault.WriteModeOverwrite)
		if err == nil {
			t.Fatal("expected error for symlink escape, got nil")
		}
		if !errors.Is(err, vault.ErrSymlinkEscape) {
			t.Errorf("error = %v, want errors.Is(ErrSymlinkEscape)", err)
		}
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()

		err := svc.WriteNote(cancelCtx, "Notes/cancelled.md", "content", vault.WriteModeOverwrite)
		if err == nil {
			t.Fatal("expected error for cancelled context, got nil")
		}
	})
}

// ----------------------------------------------------------------------------
// ListDirectory
// ----------------------------------------------------------------------------

func TestService_ListDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService(testVaultRoot)

	t.Run("list root excludes .obsidian and .git", func(t *testing.T) {
		t.Parallel()
		entries, err := svc.ListDirectory(ctx, "")
		if err != nil {
			t.Fatalf("ListDirectory(\"\") error: %v", err)
		}
		for _, e := range entries {
			if e.Name == ".obsidian" || e.Name == ".git" {
				t.Errorf("ListDirectory(\"\") returned ignored entry %q", e.Name)
			}
		}
		// Root should have at least some entries (Notes, Daily Notes, Nested).
		if len(entries) == 0 {
			t.Error("ListDirectory(\"\") returned no entries")
		}
	})

	t.Run("list Notes directory returns expected files", func(t *testing.T) {
		t.Parallel()
		entries, err := svc.ListDirectory(ctx, "Notes")
		if err != nil {
			t.Fatalf("ListDirectory(\"Notes\") error: %v", err)
		}

		names := make(map[string]bool)
		for _, e := range entries {
			names[e.Name] = true
		}

		expected := []string{"simple.md", "with-fm.md", "tagged.md", "linked.md", "unicode.md"}
		for _, want := range expected {
			if !names[want] {
				t.Errorf("ListDirectory(\"Notes\") missing %q; got %v", want, entries)
			}
		}
	})

	t.Run("list nonexistent directory returns error", func(t *testing.T) {
		t.Parallel()
		_, err := svc.ListDirectory(ctx, "NonExistent")
		if err == nil {
			t.Fatal("expected error for nonexistent directory, got nil")
		}
		if !errors.Is(err, vault.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("DirEntry fields are populated", func(t *testing.T) {
		t.Parallel()
		entries, err := svc.ListDirectory(ctx, "Notes")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name == "" {
				t.Error("DirEntry.Name is empty")
			}
			if e.Path == "" {
				t.Error("DirEntry.Path is empty")
			}
			if !e.IsDir && e.Size <= 0 {
				t.Errorf("DirEntry %q: Size = %d, want > 0", e.Name, e.Size)
			}
			if e.ModTime.IsZero() {
				t.Errorf("DirEntry %q: ModTime is zero", e.Name)
			}
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		_, err := svc.ListDirectory(ctx, "../")
		if err == nil {
			t.Fatal("expected ErrPathTraversal, got nil")
		}
		if !errors.Is(err, vault.ErrPathTraversal) {
			t.Errorf("error = %v, want errors.Is(ErrPathTraversal)", err)
		}
	})

	t.Run("empty directory returns non-nil empty slice", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

		entries, err := svc.ListDirectory(ctx, "")
		if err != nil {
			t.Fatalf("ListDirectory on empty dir: %v", err)
		}
		if entries == nil {
			t.Fatal("expected non-nil slice, got nil")
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		t.Parallel()
		svc := newService(testVaultRoot)

		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := svc.ListDirectory(cancelCtx, "")
		if err == nil {
			t.Fatal("expected error for cancelled context, got nil")
		}
	})
}

func TestService_ReadNote_CancelledContext(t *testing.T) {
	t.Parallel()

	svc := newService(testVaultRoot)

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ReadNote(cancelCtx, "Notes/simple.md")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
