# AGENTS.md

This file provides guidance to AI coding agents (Claude Code, Codex, and others) when working
with code in this repository. It is the single source of truth — `.claude/CLAUDE.md` just points
here so Claude Code picks it up too.

## Project

Go MCP server for Obsidian vaults. Filesystem-based — no Obsidian app dependency. Module: `github.com/tylern91/obsidian-mcp-server`. Framework: `mark3labs/mcp-go`. Transport: stdio (default) or Streamable HTTP over TLS via `--transport http`. Go 1.27+.

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

**Request flow** (`cmd/obsidian-mcp/main.go`): wires `vault.New(...)`, `search.New(vaultSvc)`, `periodic.New(...)` into a single `tools.Deps` struct, then `tools.RegisterAll(server, deps)` registers 24 mcp-go `ToolHandlerFunc` closures. `cfg.Transport` then picks the transport: `stdio` (default) via `mcpserver.ServeStdio(s)`, or `http` via `httptransport.Run(ctx, s, cfg, logger)` (`internal/httptransport/server.go`) — a hand-rolled `*http.Server` with TLS, bearer auth, and Host/Origin allowlisting; never `mcp-go`'s own `StreamableHTTPServer.Start()`, which has no auth, no TLS, and an uncapped body read.

**Key seam**: `tools.VaultService` / `tools.SearchService` / `tools.PeriodicService` interfaces live in `internal/tools/registration.go` (consumer-side), so tests can mock them. Concrete types live in `internal/vault`, `internal/search`, `internal/periodic`.

**Adding a tool**: define `registerXxx(s, deps)` + `xxxHandler(deps) server.ToolHandlerFunc` (closure factory) in a themed file under `internal/tools/`, append `registerXxx` to `RegisterAll` in `registration.go`, and add a handler alias in `export_test.go` if the handler is unexported (testing convention).

## Package layout

- `cmd/obsidian-mcp/` — entry point, transport selection (stdio/http), slog JSON handler to stderr
- `internal/config/` — CLI flags > env vars (`OBSIDIAN_*`) > defaults; `--version` short-circuits via `ErrVersionRequested`
- `internal/vault/` — `vault.Service` (path security, CRUD, frontmatter, tags, links); 16 MB read/write cap
- `internal/search/` — per-query BM25 (no persistent index), regex/glob; Okapi BM25 with title boost + bigram phrase bonus
- `internal/periodic/` — daily/weekly/etc. note resolution; reads `.obsidian/plugins/periodic-notes/data.json`
- `internal/tools/` — 24 MCP tool registrations, grouped by theme (`notes.go`, `search.go`, `batch.go`, `tags.go`, `renametag.go`, `replace.go`, …)
- `internal/response/` — single canonical `FormatJSON`; `CountTokens` (cl100k_base via tiktoken-go); rune-safe truncation
- `internal/prompts/` — MCP prompt templates
- `internal/version/` — single `const Version` (hand-edited per release; not ldflags-injected)
- `internal/httptransport/` — Streamable HTTP transport: security middleware (Host/Origin allowlist → body cap → auth), TLS cert generation, bearer token generation, custom `SessionIdManager`
- `testdata/vault/` — fixture vault for tests; **load-bearing**, do not mutate

## Critical conventions

- **Stdout is reserved for JSON-RPC.** Never `fmt.Println` from server code. Logs go to stderr via slog JSON handler (`main.go`).
- **Tool errors return `mcp.NewToolResultError(...)`**, never Go errors (Go errors surface as protocol errors).
- **Path security**: `sanitizePath` is purely lexical (no syscalls) and may run *outside* `s.mu.Lock()`; `checkSymlinksForWrite` and the write syscall itself must both run *inside* the lock — that ordering, not `sanitizePath`'s position, is the TOCTOU defense (against races within this process; a local attacker with vault write access can still race the check against an external write — see `checkSymlinksForWrite`'s doc comment and `SECURITY.md`). When adding a new write op, follow this exact ordering. Read paths use case-insensitive fallback resolution (`ResolvePath` → `existenceCheck`); write paths use `existsStrict` — never mix the two, since a case-insensitive fallback on a write target would resolve ambiguously.
- **D-4 (locked, 2026-08-27): read/write path resolution asymmetry is deliberate, not an inconsistency.** Reads resolve case-insensitively (`ResolvePath` → `existenceCheck`, returning `ErrAmbiguousPath` on a case collision) because a read only needs to find *a* matching file. Writes use `existsStrict` and never fall back case-insensitively, because a write needs certainty about exactly which file is being mutated — a case-insensitive match on a write target could silently overwrite the wrong one of two similarly-named files. Do not "fix" this into symmetry; it is the correct behavior for each operation's risk profile. No `CHANGELOG.md` entry — nothing observable changes.
- **Frontmatter writes preserve key order** via yaml.v3 Node API in `internal/vault/frontmatter.go` (`UpdateFrontmatter` walks `MappingNode.Content`). Naive `yaml.Marshal` round-trip will reorder keys and corrupt user files — don't.
- **Test fixture is load-bearing**: `testdata/vault/` tag counts, link graphs, and `.obsidian/plugins/periodic-notes/data.json` back assertions in search/audit/periodic tests. Use `t.TempDir()` and copy fixtures rather than mutate.
- **Error sentinel**: `config.ErrVersionRequested` is control flow for `--version`, not a real error — `main.go` checks for it before logging.
- **Optimistic concurrency (etags)**: `vault.Etag(data []byte) string` (`internal/vault/etag.go`) is the single canonical SHA-256 function — every etag emitted by `read_note`/`read_multiple_notes`/`get_notes_info` and every `if_match` comparison must go through it, never a second hash implementation. Mutating handlers (`write_note`, `patch_note`, `update_frontmatter`, `manage_tags`, `delete_note`, `move_note`) accept an optional `if_match`; a mismatch returns a `REVISION_CONFLICT`-prefixed `mcp.NewToolResultError`, never a raw Go error. The compare (`checkIfMatch`) runs inside `s.mu.Lock()`, immediately after `checkSymlinksForWrite` — a decorator or tools-layer check would race the write it guards, since every vault read path is lock-free. Two documented edge cases: `if_match` against a note that doesn't exist yet is a conflict, not an implicit create; `move_note`'s `dryRun: true` path does not enforce `if_match` (it's a lock-free `os.Stat`-only preview).
- **HTTP transport (`internal/httptransport/`)**: never call `mcp-go`'s `StreamableHTTPServer.Start()` — own the `*http.Server` directly. Security middleware order is fixed: Host/Origin allowlist → `http.MaxBytesReader` body cap → bearer/mTLS auth, all evaluated before any handler logic. TLS defaults on, `MinVersion: tls.VersionTLS13`; the self-signed cert, key, and bearer token are generated on first run into the OS user config dir (`os.UserConfigDir()/obsidian-mcp`), never under the vault path. Compare the bearer token by hashing both sides to a fixed width before `subtle.ConstantTimeCompare` — never a raw `==`, which leaks length via early return. The custom `SessionIdManager` binds each session to its issuing credential (`sha256(token)`, or client cert under mTLS) and rejects a session ID presented with a different one. `--allow-non-loopback` requires non-empty `--allowed-hosts` and `--allowed-origins` — never relax this to a single flag. See `SECURITY.md` § HTTP transport for the full posture.

## Operational notes

- **CI**: `.github/workflows/go.yml` runs `make lint`, `make test`, `make build`, and `govulncheck ./...` on every PR. `.github/workflows/release.yml` handles tag-triggered releases. `make lint` locally reproduces the same gate before pushing.
- **Releases**: bump `internal/version/version.go`, tag SemVer (`vX.Y.Z`), update `CHANGELOG.md`. No goreleaser, no Dockerfile.
- **MCP integration**: `claude mcp add obsidian -s user -e OBSIDIAN_VAULT_PATH=/path/to/vault -- obsidian-mcp` (Claude Code) or `codex mcp add obsidian -s user -e OBSIDIAN_VAULT_PATH=/path/to/vault -- obsidian-mcp` (Codex). See `README.md` § Installation for Claude Desktop and other clients.
- **`--log-level debug`** (or `OBSIDIAN_LOG_LEVEL=debug`) for verbose JSON logs to stderr. Default is `warn`.
- **`--transport http`** starts the Streamable HTTP listener (default `stdio`, unchanged). New flags: `--http-bind` (default `127.0.0.1`), `--http-port` (default `8443`), `--allow-non-loopback`, `--allowed-hosts`, `--allowed-origins`, `--client-ca`; matching `OBSIDIAN_*` env vars. See `SECURITY.md` § HTTP transport and `README.md` § Configuration.
- **Pre-built `./obsidian-mcp` at repo root** is `.gitignore`d but may exist locally and be stale — prefer `make build` before testing.
