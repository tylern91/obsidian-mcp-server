package httptransport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBoundSessionIdManager_GenerateAndValidate(t *testing.T) {
	store := newSessionStore(time.Minute)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}

	id := mgr.Generate()
	if id == "" {
		t.Fatal("Generate returned an empty session ID")
	}

	isTerminated, err := mgr.Validate(id)
	if err != nil {
		t.Fatalf("Validate(own session) error = %v, want nil", err)
	}
	if isTerminated {
		t.Error("Validate(fresh session) isTerminated = true, want false")
	}
}

func TestBoundSessionIdManager_RejectsCredentialMismatch(t *testing.T) {
	store := newSessionStore(time.Minute)
	owner := &boundSessionIdManager{store: store, credentialHash: "cred-owner"}
	attacker := &boundSessionIdManager{store: store, credentialHash: "cred-attacker"}

	id := owner.Generate()

	if _, err := attacker.Validate(id); err != errCredentialMismatch {
		t.Errorf("Validate(session bound to different credential) error = %v, want errCredentialMismatch", err)
	}

	isNotAllowed, err := attacker.Terminate(id)
	if err != errCredentialMismatch {
		t.Errorf("Terminate(session bound to different credential) error = %v, want errCredentialMismatch", err)
	}
	if !isNotAllowed {
		t.Error("Terminate(mismatched credential) isNotAllowed = false, want true")
	}

	// The session must still be valid for its actual owner — a mismatched
	// Terminate attempt must not have deleted it.
	if _, err := owner.Validate(id); err != nil {
		t.Errorf("owner Validate after attacker's failed Terminate: %v, want nil", err)
	}
}

func TestBoundSessionIdManager_UnknownSessionID(t *testing.T) {
	store := newSessionStore(time.Minute)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}

	if _, err := mgr.Validate("does-not-exist"); err == nil {
		t.Error("Validate(unknown id) error = nil, want non-nil")
	}
	if _, err := mgr.Terminate("does-not-exist"); err == nil {
		t.Error("Terminate(unknown id) error = nil, want non-nil")
	}
}

func TestBoundSessionIdManager_Terminate(t *testing.T) {
	store := newSessionStore(time.Minute)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}

	id := mgr.Generate()
	if isNotAllowed, err := mgr.Terminate(id); err != nil || isNotAllowed {
		t.Fatalf("Terminate(own session) = (%v, %v), want (false, nil)", isNotAllowed, err)
	}

	if _, err := mgr.Validate(id); err == nil {
		t.Error("Validate after Terminate: error = nil, want non-nil (session should be gone)")
	}
}

func TestSessionStore_SweepRemovesExpired(t *testing.T) {
	store := newSessionStore(time.Millisecond)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}

	id := mgr.Generate()
	time.Sleep(5 * time.Millisecond)

	store.sweep(time.Now())

	store.mu.Lock()
	_, exists := store.sessions[id]
	store.mu.Unlock()
	if exists {
		t.Error("sweep did not remove an expired session")
	}
}

func TestSessionStore_ValidateReportsTerminatedOnExpiry(t *testing.T) {
	store := newSessionStore(time.Millisecond)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}

	id := mgr.Generate()
	time.Sleep(5 * time.Millisecond)

	isTerminated, err := mgr.Validate(id)
	if err != nil {
		t.Fatalf("Validate(expired session) error = %v, want nil", err)
	}
	if !isTerminated {
		t.Error("Validate(expired session) isTerminated = false, want true")
	}
}

func TestSessionStore_ValidateSlidesExpiryWhileActive(t *testing.T) {
	const ttl = 500 * time.Millisecond
	store := newSessionStore(ttl)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}

	id := mgr.Generate()
	for i := 0; i < 15; i++ {
		time.Sleep(50 * time.Millisecond)
		isTerminated, err := mgr.Validate(id)
		if err != nil || isTerminated {
			t.Fatalf("Validate at %dms = (%v, %v), want (false, nil): active session must not expire", (i+1)*50, isTerminated, err)
		}
	}

	time.Sleep(ttl + 100*time.Millisecond)
	isTerminated, err := mgr.Validate(id)
	if err != nil || !isTerminated {
		t.Errorf("Validate after idling = (%v, %v), want (true, nil)", isTerminated, err)
	}
}

func TestSessionResolver_BindsToContextCredentialHash(t *testing.T) {
	store := newSessionStore(time.Minute)
	resolver := &sessionResolver{store: store}

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req = req.WithContext(withCredentialHash(req.Context(), "cred-from-context"))

	mgr, ok := resolver.ResolveSessionIdManager(req).(*boundSessionIdManager)
	if !ok {
		t.Fatalf("ResolveSessionIdManager returned %T, want *boundSessionIdManager", resolver.ResolveSessionIdManager(req))
	}
	if mgr.credentialHash != "cred-from-context" {
		t.Errorf("resolved manager credentialHash = %q, want %q", mgr.credentialHash, "cred-from-context")
	}
}

func TestSessionResolver_MissingCredentialHashBindsToEmpty(t *testing.T) {
	store := newSessionStore(time.Minute)
	resolver := &sessionResolver{store: store}

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)

	mgr, ok := resolver.ResolveSessionIdManager(req).(*boundSessionIdManager)
	if !ok {
		t.Fatalf("ResolveSessionIdManager returned %T, want *boundSessionIdManager", resolver.ResolveSessionIdManager(req))
	}
	if mgr.credentialHash != "" {
		t.Errorf("resolved manager credentialHash = %q, want empty", mgr.credentialHash)
	}
}

// fakeClock is an injectable clock so lifetime tests never sleep.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newClockedStore(ttl, maxLifetime time.Duration) (*sessionStore, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	store := newSessionStore(ttl)
	store.now = clock.Now
	store.maxLifetime = maxLifetime
	return store, clock
}

func TestSessionStore_MaxLifetimeExpiresActiveSession(t *testing.T) {
	store, clock := newClockedStore(10*time.Minute, time.Hour)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}
	id := mgr.Generate()

	// Activity every 5 minutes keeps the sliding TTL alive well past the cap.
	for i := 0; i < 11; i++ {
		clock.Advance(5 * time.Minute)
		if terminated, err := mgr.Validate(id); err != nil || terminated {
			t.Fatalf("Validate at %dm = (%v, %v), want (false, nil)", (i+1)*5, terminated, err)
		}
	}

	clock.Advance(6 * time.Minute) // 61 minutes since creation
	if terminated, err := mgr.Validate(id); err != nil || !terminated {
		t.Errorf("Validate past max lifetime = (%v, %v), want (true, nil)", terminated, err)
	}
	if _, err := mgr.Validate(id); err == nil {
		t.Error("expired session must be removed: second Validate should report an unknown id")
	}
}

func TestSessionStore_MaxLifetimeZeroDisablesCap(t *testing.T) {
	store, clock := newClockedStore(10*time.Minute, 0)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}
	id := mgr.Generate()

	for i := 0; i < 100; i++ {
		clock.Advance(5 * time.Minute)
		if terminated, err := mgr.Validate(id); err != nil || terminated {
			t.Fatalf("Validate at iteration %d = (%v, %v), want (false, nil) with the cap disabled", i, terminated, err)
		}
	}
}

func TestSessionStore_SweepRemovesSessionsPastMaxLifetime(t *testing.T) {
	store, clock := newClockedStore(10*time.Minute, time.Hour)
	mgr := &boundSessionIdManager{store: store, credentialHash: "cred-a"}
	id := mgr.Generate()

	for i := 0; i < 11; i++ {
		clock.Advance(5 * time.Minute)
		if _, err := mgr.Validate(id); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	}
	clock.Advance(6 * time.Minute)
	store.sweep(clock.Now())

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.sessions) != 0 {
		t.Errorf("sweep left %d sessions, want 0 past the max lifetime", len(store.sessions))
	}
}
