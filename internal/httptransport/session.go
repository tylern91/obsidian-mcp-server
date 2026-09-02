package httptransport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

const (
	sessionIDByteLen  = 32
	defaultSessionTTL = 30 * time.Minute
)

var errCredentialMismatch = errors.New("session bound to a different credential")

type credentialHashKey struct{}

// withCredentialHash stashes the caller's credential hash (computed once by
// the auth middleware) into the request context, so the SessionIdManager
// resolved for this request can bind new sessions to it and reject session
// IDs presented with a different credential.
func withCredentialHash(ctx context.Context, hash string) context.Context {
	return context.WithValue(ctx, credentialHashKey{}, hash)
}

func credentialHashFromContext(ctx context.Context) (string, bool) {
	hash, ok := ctx.Value(credentialHashKey{}).(string)
	return hash, ok
}

type sessionRecord struct {
	credentialHash string
	expiresAt      time.Time
}

// sessionStore is the server-side TTL map backing every credential-bound
// SessionIdManager handed out by sessionResolver. A single store is shared
// across all requests; each resolved manager only ever sees the credential
// hash of the request that resolved it.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]sessionRecord
	ttl      time.Duration
}

func newSessionStore(ttl time.Duration) *sessionStore {
	return &sessionStore{sessions: make(map[string]sessionRecord), ttl: ttl}
}

// sweep removes expired sessions. Call periodically from a background
// goroutine; safe to call concurrently with normal operation.
func (s *sessionStore) sweep(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rec := range s.sessions {
		if now.After(rec.expiresAt) {
			delete(s.sessions, id)
		}
	}
}

// boundSessionIdManager is a mcpserver.SessionIdManager scoped to a single
// request's credential hash. Generate binds new session IDs to that hash;
// Validate and Terminate reject any session ID bound to a different
// credential (A-HIGH-4) rather than merely checking ID shape.
type boundSessionIdManager struct {
	store          *sessionStore
	credentialHash string
}

func (m *boundSessionIdManager) Generate() string {
	raw := make([]byte, sessionIDByteLen)
	if _, err := rand.Read(raw); err != nil {
		// crypto/rand.Read failing means the OS RNG is broken — there is no
		// safe fallback for a security-sensitive ID.
		panic(fmt.Sprintf("httptransport: crypto/rand unavailable: %v", err))
	}
	id := hex.EncodeToString(raw)

	m.store.mu.Lock()
	defer m.store.mu.Unlock()
	m.store.sessions[id] = sessionRecord{
		credentialHash: m.credentialHash,
		expiresAt:      time.Now().Add(m.store.ttl),
	}
	return id
}

func (m *boundSessionIdManager) Validate(sessionID string) (isTerminated bool, err error) {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	rec, ok := m.store.sessions[sessionID]
	if !ok {
		return false, fmt.Errorf("unknown session id")
	}
	if rec.credentialHash != m.credentialHash {
		return false, errCredentialMismatch
	}
	if time.Now().After(rec.expiresAt) {
		delete(m.store.sessions, sessionID)
		return true, nil
	}
	return false, nil
}

func (m *boundSessionIdManager) Terminate(sessionID string) (isNotAllowed bool, err error) {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	rec, ok := m.store.sessions[sessionID]
	if !ok {
		return false, fmt.Errorf("unknown session id")
	}
	if rec.credentialHash != m.credentialHash {
		return true, errCredentialMismatch
	}
	delete(m.store.sessions, sessionID)
	return false, nil
}

// sessionResolver implements mcpserver.SessionIdManagerResolver by binding
// each resolved manager to the credential hash the auth middleware attached
// to the request's context.
type sessionResolver struct {
	store *sessionStore
}

func (r *sessionResolver) ResolveSessionIdManager(req *http.Request) mcpserver.SessionIdManager {
	hash, ok := credentialHashFromContext(req.Context())
	if !ok {
		// The auth middleware runs before this is ever reached in the real
		// server chain; an empty hash here can only bind to (and thus only
		// ever match) other requests that also failed to authenticate.
		hash = ""
	}
	return &boundSessionIdManager{store: r.store, credentialHash: hash}
}
