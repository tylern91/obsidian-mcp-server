package httptransport

import (
	"crypto/sha256"
	"crypto/subtle"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testToken(t *testing.T, raw string) authToken {
	t.Helper()
	return authToken{hash: sha256.Sum256([]byte(raw))}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestHostAllowed(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		allowed map[string]struct{}
		want    bool
	}{
		{"loopback default localhost", "localhost", nil, true},
		{"loopback default 127.0.0.1", "127.0.0.1", nil, true},
		{"loopback default ::1", "::1", nil, true},
		{"loopback default rejects other", "evil.example.com", nil, false},
		{"explicit allowlist match", "internal.example.com", map[string]struct{}{"internal.example.com": {}}, true},
		{"explicit allowlist rejects localhost when not listed", "localhost", map[string]struct{}{"internal.example.com": {}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hostAllowed(tt.host, tt.allowed); got != tt.want {
				t.Errorf("hostAllowed(%q, %v) = %v, want %v", tt.host, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestOriginAllowed(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		allowed map[string]struct{}
		want    bool
	}{
		{"empty allowlist accepts any origin", "https://anything.example.com", nil, true},
		{"explicit allowlist match", "https://app.example.com", map[string]struct{}{"https://app.example.com": {}}, true},
		{"explicit allowlist rejects unlisted", "https://evil.example.com", map[string]struct{}{"https://app.example.com": {}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := originAllowed(tt.origin, tt.allowed); got != tt.want {
				t.Errorf("originAllowed(%q, %v) = %v, want %v", tt.origin, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestStripPort(t *testing.T) {
	tests := []struct{ host, want string }{
		{"localhost", "localhost"},
		{"localhost:8443", "localhost"},
		{"127.0.0.1:8443", "127.0.0.1"},
		{"127.0.0.1", "127.0.0.1"},
		{"[::1]:8443", "[::1]"},
		{"[::1]", "[::1]"},
	}
	for _, tt := range tests {
		if got := stripPort(tt.host); got != tt.want {
			t.Errorf("stripPort(%q) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header    string
		wantToken string
		wantOK    bool
	}{
		{"Bearer abc123", "abc123", true},
		{"", "", false},
		{"Basic abc123", "", false},
		{"Bearer ", "", false},
		{"Bearer", "", false},
	}
	for _, tt := range tests {
		token, ok := bearerToken(tt.header)
		if ok != tt.wantOK || token != tt.wantToken {
			t.Errorf("bearerToken(%q) = (%q, %v), want (%q, %v)", tt.header, token, ok, tt.wantToken, tt.wantOK)
		}
	}
}

func TestHostOriginMiddleware_RejectsDisallowedHost(t *testing.T) {
	cfg := &chainConfig{logger: testLogger()}
	srv := httptest.NewServer(cfg.hostOriginMiddleware(okHandler()))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = "evil.example.com"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestHostOriginMiddleware_RejectsDisallowedOrigin(t *testing.T) {
	// An explicit allowedOrigins set is required to exercise rejection — an
	// empty set deliberately accepts any Origin (see originAllowed), since a
	// loopback-only listener has no cross-origin attacker to defend against
	// beyond what the Host check already covers.
	cfg := &chainConfig{
		logger:         testLogger(),
		allowedOrigins: map[string]struct{}{"https://app.example.com": {}},
	}
	srv := httptest.NewServer(cfg.hostOriginMiddleware(okHandler()))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = "127.0.0.1"
	req.Header.Set("Origin", "https://evil.example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestHostOriginMiddleware_AllowsLoopbackWithNoOrigin(t *testing.T) {
	cfg := &chainConfig{logger: testLogger()}
	srv := httptest.NewServer(cfg.hostOriginMiddleware(okHandler()))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = "127.0.0.1"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestBodyCapMiddleware_RejectsOversizedBody(t *testing.T) {
	cfg := &chainConfig{logger: testLogger()}
	handler := cfg.bodyCapMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	oversized := strings.NewReader(strings.Repeat("a", maxRequestBodyBytes+1))
	resp, err := http.Post(srv.URL, "application/octet-stream", oversized)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
}

func TestBodyCapMiddleware_AllowsBodyUnderCap(t *testing.T) {
	cfg := &chainConfig{logger: testLogger()}
	handler := cfg.bodyCapMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/octet-stream", strings.NewReader("small body"))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAuthMiddleware_RejectsMissingOrWrongToken(t *testing.T) {
	cfg := &chainConfig{token: testToken(t, "correct-token"), logger: testLogger()}
	srv := httptest.NewServer(cfg.authMiddleware(okHandler()))
	defer srv.Close()

	tests := []struct {
		name   string
		header string
	}{
		{"missing header", ""},
		{"wrong token", "Bearer wrong-token"},
		{"malformed header", "wrong-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
			if got := resp.Header.Get("WWW-Authenticate"); got == "" {
				t.Error("expected WWW-Authenticate header on 401 response")
			}
		})
	}
}

func TestAuthMiddleware_AcceptsCorrectToken(t *testing.T) {
	cfg := &chainConfig{token: testToken(t, "correct-token"), logger: testLogger()}
	srv := httptest.NewServer(cfg.authMiddleware(okHandler()))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer correct-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAuthMiddleware_SetsCredentialHashInContext(t *testing.T) {
	tok := testToken(t, "correct-token")
	cfg := &chainConfig{token: tok, logger: testLogger()}

	var gotHash string
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHash, gotOK = credentialHashFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(cfg.authMiddleware(next))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer correct-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if !gotOK || gotHash == "" {
		t.Fatal("expected a credential hash to be set in the request context")
	}

	want := credentialHashForRequest(&http.Request{TLS: nil}, tok)
	if gotHash != want {
		t.Errorf("credential hash = %q, want %q", gotHash, want)
	}
}

func TestCredentialHashForRequest_StableForSameToken(t *testing.T) {
	tok := testToken(t, "correct-token")
	req := &http.Request{}

	h1 := credentialHashForRequest(req, tok)
	h2 := credentialHashForRequest(req, tok)
	if h1 != h2 {
		t.Error("credentialHashForRequest is not stable for the same token and no TLS state")
	}

	other := testToken(t, "different-token")
	h3 := credentialHashForRequest(req, other)
	if subtle.ConstantTimeCompare([]byte(h1), []byte(h3)) == 1 {
		t.Error("credentialHashForRequest produced the same hash for two different tokens")
	}
}

func TestBuildChain_OrderingHostRejectionPrecedesAuth(t *testing.T) {
	cfg := &chainConfig{token: testToken(t, "correct-token"), logger: testLogger()}
	srv := httptest.NewServer(cfg.buildChain(okHandler()))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = "evil.example.com"
	// No Authorization header — if auth ran first this would be 401, not 403.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d (host/origin must be checked before auth)", resp.StatusCode, http.StatusForbidden)
	}
}

func TestBuildChain_FullChainAcceptsValidRequest(t *testing.T) {
	cfg := &chainConfig{token: testToken(t, "correct-token"), logger: testLogger()}
	srv := httptest.NewServer(cfg.buildChain(okHandler()))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = "127.0.0.1"
	req.Header.Set("Authorization", "Bearer correct-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
