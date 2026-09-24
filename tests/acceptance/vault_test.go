package acceptance_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// newVaultService copies the fixture vault into a fresh temp dir and returns
// a raw *vault.Service, for tests that need direct vault-layer access rather
// than the tools.Deps wrapper newVaultDeps (helper_test.go) provides.
func newVaultService(t *testing.T) *vault.Service {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir("../../testdata/vault", dir); err != nil {
		t.Fatalf("copy fixture vault: %v", err)
	}
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return vault.New(dir, filter)
}

func TestVault_Acceptance(t *testing.T) {
	ctx := context.Background()

	t.Run("[AC-1] path traversal outside the vault root is rejected", func(t *testing.T) {
		s := newVaultService(t)
		// Write a real file just outside root so a defeated traversal guard
		// would actually resolve it (existenceCheck would otherwise return
		// ErrNotFound for a nonexistent target, masking the guard).
		outside := filepath.Join(filepath.Dir(s.Root()), "outside.md")
		if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
			t.Fatalf("write outside file: %v", err)
		}
		if _, err := s.ResolvePath("../outside.md"); err == nil {
			t.Fatal("expected error for path traversal")
		}
	})

	t.Run("[AC-2] absolute paths are rejected", func(t *testing.T) {
		s := newVaultService(t)
		// Use a path that maps to a real, allowed-extension file once joined
		// under root, so only the explicit IsAbs guard can be responsible for
		// the rejection (an extension-filter or not-found error on an
		// arbitrary absolute path would mask a broken IsAbs check).
		if _, err := s.ResolvePath("/Notes/simple.md"); err == nil {
			t.Fatal("expected error for absolute path")
		}
	})

	t.Run("[AC-3] null bytes in a path are rejected", func(t *testing.T) {
		s := newVaultService(t)
		// Assert the specific sentinel: the OS syscall layer would also
		// reject a raw NUL byte, so checking err != nil alone wouldn't
		// isolate the application-level validation from that OS fallback.
		_, err := s.ResolvePath("Notes/simple\x00.md")
		if !errors.Is(err, vault.ErrPathTraversal) {
			t.Fatalf("expected ErrPathTraversal for null byte in path, got %v", err)
		}
	})

	t.Run("[AC-4] Windows-reserved device names are rejected", func(t *testing.T) {
		s := newVaultService(t)
		// A real file, so a defeated guard would actually resolve it instead
		// of being masked by an unrelated not-found error.
		if err := os.WriteFile(filepath.Join(s.Root(), "CON.md"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write CON.md: %v", err)
		}
		if _, err := s.ResolvePath("CON.md"); err == nil {
			t.Fatal("expected error for reserved device name")
		}
	})

	t.Run("[AC-5] internal state directory .obsidian-mcp is always excluded, even without a configured ignore pattern", func(t *testing.T) {
		filterNoIgnores := vault.NewPathFilter(nil, nil)
		dir := t.TempDir()
		if err := copyDir("../../testdata/vault", dir); err != nil {
			t.Fatalf("copy fixture vault: %v", err)
		}
		s := vault.New(dir, filterNoIgnores)
		stateDir := filepath.Join(dir, ".obsidian-mcp")
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatalf("mkdir .obsidian-mcp: %v", err)
		}
		if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write state.json: %v", err)
		}
		if _, err := s.ResolvePath(".obsidian-mcp/state.json"); err == nil {
			t.Fatal("expected .obsidian-mcp to be excluded regardless of ignore patterns")
		}
	})

	t.Run("[AC-6] a symlink resolving outside the vault root is rejected", func(t *testing.T) {
		dir := t.TempDir()
		if err := copyDir("../../testdata/vault", dir); err != nil {
			t.Fatalf("copy fixture vault: %v", err)
		}
		outside := t.TempDir()
		target := filepath.Join(outside, "escape.md")
		if err := os.WriteFile(target, []byte("escaped"), 0644); err != nil {
			t.Fatalf("write outside target: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "link.md")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		filter := vault.NewPathFilter([]string{".obsidian", ".git"}, []string{".md"})
		s := vault.New(dir, filter)
		if _, err := s.ReadNote(ctx, "link.md"); err == nil {
			t.Fatal("expected error reading a symlink that escapes the vault root")
		}
	})

	t.Run("[AC-7] reading an existing note returns its content and a non-empty etag", func(t *testing.T) {
		s := newVaultService(t)
		note, err := s.ReadNote(ctx, "Notes/simple.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		if !strings.Contains(note.Content, "This is a simple note") {
			t.Fatalf("unexpected content: %q", note.Content)
		}
		if note.Etag == "" {
			t.Fatal("expected non-empty etag")
		}
	})

	t.Run("[AC-8] reading a nonexistent note returns ErrNotFound", func(t *testing.T) {
		s := newVaultService(t)
		_, err := s.ReadNote(ctx, "Notes/does-not-exist.md")
		if !errors.Is(err, vault.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("[AC-9] resolving a path is case-insensitive when the exact case does not exist", func(t *testing.T) {
		s := newVaultService(t)
		// Case-insensitive fallback only covers the final path component
		// (existenceCheck matches within the parent dir); the directory
		// segment's case must match exactly, so only the filename's case
		// differs here. On macOS's default case-insensitive filesystem the
		// OS itself would mask a directory-case mismatch too, giving a
		// false pass that fails on CI's case-sensitive Linux runners.
		note, err := s.ReadNote(ctx, "Notes/SIMPLE.md")
		if err != nil {
			t.Fatalf("ReadNote case-insensitive: %v", err)
		}
		if !strings.Contains(note.Content, "This is a simple note") {
			t.Fatalf("unexpected content: %q", note.Content)
		}
	})

	t.Run("[AC-10] an ambiguous case-insensitive match returns ErrAmbiguousPath", func(t *testing.T) {
		// Two files whose names differ only by case can coexist only on a
		// case-sensitive filesystem — skip on case-insensitive filesystems
		// (e.g. macOS default APFS), matching the pre-migration unit test's
		// own skip convention for this same scenario.
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "Note.md"), []byte("a"), 0644); err != nil {
			t.Skipf("filesystem doesn't support first write: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("b"), 0644); err != nil {
			t.Skipf("filesystem doesn't support second write: %v", err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) < 2 {
			t.Skip("filesystem collapsed the two case-variant filenames into one entry")
		}
		filter := vault.NewPathFilter([]string{".obsidian", ".git"}, []string{".md"})
		s := vault.New(dir, filter)
		if _, err := s.ReadNote(ctx, "NOTE.MD"); err == nil {
			t.Fatal("expected ErrAmbiguousPath when multiple case-insensitive matches exist")
		}
	})

	t.Run("[AC-11] writing a new note with overwrite mode creates it and it can be read back", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.WriteNote(ctx, "Notes/new-note.md", "hello world", vault.WriteModeOverwrite); err != nil {
			t.Fatalf("WriteNote: %v", err)
		}
		note, err := s.ReadNote(ctx, "Notes/new-note.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		if note.Content != "hello world" {
			t.Fatalf("got content %q, want %q", note.Content, "hello world")
		}
	})

	t.Run("[AC-12] appending to an existing note preserves prior content and adds the new content after it", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.WriteNote(ctx, "Notes/append-target.md", "first", vault.WriteModeOverwrite); err != nil {
			t.Fatalf("seed WriteNote: %v", err)
		}
		if err := s.WriteNote(ctx, "Notes/append-target.md", "second", vault.WriteModeAppend); err != nil {
			t.Fatalf("append WriteNote: %v", err)
		}
		note, err := s.ReadNote(ctx, "Notes/append-target.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		if !strings.Contains(note.Content, "first") || !strings.Contains(note.Content, "second") {
			t.Fatalf("expected both first and second content, got %q", note.Content)
		}
		if strings.Index(note.Content, "first") > strings.Index(note.Content, "second") {
			t.Fatalf("expected first before second, got %q", note.Content)
		}
	})

	t.Run("[AC-13] prepending to an existing note places the new content before the prior content", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.WriteNote(ctx, "Notes/prepend-target.md", "original", vault.WriteModeOverwrite); err != nil {
			t.Fatalf("seed WriteNote: %v", err)
		}
		if err := s.WriteNote(ctx, "Notes/prepend-target.md", "prefix", vault.WriteModePrepend); err != nil {
			t.Fatalf("prepend WriteNote: %v", err)
		}
		note, err := s.ReadNote(ctx, "Notes/prepend-target.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		if strings.Index(note.Content, "prefix") > strings.Index(note.Content, "original") {
			t.Fatalf("expected prefix before original, got %q", note.Content)
		}
	})

	t.Run("[AC-14] writing with a stale If-Match etag is rejected with a conflict, and the note is unchanged", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.WriteNote(ctx, "Notes/etag-target.md", "v1", vault.WriteModeOverwrite); err != nil {
			t.Fatalf("seed WriteNote: %v", err)
		}
		err := s.WriteNote(ctx, "Notes/etag-target.md", "v2", vault.WriteModeOverwrite, vault.WithIfMatch("stale-etag"))
		if err == nil {
			t.Fatal("expected conflict error for stale etag")
		}
		note, readErr := s.ReadNote(ctx, "Notes/etag-target.md")
		if readErr != nil {
			t.Fatalf("ReadNote: %v", readErr)
		}
		if note.Content != "v1" {
			t.Fatalf("expected content unchanged at v1, got %q", note.Content)
		}
	})

	t.Run("[AC-15] writing with If-Match against a nonexistent note is a conflict, not an implicit create", func(t *testing.T) {
		s := newVaultService(t)
		err := s.WriteNote(ctx, "Notes/never-existed.md", "content", vault.WriteModeOverwrite, vault.WithIfMatch("any-etag"))
		if err == nil {
			t.Fatal("expected conflict error, got nil")
		}
		if _, readErr := s.ReadNote(ctx, "Notes/never-existed.md"); readErr == nil {
			t.Fatal("note should not have been created")
		}
	})

	t.Run("[AC-16] writing content larger than the max file size is rejected", func(t *testing.T) {
		s := newVaultService(t)
		big := strings.Repeat("a", 16*1024*1024+1)
		if err := s.WriteNote(ctx, "Notes/too-big.md", big, vault.WriteModeOverwrite); err == nil {
			t.Fatal("expected error for oversized content")
		}
	})

	t.Run("[AC-17] listing the vault root returns entries filtered by ignore patterns and allowed extensions", func(t *testing.T) {
		s := newVaultService(t)
		entries, err := s.ListDirectory(ctx, "")
		if err != nil {
			t.Fatalf("ListDirectory: %v", err)
		}
		names := make(map[string]bool)
		for _, e := range entries {
			names[e.Name] = true
		}
		if !names["Notes"] {
			t.Fatalf("expected Notes directory in root listing, got %+v", names)
		}
		if names[".obsidian"] || names[".git"] {
			t.Fatalf("expected .obsidian and .git to be filtered out, got %+v", names)
		}
	})

	t.Run("[AC-18] StatNote returns note metadata including title, tag count, link count, and etag without a second read round-trip mismatch", func(t *testing.T) {
		s := newVaultService(t)
		info, err := s.StatNote(ctx, "Notes/tagged.md")
		if err != nil {
			t.Fatalf("StatNote: %v", err)
		}
		if info.TagCount == 0 {
			t.Fatalf("expected non-zero tag count, got %d", info.TagCount)
		}
		if info.Etag == "" {
			t.Fatal("expected non-empty etag")
		}
	})

	t.Run("[AC-19] StatNote falls back to the filename stem for the title when frontmatter has none", func(t *testing.T) {
		s := newVaultService(t)
		info, err := s.StatNote(ctx, "Notes/simple.md")
		if err != nil {
			t.Fatalf("StatNote: %v", err)
		}
		if info.Title != "simple" {
			t.Fatalf("expected title fallback %q, got %q", "simple", info.Title)
		}
	})

	t.Run("[AC-20] frontmatter is parsed into fields with correct types (string, list, number, boolean)", func(t *testing.T) {
		s := newVaultService(t)
		fm, _, err := s.GetFrontmatter(ctx, "Notes/with-fm.md")
		if err != nil {
			t.Fatalf("GetFrontmatter: %v", err)
		}
		if fm["title"] != "Note With Frontmatter" {
			t.Fatalf("expected title field, got %+v", fm["title"])
		}
		if fm["priority"] != "high" {
			t.Fatalf("expected priority=high, got %+v", fm["priority"])
		}
		if fm["count"] != 42 {
			t.Fatalf("expected count=42, got %+v (%T)", fm["count"], fm["count"])
		}
		if fm["enabled"] != true {
			t.Fatalf("expected enabled=true, got %+v", fm["enabled"])
		}
	})

	t.Run("[AC-21] updating frontmatter preserves existing key order and unrelated values", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.UpdateFrontmatter(ctx, "Notes/with-fm.md", map[string]any{"priority": "low"}, nil); err != nil {
			t.Fatalf("UpdateFrontmatter: %v", err)
		}
		note, err := s.ReadNote(ctx, "Notes/with-fm.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		raw, _, hasFM := vault.SplitFrontmatter(note.Content)
		if !hasFM {
			t.Fatal("expected frontmatter to survive the update")
		}
		titleIdx := strings.Index(raw, "title:")
		priorityIdx := strings.Index(raw, "priority:")
		if titleIdx == -1 || priorityIdx == -1 || titleIdx > priorityIdx {
			t.Fatalf("expected title before priority (order preserved), raw=%q", raw)
		}
		if !strings.Contains(raw, "priority: low") {
			t.Fatalf("expected updated priority value, raw=%q", raw)
		}
	})

	t.Run("[AC-22] updating frontmatter can remove keys", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.UpdateFrontmatter(ctx, "Notes/with-fm.md", nil, []string{"count"}); err != nil {
			t.Fatalf("UpdateFrontmatter remove: %v", err)
		}
		fm, _, err := s.GetFrontmatter(ctx, "Notes/with-fm.md")
		if err != nil {
			t.Fatalf("GetFrontmatter: %v", err)
		}
		if _, exists := fm["count"]; exists {
			t.Fatalf("expected count key removed, still present: %+v", fm)
		}
	})

	t.Run("[AC-23] ExtractLinks finds wikilinks, embeds, aliased links, and markdown links in document order, deduplicated", func(t *testing.T) {
		s := newVaultService(t)
		note, err := s.ReadNote(ctx, "Notes/linked.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		links := vault.ExtractLinks(note.Content)
		want := []string{"simple", "with-fm", "Nested/Deep/note.md", "Notes/unicode"}
		if len(links) != len(want) {
			t.Fatalf("got links %v, want %v", links, want)
		}
		for i, w := range want {
			if links[i] != w {
				t.Fatalf("link[%d] = %q, want %q (full: %v)", i, links[i], w, links)
			}
		}
	})

	t.Run("[AC-24] GetBacklinks finds notes that link to a target and excludes the target itself", func(t *testing.T) {
		s := newVaultService(t)
		backlinks, err := s.GetBacklinks(ctx, "Notes/simple.md")
		if err != nil {
			t.Fatalf("GetBacklinks: %v", err)
		}
		if len(backlinks) == 0 {
			t.Fatal("expected at least one backlink to Notes/simple.md")
		}
		for _, bl := range backlinks {
			if bl.Path == "Notes/simple.md" {
				t.Fatal("target note should not backlink to itself")
			}
		}
	})

	t.Run("[AC-25] moving a note with updateLinks rewrites inbound links elsewhere in the vault", func(t *testing.T) {
		s := newVaultService(t)
		result, err := s.MoveNote(ctx, "Notes/simple.md", "Notes/renamed-simple.md", "Notes/simple.md", true, false)
		if err != nil {
			t.Fatalf("MoveNote: %v", err)
		}
		if !result.Moved {
			t.Fatal("expected Moved=true")
		}
		linked, err := s.ReadNote(ctx, "Notes/linked.md")
		if err != nil {
			t.Fatalf("ReadNote linked.md: %v", err)
		}
		if !strings.Contains(linked.Content, "renamed-simple") {
			t.Fatalf("expected inbound link rewritten to renamed-simple, got %q", linked.Content)
		}
	})

	t.Run("[AC-26] a dry-run move changes nothing on disk", func(t *testing.T) {
		s := newVaultService(t)
		result, err := s.MoveNote(ctx, "Notes/simple.md", "Notes/dry-run-target.md", "Notes/simple.md", true, true)
		if err != nil {
			t.Fatalf("MoveNote dry-run: %v", err)
		}
		if result.Moved {
			t.Fatal("expected Moved=false for dry run")
		}
		if _, err := s.ReadNote(ctx, "Notes/simple.md"); err != nil {
			t.Fatalf("expected source note untouched: %v", err)
		}
		if _, err := s.ReadNote(ctx, "Notes/dry-run-target.md"); err == nil {
			t.Fatal("expected destination to not exist after dry run")
		}
	})

	t.Run("[AC-27] moving a note to a confirm value that does not match src is rejected", func(t *testing.T) {
		s := newVaultService(t)
		if _, err := s.MoveNote(ctx, "Notes/simple.md", "Notes/moved.md", "wrong-confirm", false, false); err == nil {
			t.Fatal("expected ErrConfirmMismatch")
		}
	})

	t.Run("[AC-28] moving a note to a destination that already exists is rejected", func(t *testing.T) {
		s := newVaultService(t)
		if _, err := s.MoveNote(ctx, "Notes/simple.md", "Notes/tagged.md", "Notes/simple.md", false, false); err == nil {
			t.Fatal("expected ErrAlreadyExists")
		}
	})

	t.Run("[AC-29] deleting a note with a mismatched confirm value is rejected", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.DeleteNote(ctx, "Notes/simple.md", "wrong-confirm", false); err == nil {
			t.Fatal("expected ErrConfirmMismatch")
		}
	})

	t.Run("[AC-30] deleting a note without permanent moves it to trash and it is no longer readable at its original path", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md", false); err != nil {
			t.Fatalf("DeleteNote: %v", err)
		}
		if _, err := s.ReadNote(ctx, "Notes/simple.md"); err == nil {
			t.Fatal("expected note gone from its original path after trash-delete")
		}
	})

	t.Run("[AC-31] deleting a note with permanent=true removes it entirely, not to trash", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md", true); err != nil {
			t.Fatalf("DeleteNote permanent: %v", err)
		}
		if _, err := s.ReadNote(ctx, "Notes/simple.md"); err == nil {
			t.Fatal("expected note gone after permanent delete")
		}
	})

	t.Run("[AC-32] PruneTrash removes trash entries older than the retention window and keeps newer ones", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md", false); err != nil {
			t.Fatalf("DeleteNote: %v", err)
		}
		removed, err := s.PruneTrash(time.Now().Add(48*time.Hour), 1)
		if err != nil {
			t.Fatalf("PruneTrash: %v", err)
		}
		if removed == 0 {
			t.Fatal("expected at least one trash entry pruned when far past retention")
		}
	})

	t.Run("[AC-33] adding a frontmatter tag creates the tags sequence when absent and is idempotent when already present", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.AddTag(ctx, "Notes/simple.md", "newtag", "frontmatter"); err != nil {
			t.Fatalf("AddTag: %v", err)
		}
		tags, err := s.ListTags(ctx, "Notes/simple.md")
		if err != nil {
			t.Fatalf("ListTags: %v", err)
		}
		if !containsStr(tags, "newtag") {
			t.Fatalf("expected newtag present, got %v", tags)
		}
		if err := s.AddTag(ctx, "Notes/simple.md", "newtag", "frontmatter"); err != nil {
			t.Fatalf("AddTag idempotent: %v", err)
		}
		tags2, err := s.ListTags(ctx, "Notes/simple.md")
		if err != nil {
			t.Fatalf("ListTags: %v", err)
		}
		count := 0
		for _, tg := range tags2 {
			if tg == "newtag" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected newtag exactly once after duplicate add, got %d occurrences in %v", count, tags2)
		}
	})

	t.Run("[AC-34] adding an inline tag appends it to the note body", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.AddTag(ctx, "Notes/untagged.md", "fresh", "inline"); err != nil {
			t.Fatalf("AddTag inline: %v", err)
		}
		note, err := s.ReadNote(ctx, "Notes/untagged.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		if !strings.Contains(note.Content, "#fresh") {
			t.Fatalf("expected #fresh in content, got %q", note.Content)
		}
	})

	t.Run("[AC-35] removing a tag removes it from both frontmatter and inline occurrences", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.RemoveTag(ctx, "Notes/tagged.md", "project"); err != nil {
			t.Fatalf("RemoveTag frontmatter: %v", err)
		}
		if err := s.RemoveTag(ctx, "Notes/tagged.md", "todo"); err != nil {
			t.Fatalf("RemoveTag inline: %v", err)
		}
		tags, err := s.ListTags(ctx, "Notes/tagged.md")
		if err != nil {
			t.Fatalf("ListTags: %v", err)
		}
		if containsStr(tags, "project") || containsStr(tags, "todo") {
			t.Fatalf("expected project and todo removed, got %v", tags)
		}
	})

	t.Run("[AC-36] a tag written only inside a fenced code block is not extracted as a real tag", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.WriteNote(ctx, "Notes/fenced.md", "Body text.\n\n```\n#nottag in a fence\n```\n\nReal #realtag here.\n", vault.WriteModeOverwrite); err != nil {
			t.Fatalf("WriteNote: %v", err)
		}
		tags, err := s.ListTags(ctx, "Notes/fenced.md")
		if err != nil {
			t.Fatalf("ListTags: %v", err)
		}
		if containsStr(tags, "nottag") {
			t.Fatalf("expected fenced #nottag excluded, got %v", tags)
		}
		if !containsStr(tags, "realtag") {
			t.Fatalf("expected #realtag included, got %v", tags)
		}
	})

	t.Run("[AC-37] AggregateTags counts each tag once per note across the vault", func(t *testing.T) {
		s := newVaultService(t)
		counts, err := s.AggregateTags(ctx)
		if err != nil {
			t.Fatalf("AggregateTags: %v", err)
		}
		if counts["obsidian"] != 1 {
			t.Fatalf("expected obsidian tag counted once, got %d", counts["obsidian"])
		}
	})

	t.Run("[AC-38] RenameTag renames a tag across frontmatter and inline occurrences vault-wide", func(t *testing.T) {
		s := newVaultService(t)
		renames, err := s.RenameTag(ctx, "obsidian", "obsidian-mcp")
		if err != nil {
			t.Fatalf("RenameTag: %v", err)
		}
		if len(renames) == 0 {
			t.Fatal("expected at least one note renamed")
		}
		counts, err := s.AggregateTags(ctx)
		if err != nil {
			t.Fatalf("AggregateTags: %v", err)
		}
		if _, stillHasOld := counts["obsidian"]; stillHasOld {
			t.Fatalf("expected old tag gone, counts=%v", counts)
		}
		if counts["obsidian-mcp"] != 1 {
			t.Fatalf("expected new tag counted once, got %d", counts["obsidian-mcp"])
		}
	})

	t.Run("[AC-39] GetNoteOutline returns the heading tree with levels and line numbers", func(t *testing.T) {
		s := newVaultService(t)
		headings, err := s.GetNoteOutline(ctx, "Daily Notes/2024-01-15.md")
		if err != nil {
			t.Fatalf("GetNoteOutline: %v", err)
		}
		if len(headings) < 3 {
			t.Fatalf("expected at least 3 headings, got %+v", headings)
		}
		if headings[0].Level != 1 {
			t.Fatalf("expected first heading level 1, got %d", headings[0].Level)
		}
	})

	t.Run("[AC-40] ReadNoteLines returns a bounded line range and reports total line count", func(t *testing.T) {
		s := newVaultService(t)
		result, err := s.ReadNoteLines(ctx, "Daily Notes/2024-01-15.md", 1, 3)
		if err != nil {
			t.Fatalf("ReadNoteLines: %v", err)
		}
		if result.StartLine != 1 || result.EndLine != 3 {
			t.Fatalf("expected lines 1-3, got %d-%d", result.StartLine, result.EndLine)
		}
		if result.TotalLines <= 3 {
			t.Fatalf("expected total lines > 3, got %d", result.TotalLines)
		}
	})

	t.Run("[AC-41] PatchNote with position=after inserts content after a heading's body, before the next heading", func(t *testing.T) {
		s := newVaultService(t)
		err := s.PatchNote(ctx, "Daily Notes/2024-01-15.md", vault.PatchOp{
			Heading:  "Today's Goals",
			Position: "after",
			Content:  "Inserted line.",
		})
		if err != nil {
			t.Fatalf("PatchNote: %v", err)
		}
		note, err := s.ReadNote(ctx, "Daily Notes/2024-01-15.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		notesIdx := strings.Index(note.Content, "## Notes")
		insertedIdx := strings.Index(note.Content, "Inserted line.")
		if insertedIdx == -1 || notesIdx == -1 || insertedIdx > notesIdx {
			t.Fatalf("expected insertion before ## Notes, content=%q", note.Content)
		}
	})

	t.Run("[AC-42] PatchNote against a heading that does not exist returns ErrHeadingNotFound", func(t *testing.T) {
		s := newVaultService(t)
		err := s.PatchNote(ctx, "Notes/simple.md", vault.PatchOp{
			Heading:  "No Such Heading",
			Position: "after",
			Content:  "x",
		})
		if err == nil {
			t.Fatal("expected ErrHeadingNotFound")
		}
	})

	t.Run("[AC-43] ReplaceInNote replaces literal occurrences and reports the found/replaced counts", func(t *testing.T) {
		s := newVaultService(t)
		result, err := s.ReplaceInNote(ctx, "Notes/simple.md", "paragraph", "section", false, 0)
		if err != nil {
			t.Fatalf("ReplaceInNote: %v", err)
		}
		if result.OccurrencesFound != 2 || result.Replaced != 2 {
			t.Fatalf("expected 2 found and 2 replaced, got found=%d replaced=%d", result.OccurrencesFound, result.Replaced)
		}
		note, err := s.ReadNote(ctx, "Notes/simple.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		if strings.Contains(note.Content, "paragraph") {
			t.Fatalf("expected all occurrences replaced, got %q", note.Content)
		}
	})

	t.Run("[AC-44] ReplaceInNote caps replacements at maxOccurrences while still reporting the true total found", func(t *testing.T) {
		s := newVaultService(t)
		result, err := s.ReplaceInNote(ctx, "Notes/simple.md", "paragraph", "section", false, 1)
		if err != nil {
			t.Fatalf("ReplaceInNote: %v", err)
		}
		if result.OccurrencesFound != 2 || result.Replaced != 1 {
			t.Fatalf("expected found=2 replaced=1, got found=%d replaced=%d", result.OccurrencesFound, result.Replaced)
		}
	})

	t.Run("[AC-45] VaultStats aggregates note count, byte total, and link total across the vault in one walk", func(t *testing.T) {
		s := newVaultService(t)
		stats, err := s.VaultStats(ctx, vault.VaultStatsOpts{})
		if err != nil {
			t.Fatalf("VaultStats: %v", err)
		}
		if stats.NoteCount == 0 {
			t.Fatal("expected non-zero note count")
		}
		if stats.TotalBytes == 0 {
			t.Fatal("expected non-zero total bytes")
		}
		if stats.TotalLinks == 0 {
			t.Fatal("expected non-zero total links across the vault")
		}
	})

	t.Run("[AC-46] WalkNotes never visits files inside the internal state directory", func(t *testing.T) {
		s := newVaultService(t)
		if err := s.WriteNote(ctx, "Notes/marker-for-walk.md", "x", vault.WriteModeOverwrite); err != nil {
			t.Fatalf("seed WriteNote: %v", err)
		}
		visited := make(map[string]bool)
		err := s.WalkNotes(ctx, func(rel, abs string) error {
			visited[rel] = true
			return nil
		})
		if err != nil {
			t.Fatalf("WalkNotes: %v", err)
		}
		for rel := range visited {
			if strings.HasPrefix(rel, ".obsidian-mcp") {
				t.Fatalf("expected no internal-state paths visited, saw %q", rel)
			}
		}
		if !visited["Notes/marker-for-walk.md"] {
			t.Fatal("expected the newly written note to be visited")
		}
	})
}

func containsStr(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}
