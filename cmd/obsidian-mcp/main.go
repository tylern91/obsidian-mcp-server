package main

import (
	"fmt"
	"log/slog"
	"os"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/config"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "obsidian-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}

	// Structured logging to stderr — stdout carries JSON-RPC traffic.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.SlogLevel()}))
	slog.SetDefault(logger)

	slog.Info("starting obsidian-mcp server",
		"vault", cfg.VaultPath,
		"extensions", cfg.Extensions,
		"maxResults", cfg.MaxResults,
	)

	filter := vault.NewPathFilter(cfg.IgnorePatterns, cfg.Extensions)
	vaultSvc := vault.New(cfg.VaultPath, filter)
	searchSvc := search.New(vaultSvc)

	s := mcpserver.NewMCPServer(
		"obsidian-mcp",
		"0.1.0",
		mcpserver.WithToolCapabilities(true),
	)

	tools.RegisterAll(s, tools.Deps{
		Vault:       vaultSvc,
		Search:      searchSvc,
		PrettyPrint: cfg.PrettyPrint,
		MaxBatch:    cfg.MaxBatch,
		MaxResults:  cfg.MaxResults,
	})

	slog.Info("tools registered, serving stdio")
	return mcpserver.ServeStdio(s)
}
