# obsidian-mcp-server

A Go [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server for [Obsidian](https://obsidian.md) vaults. It gives AI agents and development tools direct filesystem access to your vault — no running Obsidian instance required.

## Features

- **Read, write, and list** notes and directories via MCP tools
- **Frontmatter** — parse and update YAML frontmatter with format-preserving rewrites
- **Tags** — extract inline `#tags`, aggregate vault-wide tag counts, add/remove tags
- **Backlinks** — on-demand reverse link graph (wikilinks and markdown links)
- **Mutations** — heading-anchored patch, safe delete, and move with confirmation guards
- **Full-text search** — BM25 Okapi ranked search with match snippets
- **Regex/glob search** — RE2 regex or filepath glob search across paths and content
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
| `get_frontmatter` | Read YAML frontmatter from a note | `path` (required), `prettyPrint` |
| `update_frontmatter` | Set or remove frontmatter keys (format-preserving) | `path` (required), `updates` (JSON object), `removeKeys` (JSON array) |
| `manage_tags` | Add or remove a tag on a note | `path`, `action`: add/remove (required), `tag` (required), `location`: frontmatter/inline |
| `list_all_tags` | Aggregate all tags across the vault with counts | `prettyPrint` |
| `get_backlinks` | Find all notes that link to a target note | `path` (required), `prettyPrint` |
| `patch_note` | Apply a heading-anchored patch to a note | `path`, `heading`, `position`: before/after/replace_body, `content` (all required) |
| `delete_note` | Permanently delete a note (requires confirm) | `path`, `confirm` (must match path exactly) |
| `move_note` | Move or rename a note within the vault (requires confirm) | `src`, `dst`, `confirm` (must match src exactly) |
| `search_notes` | BM25 full-text search with ranked results and match snippets | `query` (required), `limit`, `maxMatchesPerFile`, `caseSensitive`, `searchContent`, `searchFrontmatter`, `pathScope`, `prettyPrint` |
| `search_regex` | Search using RE2 regex or glob pattern | `pattern` (required), `isGlob`, `scope`, `limit`, `maxMatchesPerFile`, `prettyPrint` |
| `read_multiple_notes` | Read the content of multiple notes in a single request | `paths` (required, JSON array), `summary` (bool, default false), `headChars` (int, default 200) |
| `get_notes_info` | Get metadata for multiple notes without reading full content | `paths` (required, JSON array) |
| `get_vault_stats` | Get aggregate statistics about the entire vault | `includeTokenCounts` (bool, default false) |
| `get_periodic_note` | Get a periodic note (daily, weekly, monthly, quarterly, or yearly) | `granularity` (required, enum: daily/weekly/monthly/quarterly/yearly), `offset` (int, default 0), `createIfMissing` (bool, default false) |
| `get_recent_periodic_notes` | Get the N most recent periodic notes | `granularity` (required, enum: daily/weekly/monthly/quarterly/yearly), `count` (int, default 5), `summary` (bool, default true) |
| `get_recent_changes` | List notes most recently modified in the vault | `limit` (int, default 10), `since` (string, ISO-8601), `summary` (bool, default true) |
| `audit_notes` | Audit the vault for hygiene issues: orphans, dangling links, untagged notes, duplicate titles | `classes` (JSON array: orphans/dangling-links/untagged/duplicate-titles, default all), `limit` (int per class, default 20) |

### Notes

**`patch_note` semantics**: `position` controls where `content` is inserted relative to the heading:
- `before` — inserted immediately before the heading line
- `after` — inserted after the heading's body (before the next same-level or higher heading)
- `replace_body` — replaces everything between the heading line and the next same-level heading

**`search_notes` parameters**:

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `query` | string | required | Search query. Multi-term queries use OR logic; the full phrase contributes a bonus score. |
| `limit` | integer | 20 | Maximum number of results |
| `maxMatchesPerFile` | integer | 3 | Maximum match snippets per result |
| `caseSensitive` | boolean | false | Case-sensitive matching |
| `searchContent` | boolean | true | Include note body in scoring |
| `searchFrontmatter` | boolean | true | Include frontmatter values in scoring |
| `pathScope` | string | — | Glob pattern to restrict search scope (e.g. `Daily Notes/*`) |
| `prettyPrint` | boolean | false | Format JSON with indentation |

Returns: `{ query, results: [{ path, score, matchCount, matches: [{line, snippet, term}], tokenCount, reason }], total }`

**`search_regex` parameters**:

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `pattern` | string | required | RE2 regex or glob pattern |
| `isGlob` | boolean | false | Treat pattern as a filepath glob (`**` matches across dirs) |
| `scope` | string | content | `path`, `content`, or `both` |
| `limit` | integer | 20 | Maximum number of results |
| `maxMatchesPerFile` | integer | 5 | Maximum match snippets per result |
| `prettyPrint` | boolean | false | Format JSON with indentation |

Returns: `{ pattern, scope, results: [{ path, matches: [{line, snippet}] }], total }`

**Batch tools (`read_multiple_notes`, `get_notes_info`)**: The `paths` parameter is a JSON array string — e.g. `'["Notes/foo.md","Notes/bar.md"]'`. `summary:true` returns `headOf` (first N runes from `headChars`, default 200) instead of full content, which is useful for large notes to stay within context limits. Both tools enforce `--max-batch` (default 10); requests with more paths are silently truncated and the response includes `"truncated": true`.

**Periodic notes (`get_periodic_note`, `get_recent_periodic_notes`)**: Configuration (folder and date format per granularity) is read from `.obsidian/plugins/periodic-notes/data.json` inside the vault. If that file is missing, built-in defaults are used: daily notes use `YYYY-MM-DD` in `Daily Notes/`, weekly notes use `gggg-[W]ww` in `Weekly Notes/`, and so on. `offset=0` resolves to the current period, `offset=-1` to the previous period (yesterday, last week, etc.), and `offset=+1` to the next period. `createIfMissing=true` creates an empty note at the resolved path if it does not already exist.

**`get_vault_stats`**: Returns `noteCount`, `totalBytes`, `totalLinks`, `totalTags`, `topTags` (top 20 by count), `oldestNote`, `newestNote`, and `vaultRoot`. Setting `includeTokenCounts:true` runs token counting across every note — this is expensive for large vaults and is disabled by default.

**`audit_notes` classes**:
- `orphans` — notes that have no tags AND no incoming wikilinks or markdown links (completely isolated notes)
- `dangling-links` — notes containing links to vault paths that do not exist (broken references)
- `untagged` — notes with no frontmatter tags and no inline `#tags`
- `duplicate-titles` — multiple notes sharing the same filename stem, which causes wikilink ambiguity

Each class result is capped at `limit` entries (default 20). When results are truncated, the response includes `"truncated": true`.

## MCP Prompts

Prompts are server-defined conversation starters that the host (Claude Code, Claude Desktop) exposes in its UI. Each prompt pulls live vault data and constructs a ready-to-use message for the LLM.

| Prompt | Description | Arguments |
| --- | --- | --- |
| `summarize_note` | Summarize a note: 3 key bullets, entities, open questions | `path` (required) |
| `daily_note_review` | Review a daily note: carryover TODOs, link suggestions, missing tags | `offset` (int, default 0) |
| `weekly_review` | Weekly retrospective from the last 7 daily notes | `weekOffset` (int, default 0) |
| `find_related` | Suggest related notes worth linking, grouped by relationship type | `path` (required) |
| `vault_health_check` | Audit orphans, dangling links, untagged notes, duplicate titles; prioritize fixes | *(none)* |

Prompts are invoked from the host's prompt picker (e.g. `/` in Claude Code). They never modify the vault.

## MCP Resources

Resources are read-only vault data that the host can attach directly to a conversation context window — no explicit tool call required.

| Resource / Template | URI | MIME | Description |
| --- | --- | --- | --- |
| Vault statistics | `obsidian://vault/stats` | `application/json` | Note count, total size, top 10 tags, vault root |
| Tag index | `obsidian://vault/tags` | `application/json` | All tags with note counts, sorted by frequency |
| Note content | `obsidian://note/{path}` | `text/markdown` | Raw markdown (frontmatter + body) for any vault note |
| Periodic note | `obsidian://periodic/{granularity}` | `text/markdown` | Current daily / weekly / monthly / quarterly / yearly note |
| Backlinks | `obsidian://backlinks/{path}` | `application/json` | All notes linking to the target, with line numbers and snippets |

Static resources (`obsidian://vault/*`) are always available in the resource picker. Template resources are resolved when the host reads them — if the note does not exist, the resource returns an explanatory empty body instead of an error.


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

Print the version and exit:

```bash
obsidian-mcp --version
```

## Configuration

Configuration follows **CLI flag > environment variable > default** precedence.

| Flag | Env Var | Default | Valid values |
| --- | --- | --- | --- |
| `--vault` | `OBSIDIAN_VAULT_PATH` | *(required)* | Absolute or relative path to an existing directory. Validated at startup — non-existent paths or files (not dirs) cause an immediate error. Surrounding whitespace is trimmed. |
| `--version` | — | — | Prints the binary version to stdout and exits. Does not require `--vault`. |
| `--extensions` | `OBSIDIAN_EXTENSIONS` | `.md,.markdown,.txt,.canvas` | Comma-separated list. Each entry should start with `.` (e.g. `.md`). Whitespace around entries is trimmed; empty entries are discarded. Only files matching one of these extensions are visible to MCP tools. |
| `--ignore` | `OBSIDIAN_IGNORE` | `.obsidian,.git,node_modules,.DS_Store,.trash` | Comma-separated list of file/directory names to skip during traversal. Match is by name (not glob). Whitespace trimmed; empties discarded. |
| `--pretty` | `OBSIDIAN_PRETTY` | `false` | CLI: bare `--pretty` enables it. Env var: any value accepted by Go's `strconv.ParseBool` — `1`, `t`, `T`, `true`, `TRUE`, `True`, `0`, `f`, `F`, `false`, `FALSE`, `False`. Anything else causes a startup error. |
| `--max-batch` | `OBSIDIAN_MAX_BATCH` | `10` | Integer ≥ `1`. Non-integer or `<1` causes a startup error. Caps the number of files processed in a single batch tool call (Phase 4). **High values increase memory usage and token count per response** — very large batches can overflow an AI client's context window and slow down individual tool calls. Keep at or near the default unless your vault files are small. |
| `--max-results` | `OBSIDIAN_MAX_RESULTS` | `20` | Integer ≥ `1`. Non-integer or `<1` causes a startup error. Caps the number of search results returned. **High values increase response token count** — returning hundreds of results per search can exhaust the AI client's context window with low-relevance entries. Increase only when precision-recall trade-offs require broader result sets. |
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
  vault/              Path security, CRUD, frontmatter, tags, links, mutations
  tools/              MCP tool registrations and handlers
  response/           Token counting, JSON formatting
  search/             BM25 ranked search, regex/glob
  periodic/           Periodic note resolution (Phase 4)
  prompts/            MCP Prompt templates
  resources/          MCP Resource registrations
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

## License

[GPL-3.0](LICENSE)
