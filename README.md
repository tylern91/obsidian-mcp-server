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
| --- | --- | --- |
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

| Flag | Env Var | Default | Valid values |
| --- | --- | --- | --- |
| `--vault` | `OBSIDIAN_VAULT_PATH` | *(required)* | Absolute or relative path to an existing directory. Validated at startup — non-existent paths or files (not dirs) cause an immediate error. Surrounding whitespace is trimmed. |
| `--extensions` | `OBSIDIAN_EXTENSIONS` | `.md,.markdown,.txt,.canvas` | Comma-separated list. Each entry should start with `.` (e.g. `.md`). Whitespace around entries is trimmed; empty entries are discarded. Only files matching one of these extensions are visible to MCP tools. |
| `--ignore` | `OBSIDIAN_IGNORE` | `.obsidian,.git,node_modules,.DS_Store,.trash` | Comma-separated list of file/directory names to skip during traversal. Match is by name (not glob). Whitespace trimmed; empties discarded. |
| `--pretty` | `OBSIDIAN_PRETTY` | `false` | CLI: bare `--pretty` enables it. Env var: any value accepted by Go's `strconv.ParseBool` — `1`, `t`, `T`, `true`, `TRUE`, `True`, `0`, `f`, `F`, `false`, `FALSE`, `False`. Anything else causes a startup error. |
| `--max-batch` | `OBSIDIAN_MAX_BATCH` | `10` | Integer ≥ `1`. Non-integer or `<1` causes a startup error. Caps the number of files processed in a single batch tool call (Phase 4). **High values increase memory usage and token count per response** — very large batches can overflow an AI client's context window and slow down individual tool calls. Keep at or near the default unless your vault files are small. |
| `--max-results` | `OBSIDIAN_MAX_RESULTS` | `20` | Integer ≥ `1`. Non-integer or `<1` causes a startup error. Caps the number of search results returned (Phase 3). **High values increase response token count** — returning hundreds of results per search can exhaust the AI client's context window with low-relevance entries. Increase only when precision-recall trade-offs require broader result sets. |
| `--log-level` | `OBSIDIAN_LOG_LEVEL` | `warn` | One of: `debug`, `info`, `warn`, `error` (lowercase, case-sensitive). Unknown values silently fall back to `warn` — no error, no warning logged. |

### Examples

```bash
# Override extensions to include Excalidraw drawings
obsidian-mcp --vault ./my-vault --extensions ".md,.canvas,.excalidraw"

# Add a custom ignore pattern alongside defaults (you must repeat the defaults
# you want to keep — values fully replace, not merge)
obsidian-mcp --vault ./my-vault \
  --ignore ".obsidian,.git,node_modules,.DS_Store,.trash,Archive,Templates"

# Enable pretty JSON via env var (any ParseBool-compatible truthy value works)
OBSIDIAN_PRETTY=1 obsidian-mcp --vault ./my-vault
OBSIDIAN_PRETTY=true obsidian-mcp --vault ./my-vault

# Verbose logging while debugging an integration
OBSIDIAN_LOG_LEVEL=debug obsidian-mcp --vault ./my-vault
```

**Precedence in action**: with `OBSIDIAN_LOG_LEVEL=debug` exported, `obsidian-mcp --vault ... --log-level info` runs at `info` — the explicit flag wins. Unset flags inherit the env var; if neither is set, the default applies.

## Security

All paths are validated through a 4-layer security model before any filesystem operation:

1. **Lexical** — rejects absolute paths, `..` traversal, and null bytes
2. **Filter** — blocks ignored patterns (`.git`, `.obsidian`, etc.) and unapproved extensions
3. **Existence** — verifies the file exists with a case-insensitive fallback; rejects ambiguous matches
4. **Symlink** — resolves symlinks and verifies the target remains inside the vault root

## Project Structure

```text
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
