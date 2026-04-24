# obsidian-mcp-server

Go MCP server for Obsidian vaults. Filesystem-based, no Obsidian dependency required.

## Module

`github.com/tylern91/obsidian-mcp-server`

## Build & Test

```bash
make build    # builds ./obsidian-mcp binary
make test     # go test -race ./...
make vet      # go vet ./...
make fmt      # gofmt -w .
make run ARGS="--vault /path/to/vault"
```

## Package Layout

- `cmd/obsidian-mcp/` — entry point, stdio transport
- `internal/config/` — CLI flags > env vars > defaults
- `internal/vault/` — path security, CRUD, frontmatter, tags, links
- `internal/search/` — BM25 ranked search, regex/glob
- `internal/periodic/` — periodic note resolution
- `internal/tools/` — MCP tool registrations and handlers
- `internal/response/` — token-optimized JSON formatting
- `internal/prompts/` — MCP Prompt templates
- `testdata/vault/` — fixture vault for tests
