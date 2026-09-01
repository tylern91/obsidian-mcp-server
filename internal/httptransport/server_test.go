package httptransport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/tylern91/obsidian-mcp-server/internal/config"
)

func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"192.168.1.1", false},
		{"example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isLoopbackHost(tt.host); got != tt.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestToSet(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want map[string]struct{}
	}{
		{"nil", nil, map[string]struct{}{}},
		{"empty", []string{}, map[string]struct{}{}},
		{"single", []string{"a"}, map[string]struct{}{"a": {}}},
		{"dedup", []string{"a", "a", "b"}, map[string]struct{}{"a": {}, "b": {}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toSet(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("toSet(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for k := range tt.want {
				if _, ok := got[k]; !ok {
					t.Errorf("toSet(%v) missing key %q", tt.in, k)
				}
			}
		})
	}
}

func TestRun_RefusesNonLoopbackWithoutAllowFlag(t *testing.T) {
	cfg := &config.Config{
		Transport: "http",
		HTTPBind:  "0.0.0.0",
		HTTPPort:  8443,
	}

	err := Run(context.Background(), mcpserver.NewMCPServer("test", "0.0.0-test"), cfg, testLogger())
	if err == nil {
		t.Fatal("expected error for non-loopback bind without --allow-non-loopback, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to bind non-loopback") {
		t.Errorf("error = %q, want it to mention refusing a non-loopback bind", err.Error())
	}
}

// TestRun_ServesHTTPSWithAuth is an end-to-end smoke test: it starts the real
// listener Run builds, confirms auth is enforced, ALPN negotiates h2 (the
// free win TLS gives us over HTTP/1.1), and that Run shuts down cleanly when
// its context is canceled.
func TestRun_ServesHTTPSWithAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	port := freePort(t)
	cfg := &config.Config{
		Transport: "http",
		HTTPBind:  "127.0.0.1",
		HTTPPort:  port,
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, mcpserver.NewMCPServer("test", "0.0.0-test"), cfg, testLogger())
	}()

	waitForTLSListener(t, addr)

	dir, err := stateDir()
	if err != nil {
		t.Fatalf("stateDir: %v", err)
	}
	tokenBytes, err := os.ReadFile(filepath.Join(dir, tokenFileName))
	if err != nil {
		t.Fatalf("read generated token file: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test client trusting a self-signed cert we just generated
			ForceAttemptHTTP2: true,                                  // setting TLSClientConfig otherwise disables automatic HTTP/2 promotion
		},
	}

	req, err := http.NewRequest(http.MethodGet, "https://"+addr+"/mcp", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do (no auth): %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status without Authorization = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	req2, err := http.NewRequest(http.MethodGet, "https://"+addr+"/mcp", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req2.Header.Set("Authorization", "Bearer "+string(tokenBytes))
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("Do (with auth): %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode == http.StatusUnauthorized || resp2.StatusCode == http.StatusForbidden {
		t.Errorf("status with correct bearer token = %d, want auth to pass (not 401/403)", resp2.StatusCode)
	}
	if resp2.TLS == nil || resp2.TLS.NegotiatedProtocol != "h2" {
		t.Errorf("ALPN negotiated protocol = %+v, want h2", resp2.TLS)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned error after context cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not shut down within 5s of context cancellation")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// waitForTLSListener polls addr until a TLS handshake succeeds or the
// deadline elapses, avoiding a blind sleep before the async Run goroutine's
// listener is actually accepting connections.
func waitForTLSListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: 100 * time.Millisecond},
			"tcp", addr,
			&tls.Config{InsecureSkipVerify: true}, //nolint:gosec // liveness probe only
		)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("listener at %s did not become ready within 5s", addr)
}
