package httptransport

import (
	"bytes"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

func weakTokenLog(t *testing.T, tok authToken) string {
	t.Helper()
	var buf bytes.Buffer
	warnWeakToken(slog.New(slog.NewTextHandler(&buf, nil)), tok)
	return buf.String()
}

func TestWarnWeakToken_ShortEnvTokenWarnsWithoutLeaking(t *testing.T) {
	const secret = "hunter2-short"
	t.Setenv(envAuthToken, secret)

	tok, err := loadOrCreateToken(t.TempDir())
	if err != nil {
		t.Fatalf("loadOrCreateToken: %v", err)
	}
	out := weakTokenLog(t, tok)

	if !strings.Contains(out, "level=WARN") {
		t.Fatalf("expected a WARN line for a short token, got %q", out)
	}
	if !strings.Contains(out, "length=13") {
		t.Errorf("expected the token length in the warning, got %q", out)
	}
	if !strings.Contains(out, "32") {
		t.Errorf("expected the recommended minimum in the warning, got %q", out)
	}
	if strings.Contains(out, secret) {
		t.Errorf("warning leaked the token value: %q", out)
	}
}

func TestWarnWeakToken_LongEnvTokenIsSilent(t *testing.T) {
	t.Setenv(envAuthToken, strings.Repeat("a", minTokenLen))

	tok, err := loadOrCreateToken(t.TempDir())
	if err != nil {
		t.Fatalf("loadOrCreateToken: %v", err)
	}
	if out := weakTokenLog(t, tok); out != "" {
		t.Errorf("expected no log output for a %d-char token, got %q", minTokenLen, out)
	}
}

func TestWarnWeakToken_GeneratedTokenIsSilentAndStrong(t *testing.T) {
	t.Setenv(envAuthToken, "")

	tok, err := loadOrCreateToken(t.TempDir())
	if err != nil {
		t.Fatalf("loadOrCreateToken: %v", err)
	}
	if out := weakTokenLog(t, tok); out != "" {
		t.Errorf("expected no warning for a generated token, got %q", out)
	}
	data, err := os.ReadFile(tok.path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	raw, err := hex.DecodeString(string(data))
	if err != nil {
		t.Fatalf("generated token is not hex: %v", err)
	}
	if len(raw) < 32 {
		t.Errorf("generated token has %d bytes of entropy, want >= 32", len(raw))
	}
}
