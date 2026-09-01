package httptransport

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const (
	tokenFileName = "token"
	tokenByteLen  = 32
	envAuthToken  = "OBSIDIAN_AUTH_TOKEN"
)

// authToken holds the bearer token and its hash for comparison and logging.
// The raw token is never logged; only sha8 (the first 8 hex chars of its
// SHA-256) is safe to print.
type authToken struct {
	hash [sha256.Size]byte
	sha8 string
	path string // empty when sourced from the environment
}

// loadOrCreateToken resolves the bearer token in priority order: the
// OBSIDIAN_AUTH_TOKEN environment variable, then an existing 0600 token file
// in the state dir, then a freshly generated 32-byte token written to that
// file. There is deliberately no CLI flag for the token value — argv is
// world-readable via /proc/<pid>/cmdline and `ps`.
func loadOrCreateToken(dir string) (authToken, error) {
	if env := os.Getenv(envAuthToken); env != "" {
		return authToken{hash: sha256.Sum256([]byte(env)), sha8: sha8Hex(env)}, nil
	}

	path := filepath.Join(dir, tokenFileName)
	if data, err := os.ReadFile(path); err == nil {
		token := string(data)
		return authToken{hash: sha256.Sum256([]byte(token)), sha8: sha8Hex(token), path: path}, nil
	} else if !os.IsNotExist(err) {
		return authToken{}, fmt.Errorf("read token file %s: %w", path, err)
	}

	raw := make([]byte, tokenByteLen)
	if _, err := rand.Read(raw); err != nil {
		return authToken{}, fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return authToken{}, fmt.Errorf("write token file %s: %w", path, err)
	}
	return authToken{hash: sha256.Sum256([]byte(token)), sha8: sha8Hex(token), path: path}, nil
}

func sha8Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

// compare reports whether presented matches the configured token. Both sides
// are hashed to a fixed 32 bytes before subtle.ConstantTimeCompare — comparing
// raw strings of unequal length returns early and leaks length via timing.
func (t authToken) compare(presented string) bool {
	presentedHash := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(t.hash[:], presentedHash[:]) == 1
}
