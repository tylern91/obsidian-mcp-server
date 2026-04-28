# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go MCP server for Obsidian vaults. Filesystem-based — no Obsidian app dependency. Module: `github.com/tylern91/obsidian-mcp-server`. Framework: `mark3labs/mcp-go`. Transport: stdio only. Go 1.23+.

## Build & test

```bash
make build              # ./obsidian-mcp
make test               # go test -race ./...   (race always on)
make lint               # vet + gofmt -l check — the only pre-merge gate (no CI)
make fmt                # goimports + gofmt
make run ARGS="--vault /path/to/vault"

# Single test
go test -race ./internal/vault/ -run TestSanitizePath -v
```

## Architecture

**Request flow** (`cmd/obsidian-mcp/main.go`): wires `vault.New(...)`, `search.New(vaultSvc)`, `periodic.New(...)` into a single `tools.Deps` struct, then `tools.RegisterAll(server, deps)` registers 21 mcp-go `ToolHandlerFunc` closures. Stdio transport via `mcpserver.ServeStdio(s)`.

**Key seam**: `tools.VaultService` / `tools.SearchService` / `tools.PeriodicService` interfaces live in `internal/tools/registration.go` (consumer-side), so tests can mock them. Concrete types live in `internal/vault`, `internal/search`, `internal/periodic`.

**Adding a tool**: define `registerXxx(s, deps)` + `xxxHandler(deps) server.ToolHandlerFunc` (closure factory) in a themed file under `internal/tools/`, append `registerXxx` to `RegisterAll` in `registration.go`, and add a handler alias in `export_test.go` if the handler is unexported (testing convention).

## Package layout

- `cmd/obsidian-mcp/` — entry point, stdio transport, slog JSON handler to stderr
- `internal/config/` — CLI flags > env vars (`OBSIDIAN_*`) > defaults; `--version` short-circuits via `ErrVersionRequested`
- `internal/vault/` — `vault.Service` (path security, CRUD, frontmatter, tags, links); 16 MB read/write cap
- `internal/search/` — per-query BM25 (no persistent index), regex/glob; Okapi BM25 with title boost + bigram phrase bonus
- `internal/periodic/` — daily/weekly/etc. note resolution; reads `.obsidian/plugins/periodic-notes/data.json`
- `internal/tools/` — 21 MCP tool registrations, grouped by theme (`notes.go`, `search.go`, `batch.go`, `tags.go`, …)
- `internal/response/` — single canonical `FormatJSON`; `CountTokens` (cl100k_base via tiktoken-go); rune-safe truncation
- `internal/prompts/` — MCP prompt templates
- `internal/version/` — single `const Version` (hand-edited per release; not ldflags-injected)
- `testdata/vault/` — fixture vault for tests; **load-bearing**, do not mutate

## Critical conventions

- **Stdout is reserved for JSON-RPC.** Never `fmt.Println` from server code. Logs go to stderr via slog JSON handler (`main.go`).
- **Tool errors return `mcp.NewToolResultError(...)`**, never Go errors (Go errors surface as protocol errors).
- **Path security**: every write path goes through `vault.Service.sanitizePath` → `resolveSymlink` → `checkSymlinksForWrite` *inside* `s.mu.Lock()` (TOCTOU defense). When adding a new write op, follow this exact ordering.
- **Frontmatter writes preserve key order** via yaml.v3 Node API in `internal/vault/frontmatter.go` (`UpdateFrontmatter` walks `MappingNode.Content`). Naive `yaml.Marshal` round-trip will reorder keys and corrupt user files — don't.
- **Test fixture is load-bearing**: `testdata/vault/` tag counts, link graphs, and `.obsidian/plugins/periodic-notes/data.json` back assertions in search/audit/periodic tests. Use `t.TempDir()` and copy fixtures rather than mutate.
- **Error sentinel**: `config.ErrVersionRequested` is control flow for `--version`, not a real error — `main.go` checks for it before logging.

## Operational notes

- **No CI.** `.github/` does not exist. `make lint` locally is the only gate.
- **Releases**: bump `internal/version/version.go`, tag SemVer (`vX.Y.Z`), update `CHANGELOG.md`. No goreleaser, no Dockerfile.
- **MCP integration** (Claude Code): `claude mcp add obsidian -s user -e OBSIDIAN_VAULT_PATH=/path/to/vault -- obsidian-mcp`. Claude Desktop snippet: see `README.md` § Installation.
- **`--log-level debug`** (or `OBSIDIAN_LOG_LEVEL=debug`) for verbose JSON logs to stderr. Default is `warn`.
- **Pre-built `./obsidian-mcp` at repo root** is `.gitignore`d but may exist locally and be stale — prefer `make build` before testing.
