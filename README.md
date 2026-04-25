# obsidian-mcp-server

A Go [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server for [Obsidian](https://obsidian.md) vaults. It gives AI agents and development tools direct filesystem access to your vault — no running Obsidian instance required.

## Features

- **Read, write, and list** notes and directories via MCP tools
- **Path security** — 4-layer validation: lexical checks, ignore/extension filters, case-insensitive existence lookup, and symlink escape prevention
- **Stdio transport** — works with any MCP client (Claude Code, Claude Desktop, etc.)
- **Zero Obsidian dependency** — operates on the vault directory directly
- **Token counting** — responses include approximate token counts (cl100k_base)

## MCP Tools

| Tool | Description | Params |
|------|-------------|--------|
| `read_note` | Read a note's content and metadata | `path` (required), `prettyPrint` |
| `write_note` | Create or update a note | `path`, `content` (required), `mode`: overwrite/append/prepend |
| `list_directory` | List files and subdirectories | `path` (empty = vault root), `prettyPrint` |

## Installation

Requires Go 1.23+.

```bash
go install github.com/tylern91/obsidian-mcp-server/cmd/obsidian-mcp@latest
```

Or build from source:

```bash
git clone https://github.com/tylern91/obsidian-mcp-server.git
cd obsidian-mcp-server
make build
```

## Usage

### Claude Code

```bash
claude mcp add obsidian -s user \
  -e OBSIDIAN_VAULT_PATH="/path/to/your/vault" \
  -- obsidian-mcp
```

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "obsidian": {
      "command": "obsidian-mcp",
      "args": ["--vault", "/path/to/your/vault"],
      "env": {}
    }
  }
}
```

### Direct

```bash
obsidian-mcp --vault /path/to/your/vault
```

## Configuration

Configuration follows **CLI flag > environment variable > default** precedence.

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--vault` | `OBSIDIAN_VAULT_PATH` | *(required)* | Path to Obsidian vault directory |
| `--extensions` | `OBSIDIAN_EXTENSIONS` | `.md,.markdown,.txt,.canvas` | Comma-separated allowed file extensions |
| `--ignore` | `OBSIDIAN_IGNORE` | `.obsidian,.git,node_modules,.DS_Store,.trash` | Comma-separated ignore patterns |
| `--pretty` | `OBSIDIAN_PRETTY` | `false` | Pretty-print JSON responses |
| `--max-batch` | `OBSIDIAN_MAX_BATCH` | `10` | Max files per batch operation |
| `--max-results` | `OBSIDIAN_MAX_RESULTS` | `20` | Max search results |
| `--log-level` | `OBSIDIAN_LOG_LEVEL` | `warn` | Log level: debug, info, warn, error |

## Security

All paths are validated through a 4-layer security model before any filesystem operation:

1. **Lexical** — rejects absolute paths, `..` traversal, and null bytes
2. **Filter** — blocks ignored patterns (`.git`, `.obsidian`, etc.) and unapproved extensions
3. **Existence** — verifies the file exists with a case-insensitive fallback; rejects ambiguous matches
4. **Symlink** — resolves symlinks and verifies the target remains inside the vault root

## Project Structure

```
cmd/obsidian-mcp/     Entry point, stdio transport
internal/
  config/             CLI flags, env vars, defaults
  vault/              Path security, CRUD operations
  tools/              MCP tool registrations and handlers
  response/           Token counting, JSON formatting
  search/             BM25 ranked search (planned)
  periodic/           Periodic note resolution (planned)
  prompts/            MCP Prompt templates (planned)
testdata/vault/       Fixture vault for tests
```

## Development

```bash
make build    # compile binary
make test     # go test -race ./...
make vet      # go vet ./...
make fmt      # gofmt + goimports
make run ARGS="--vault /path/to/vault"
make help     # list all targets
```

## Roadmap

- **Phase 2** — Frontmatter parsing, tag management, backlinks, patch/delete/move notes
- **Phase 3** — BM25 full-text search, regex search
- **Phase 4** — Batch operations, vault stats, periodic notes, recent changes
- **Phase 5** — MCP Prompts, Resources, and release packaging

## License

[GPL-3.0](LICENSE)
