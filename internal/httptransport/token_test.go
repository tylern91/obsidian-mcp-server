package httptransport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateToken_GeneratesAndPersists(t *testing.T) {
	dir := t.TempDir()

	first, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("loadOrCreateToken: %v", err)
	}
	if first.path == "" {
		t.Fatal("expected token path to be set for a generated token")
	}

	info, err := os.Stat(first.path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file permissions = %o, want 0600", perm)
	}

	second, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("loadOrCreateToken (reload): %v", err)
	}
	if first.hash != second.hash {
		t.Error("reloading the token file produced a different hash — token was not persisted")
	}
}

func TestLoadOrCreateToken_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tokenFileName), []byte("filetoken"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	t.Setenv(envAuthToken, "envtoken")

	tok, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("loadOrCreateToken: %v", err)
	}
	if !tok.compare("envtoken") {
		t.Error("expected env-provided token to win over the file")
	}
	if tok.path != "" {
		t.Errorf("expected empty path for an env-sourced token, got %q", tok.path)
	}
}

func TestAuthToken_Compare(t *testing.T) {
	dir := t.TempDir()
	tok, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("loadOrCreateToken: %v", err)
	}

	raw, err := os.ReadFile(tok.path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}

	if !tok.compare(string(raw)) {
		t.Error("compare(correct token) = false, want true")
	}
	if tok.compare("wrong-token") {
		t.Error("compare(wrong token) = true, want false")
	}
	if tok.compare("") {
		t.Error("compare(empty string) = true, want false")
	}
	if tok.compare(string(raw) + "x") {
		t.Error("compare(token with trailing byte) = true, want false")
	}
}
