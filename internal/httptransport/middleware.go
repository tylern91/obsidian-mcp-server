package httptransport

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
)

// maxRequestBodyBytes bounds every request body read by the streamable HTTP
// handler (A-CRIT-3) — without this, mcp-go's own io.ReadAll(r.Body) is an
// unauthenticated remote OOM vector.
const maxRequestBodyBytes = 16 << 20 // 16 MiB, matching the vault's own file-size cap

// chainConfig carries everything the middleware chain needs to evaluate a
// request. Built once at server startup from Config and the loaded token/CA.
type chainConfig struct {
	allowedHosts   map[string]struct{} // empty means loopback defaults apply
	allowedOrigins map[string]struct{} // empty means Origin header is not required
	token          authToken
	logger         *slog.Logger
}

// buildChain wraps next with the mandated ordering: Host/Origin allowlist,
// then a body-size cap, then bearer-token auth — all evaluated before mcp-go
// ever reads the request body. mTLS, when configured, is enforced by the
// *http.Server's tls.Config (ClientAuth: RequireAndVerifyClientCert) before a
// request reaches this chain at all, so it is not re-checked here.
func (c *chainConfig) buildChain(next http.Handler) http.Handler {
	return c.hostOriginMiddleware(c.bodyCapMiddleware(c.authMiddleware(next)))
}

// hostOriginMiddleware rejects requests whose Host or Origin header is not in
// the configured allowlist (A-CRIT-1). This is DNS-rebinding and CSRF-style
// cross-origin defense for a listener that otherwise has no browser same-
// origin protection of its own.
func (c *chainConfig) hostOriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := stripPort(r.Host)
		if !hostAllowed(host, c.allowedHosts) {
			c.logger.Warn("rejected request: host not allowed", "host", host)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if origin := r.Header.Get("Origin"); origin != "" {
			if !originAllowed(origin, c.allowedOrigins) {
				c.logger.Warn("rejected request: origin not allowed", "origin", origin)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// bodyCapMiddleware wraps the request body in http.MaxBytesReader so any
// downstream io.ReadAll (mcp-go's handlePost included) fails once the body
// exceeds maxRequestBodyBytes, instead of buffering it unbounded (A-CRIT-3).
func (c *chainConfig) bodyCapMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddleware enforces the bearer token and, on success, attaches a
// credential hash to the request context for sessionResolver to bind new
// sessions to (A-HIGH-4). Under mTLS the client certificate's raw bytes are
// folded into that hash, so a session is bound to the pairing of the two
// credentials, not the token alone. The Authorization header is never logged.
func (c *chainConfig) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok || !c.token.compare(presented) {
			c.logger.Warn("rejected request: invalid or missing bearer token")
			w.Header().Set("WWW-Authenticate", `Bearer realm="obsidian-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := withCredentialHash(r.Context(), credentialHashForRequest(r, c.token))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// credentialHashForRequest returns the hash a session should be bound to:
// the bearer token alone, or the token combined with the client certificate's
// raw DER bytes when mTLS presented one. This is the full-length hash used
// for session binding, distinct from authToken.sha8 (which is truncated and
// safe only for logging).
func credentialHashForRequest(r *http.Request, token authToken) string {
	h := sha256.New()
	h.Write(token.hash[:])
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		h.Write(r.TLS.PeerCertificates[0].Raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header value. ok is false for any other scheme or an empty header.
func bearerToken(header string) (token string, ok bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return "", false
	}
	return header[len(prefix):], true
}

// stripPort removes a trailing ":<port>" from a Host header value, if present.
func stripPort(host string) string {
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ']' { // IPv6 literal with no port, e.g. "[::1]"
			return host
		}
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

// hostAllowed reports whether host is permitted. An empty allowlist applies
// the loopback default (localhost, 127.0.0.1, ::1) — the safe posture for a
// server that refuses to bind non-loopback without an explicit allowlist.
func hostAllowed(host string, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		switch host {
		case "localhost", "127.0.0.1", "::1":
			return true
		default:
			return false
		}
	}
	_, ok := allowed[host]
	return ok
}

// originAllowed reports whether origin is permitted. An empty allowlist
// permits any Origin header — loopback mode has no non-loopback attacker to
// defend against by cross-origin request, and Host-header validation above
// already covers DNS rebinding.
func originAllowed(origin string, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[origin]
	return ok
}
