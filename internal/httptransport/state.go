// Package httptransport implements the Streamable HTTP transport: a security
// middleware chain (Host/Origin allowlist, body-size cap, bearer/mTLS auth),
// self-signed TLS certificate management, and a credential-bound
// SessionIdManager. mcp-go's own StreamableHTTPServer.Start is never called —
// this package owns the *http.Server so it can set TLS, timeouts, and the
// middleware chain that Start does not provide.
package httptransport

import (
	"fmt"
	"os"
	"path/filepath"
)

// stateDir returns the directory used to store the auto-generated TLS
// certificate, key, and bearer token. It is always outside the vault path —
// a secret stored under the vault would be exfiltratable through the very
// read/search API it protects.
func stateDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	dir := filepath.Join(base, "obsidian-mcp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir %s: %w", dir, err)
	}
	return dir, nil
}
