// Package acceptance_test holds [AC-N]-named acceptance tests, one per acceptance criterion.
// See AGENTS.md § Critical conventions for the ATDD convention this package follows.
package acceptance_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// startClient wires register's registrations (e.g. prompts.RegisterAll,
// resources.RegisterAll) onto a real *server.MCPServer, serves it over an
// in-process stdio pipe, and returns an initialized client connected to it.
// This exercises the real MCP dispatch path (URI template matching, argument
// parsing) rather than calling unexported handler funcs directly.
func startClient(t *testing.T, register func(*server.MCPServer)) *client.Client {
	t.Helper()
	ctx := t.Context()

	mcpServer := server.NewMCPServer(t.Name(), "0.0.0-test")
	register(mcpServer)

	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	stdioServer := server.NewStdioServer(mcpServer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = stdioServer.Listen(ctx, serverReader, serverWriter)
	}()

	tr := transport.NewIO(clientReader, clientWriter, io.NopCloser(strings.NewReader("")))
	c := client.NewClient(tr)
	require.NoError(t, c.Start(ctx))

	var initReq mcp.InitializeRequest
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	_, err := c.Initialize(ctx, initReq)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = tr.Close()
		_ = clientWriter.Close()
		_ = serverWriter.Close()
		<-done
	})

	return c
}

// newVaultDeps copies the committed testdata/vault fixture into a fresh t.TempDir()
// and returns a tools.Deps wired to it. Never mutate testdata/vault directly.
func newVaultDeps(t *testing.T) tools.Deps {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir("../../testdata/vault", dir); err != nil {
		t.Fatalf("copy fixture vault: %v", err)
	}
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return tools.Deps{
		Vault:       vault.New(dir, filter),
		PrettyPrint: false,
	}
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// TestNewVaultDepsScaffold exercises newVaultDeps (and, transitively, copyDir
// and copyFile) so these shared fixture helpers stay linked into the package
// even on a commit where no other acceptance test yet calls them directly.
func TestNewVaultDepsScaffold(t *testing.T) {
	deps := newVaultDeps(t)
	if deps.Vault == nil {
		t.Fatal("newVaultDeps returned a nil Vault")
	}
}
