package acceptance_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/config"
	"github.com/tylern91/obsidian-mcp-server/internal/httptransport"
)

// discardLogger silences transport logging during acceptance runs.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// freeHTTPPort returns an ephemeral loopback port not currently in use.
func freeHTTPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// waitForHTTPSListener polls addr until a TLS handshake succeeds or times out.
func waitForHTTPSListener(t *testing.T, addr string) {
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

// startHTTPTransport runs httptransport.Run in the background against a
// fresh isolated state dir (via HOME/XDG_CONFIG_HOME) and returns the
// listener address plus a client trusting the self-signed cert. Cleans up
// via t.Cleanup by canceling the context and waiting for Run to return.
func startHTTPTransport(t *testing.T, cfg *config.Config) (addr string, hc *http.Client) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	port := freeHTTPPort(t)
	cfg.Transport = "http"
	cfg.HTTPBind = "127.0.0.1"
	cfg.HTTPPort = port
	addr = fmt.Sprintf("127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- httptransport.Run(ctx, mcpserver.NewMCPServer(t.Name(), "0.0.0-test"), cfg, discardLogger())
	}()

	t.Cleanup(func() {
		cancel()
		// No tight timing bound here: this cleanup runs after every test that
		// calls startHTTPTransport, not just AC-8 (which alone asserts on
		// shutdown latency). A generous bound only catches a genuinely hung
		// server, so a mutation AC-8 is designed to catch doesn't collaterally
		// fail every other subtest's teardown too.
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("httptransport.Run returned error after shutdown: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("httptransport.Run did not shut down within 30s")
		}
	})

	waitForHTTPSListener(t, addr)

	hc = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test client trusting a self-signed cert we just generated
			ForceAttemptHTTP2: true,
		},
	}
	return addr, hc
}

func readGeneratedToken(t *testing.T) string {
	t.Helper()
	dir, err := os.UserConfigDir()
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "obsidian-mcp", "token"))
	require.NoError(t, err)
	return string(data)
}

func TestHTTPTransport_Acceptance(t *testing.T) {
	t.Run("[AC-1] refuses to bind a non-loopback address without --allow-non-loopback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

		cfg := &config.Config{Transport: "http", HTTPBind: "0.0.0.0", HTTPPort: freeHTTPPort(t)}

		// Bounded so a regression that removes the early refusal (and lets Run
		// actually bind and serve) fails fast instead of hanging the suite.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- httptransport.Run(ctx, mcpserver.NewMCPServer(t.Name(), "0.0.0-test"), cfg, discardLogger())
		}()

		select {
		case err := <-errCh:
			require.Error(t, err)
			require.Contains(t, err.Error(), "refusing to bind non-loopback")
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not refuse the non-loopback bind within 5s")
		}
	})

	t.Run("[AC-2] rejects a request with no bearer token", func(t *testing.T) {
		addr, hc := startHTTPTransport(t, &config.Config{})
		resp, err := hc.Get("https://" + addr + "/mcp")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.NotEmpty(t, resp.Header.Get("WWW-Authenticate"))
	})

	t.Run("[AC-3] accepts a request bearing the auto-generated bearer token", func(t *testing.T) {
		addr, hc := startHTTPTransport(t, &config.Config{})
		token := readGeneratedToken(t)

		req, err := http.NewRequest(http.MethodGet, "https://"+addr+"/mcp", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := hc.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.NotEqual(t, http.StatusUnauthorized, resp.StatusCode)
		require.NotEqual(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("[AC-4] rejects a request whose Host header is not in a configured allowlist", func(t *testing.T) {
		// mcp-go's own StreamableHTTPServer has independent, default-on
		// DNS-rebinding protection that rejects any non-"localhost"-form Host
		// header over a loopback connection (see its WithDisableLocalhostProtection
		// doc comment) — that would mask a broken hostAllowed allowlist check
		// if this test sent a Host like "evil.example.com": both layers would
		// 403 it, mutation or not. Configuring a custom, non-localhost
		// allowlist and sending "localhost" instead isolates our own
		// hostAllowed logic: mcp-go accepts "localhost" unconditionally, so a
		// 403 here can only come from our own allowlist rejecting it.
		addr, hc := startHTTPTransport(t, &config.Config{AllowedHosts: []string{"allowed.example.com"}})
		token := readGeneratedToken(t)

		req, err := http.NewRequest(http.MethodGet, "https://"+addr+"/mcp", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Host = "localhost"
		resp, err := hc.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("[AC-5] rejects a request whose Origin header is not in a configured allowlist", func(t *testing.T) {
		addr, hc := startHTTPTransport(t, &config.Config{AllowedOrigins: []string{"https://allowed.example.com"}})
		token := readGeneratedToken(t)

		req, err := http.NewRequest(http.MethodGet, "https://"+addr+"/mcp", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Origin", "https://not-allowed.example.com")
		resp, err := hc.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("[AC-6] rejects a request body larger than the size cap", func(t *testing.T) {
		// A syntactically invalid or empty oversized body gets a 400 from
		// mcp-go's own JSON parse-error path regardless of whether our body
		// cap ran, which would make this test blind to a broken cap. Using a
		// syntactically valid "ping response" body (an empty JSON-RPC result,
		// which mcp-go accepts with 202 Accepted with no further processing)
		// padded past the cap isolates our own bodyCapMiddleware: uncapped,
		// mcp-go reads and accepts the whole thing (202); capped, the body
		// read fails before mcp-go ever sees valid JSON (400).
		addr, hc := startHTTPTransport(t, &config.Config{})
		token := readGeneratedToken(t)

		pad := strings.Repeat("a", (16<<20)+1024)
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{},"pad":"%s"}`, pad)
		req, err := http.NewRequest(http.MethodPost, "https://"+addr+"/mcp", strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := hc.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("[AC-7] negotiates TLS 1.3 and persists the certificate and token across a restart", func(t *testing.T) {
		addr, hc := startHTTPTransport(t, &config.Config{})
		token1 := readGeneratedToken(t)

		req, err := http.NewRequest(http.MethodGet, "https://"+addr+"/mcp", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token1)
		resp, err := hc.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.NotNil(t, resp.TLS)
		require.Equal(t, uint16(tls.VersionTLS13), resp.TLS.Version)

		// Second Run against the same state dir (HOME unchanged) must reuse the
		// same token rather than regenerating one.
		port2 := freeHTTPPort(t)
		cfg2 := &config.Config{Transport: "http", HTTPBind: "127.0.0.1", HTTPPort: port2}
		addr2 := fmt.Sprintf("127.0.0.1:%d", port2)
		ctx2, cancel2 := context.WithCancel(context.Background())
		errCh2 := make(chan error, 1)
		go func() {
			errCh2 <- httptransport.Run(ctx2, mcpserver.NewMCPServer(t.Name()+"-2", "0.0.0-test"), cfg2, discardLogger())
		}()
		t.Cleanup(func() {
			cancel2()
			<-errCh2
		})
		waitForHTTPSListener(t, addr2)
		token2 := readGeneratedToken(t)
		require.Equal(t, token1, token2)
	})

	t.Run("[AC-8] shuts down cleanly when its context is canceled", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

		port := freeHTTPPort(t)
		cfg := &config.Config{Transport: "http", HTTPBind: "127.0.0.1", HTTPPort: port}
		addr := fmt.Sprintf("127.0.0.1:%d", port)

		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			errCh <- httptransport.Run(ctx, mcpserver.NewMCPServer(t.Name(), "0.0.0-test"), cfg, discardLogger())
		}()
		waitForHTTPSListener(t, addr)

		cancel()
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not shut down within 5s of context cancellation")
		}
	})
}
