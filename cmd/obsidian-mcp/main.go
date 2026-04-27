package main

import (
	"fmt"
	"log/slog"
	"os"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/config"
	"github.com/tylern91/obsidian-mcp-server/internal/periodic"
	"github.com/tylern91/obsidian-mcp-server/internal/prompts"
	"github.com/tylern91/obsidian-mcp-server/internal/resources"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

const version = "1.0.0"

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
		"version", version,
	)

	filter := vault.NewPathFilter(cfg.IgnorePatterns, cfg.Extensions)
	vaultSvc := vault.New(cfg.VaultPath, filter)
	searchSvc := search.New(vaultSvc)
	periodicSvc := periodic.New(cfg.VaultPath)

	s := mcpserver.NewMCPServer(
		"obsidian-mcp",
		version,
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithPromptCapabilities(true),
		mcpserver.WithResourceCapabilities(false, true),
	)

	tools.RegisterAll(s, tools.Deps{
		Vault:       vaultSvc,
		Search:      searchSvc,
		Periodic:    periodicSvc,
		PrettyPrint: cfg.PrettyPrint,
		MaxBatch:    cfg.MaxBatch,
		MaxResults:  cfg.MaxResults,
	})
	prompts.RegisterAll(s, prompts.Deps{
		Vault:    vaultSvc,
		Periodic: periodicSvc,
	})
	resources.RegisterAll(s, resources.Deps{
		Vault:       vaultSvc,
		Periodic:    periodicSvc,
		PrettyPrint: cfg.PrettyPrint,
	})

	slog.Info("registered capabilities", "tools", 20, "prompts", 5, "resources", 5)
	return mcpserver.ServeStdio(s)
}
