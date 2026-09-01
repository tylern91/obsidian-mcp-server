package httptransport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/tylern91/obsidian-mcp-server/internal/config"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 5 * time.Minute
	maxHeaderBytes    = 1 << 20 // 1 MiB
	shutdownTimeout   = 10 * time.Second
	sessionSweepEvery = time.Minute
)

// Run builds and serves the Streamable HTTP transport until ctx is canceled,
// then gracefully shuts down. mcpServer must already have every tool,
// resource, and prompt registered — this package only adds the transport.
//
// mcp-go's own StreamableHTTPServer.Start is never called: it builds a
// timeout-free plaintext server with no Host/Origin validation and an
// uncapped body read, none of which is acceptable for a listener that isn't
// stdio. This function owns the *http.Server instead, so it can set TLS,
// timeouts, and the security middleware chain that Start does not provide.
func Run(ctx context.Context, mcpServer *mcpserver.MCPServer, cfg *config.Config, logger *slog.Logger) error {
	if !cfg.AllowNonLoopback && !isLoopbackHost(cfg.HTTPBind) {
		return fmt.Errorf("refusing to bind non-loopback address %q without --allow-non-loopback", cfg.HTTPBind)
	}

	dir, err := stateDir()
	if err != nil {
		return err
	}

	token, err := loadOrCreateToken(dir)
	if err != nil {
		return fmt.Errorf("load auth token: %w", err)
	}
	if token.path != "" {
		logger.Info("bearer token ready", "path", token.path, "sha8", token.sha8)
	} else {
		logger.Info("bearer token loaded from environment", "sha8", token.sha8)
	}

	cert, err := loadOrCreateCert(dir)
	if err != nil {
		return fmt.Errorf("load TLS certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
	}

	if cfg.ClientCAPath != "" {
		pool, err := loadClientCAPool(cfg.ClientCAPath)
		if err != nil {
			return fmt.Errorf("load client CA pool: %w", err)
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	store := newSessionStore(defaultSessionTTL)
	sweepCtx, stopSweep := context.WithCancel(ctx)
	defer stopSweep()
	go runSessionSweep(sweepCtx, store)

	chain := &chainConfig{
		allowedHosts:   toSet(cfg.AllowedHosts),
		allowedOrigins: toSet(cfg.AllowedOrigins),
		token:          token,
		logger:         logger,
	}

	streamable := mcpserver.NewStreamableHTTPServer(
		mcpServer,
		mcpserver.WithSessionIdManagerResolver(&sessionResolver{store: store}),
	)

	addr := net.JoinHostPort(cfg.HTTPBind, fmt.Sprintf("%d", cfg.HTTPPort))
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           chain.buildChain(streamable),
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		WriteTimeout:      0, // GET is a long-lived SSE stream; bound writes per-response instead
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("streamable HTTP listener starting", "addr", addr, "mtls", cfg.ClientCAPath != "")
		// certFile/keyFile empty: the certificate is already set on tlsConfig.
		if err := httpServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown streamable HTTP server: %w", err)
		}
		return nil
	}
}

func runSessionSweep(ctx context.Context, store *sessionStore) {
	ticker := time.NewTicker(sessionSweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			store.sweep(now)
		}
	}
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}

func isLoopbackHost(host string) bool {
	switch host {
	case "localhost":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
