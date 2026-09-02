# obsidian-mcp-server

[![CI](https://github.com/tylern91/obsidian-mcp-server/actions/workflows/go.yml/badge.svg)](https://github.com/tylern91/obsidian-mcp-server/actions/workflows/go.yml)
[![Release](https://github.com/tylern91/obsidian-mcp-server/actions/workflows/release.yml/badge.svg)](https://github.com/tylern91/obsidian-mcp-server/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/tylern91/obsidian-mcp-server)](https://goreportcard.com/report/github.com/tylern91/obsidian-mcp-server)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/tylern91/obsidian-mcp-server)](go.mod)

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
- **Streamable HTTP transport** (optional) — TLS 1.3 + bearer token auth by default, loopback-only unless explicitly widened; see `--transport http` in Configuration and `SECURITY.md` § HTTP transport
- **Zero Obsidian dependency** — operates on the vault directory directly
- **Token counting** — responses include approximate token counts (cl100k_base)

## MCP Tools

| Tool | Description | Params |
| --- | --- | --- |
| `read_note` | Read a note's content and metadata | `path` (required), `prettyPrint` — response includes an `etag` |
| `write_note` | Create or update a note | `path`, `content` (required), `mode`: overwrite/append/prepend, `if_match` (optional etag) |
| `list_directory` | List files and subdirectories | `path` (empty = vault root), `prettyPrint` |
| `get_frontmatter` | Read YAML frontmatter from a note | `path` (required), `prettyPrint` |
| `update_frontmatter` | Set or remove frontmatter keys (format-preserving) | `path` (required), `updates` (JSON object), `removeKeys` (JSON array), `if_match` (optional etag) |
| `manage_tags` | Add or remove a tag on a note | `path`, `action`: add/remove (required), `tag` (required), `location`: frontmatter/inline, `if_match` (optional etag) |
| `list_all_tags` | Aggregate all tags across the vault with counts | `prettyPrint` |
| `get_backlinks` | Find all notes that link to a target note | `path` (required), `prettyPrint` |
| `patch_note` | Apply a heading-anchored patch to a note | `path`, `heading`, `position`: before/after/replace_body, `content` (all required), `if_match` (optional etag) |
| `delete_note` | Move a note to `.obsidian-mcp/trash` (requires confirm); pass `permanent: true` to hard-delete instead | `path`, `confirm` (must match path exactly), `permanent` (optional, default false), `if_match` (optional etag) |
| `move_note` | Move or rename a note within the vault (requires confirm); rewrites unambiguous inbound links by default | `src`, `dst`, `confirm` (must match src exactly), `updateLinks` (bool, default true), `dryRun` (bool, default false), `if_match` (optional etag; ignored when `dryRun` is true) |
| `search_notes` | BM25 full-text search with ranked results and match snippets | `query` (required), `limit`, `maxMatchesPerFile`, `caseSensitive`, `searchContent`, `searchFrontmatter`, `pathScope`, `prettyPrint` |
| `search_regex` | Search using RE2 regex or glob pattern | `pattern` (required), `isGlob`, `scope`, `limit`, `maxMatchesPerFile`, `prettyPrint` |
| `read_multiple_notes` | Read the content of multiple notes in a single request | `paths` (required, JSON array), `summary` (bool, default false), `headChars` (int, default 200) — each entry includes an `etag` |
| `get_notes_info` | Get metadata for multiple notes without reading full content | `paths` (required, JSON array) — each entry includes an `etag` |
| `get_vault_stats` | Get aggregate statistics about the entire vault | `includeTokenCounts` (bool, default false) |
| `get_periodic_note` | Get a periodic note (daily, weekly, monthly, quarterly, or yearly) | `granularity` (required, enum: daily/weekly/monthly/quarterly/yearly), `offset` (int, default 0), `createIfMissing` (bool, default false) |
| `get_recent_periodic_notes` | Get the N most recent periodic notes | `granularity` (required, enum: daily/weekly/monthly/quarterly/yearly), `count` (int, default 5), `summary` (bool, default true) |
| `get_recent_changes` | List notes most recently modified in the vault | `limit` (int, default 10), `since` (string, ISO-8601), `summary` (bool, default true) |
| `audit_notes` | Audit the vault for hygiene issues: orphans, dangling links, untagged notes, duplicate titles | `classes` (JSON array: orphans/dangling-links/untagged/duplicate-titles, default all), `limit` (int per class, default 20) |
| `get_note_outline` | Return a note's heading tree (level, text, line number) without its body | `path` (required), `prettyPrint` |
| `read_note_lines` | Read a bounded range of lines from a note | `path`, `startLine` (required), `lineCount` (int, default 200, capped at 2000) |
| `rename_tag` | Rename a tag vault-wide, across frontmatter and inline occurrences | `oldTag`, `newTag` (both required) |
| `replace_in_note` | Scoped search-and-replace within a single note, literal or regex | `path`, `pattern`, `replacement` (all required), `isRegex` (bool, default false), `maxOccurrences` (int, default 0 = unbounded) |

`search_notes`, `search_regex`, `list_directory`, and `get_recent_changes` results also include an
`obsidian://open` `deepLink` field per note, built from `--vault-name` (see Configuration below).

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

**Optimistic concurrency (`if_match` / `etag`)**: `read_note`, `read_multiple_notes`, and `get_notes_info` return a SHA-256 `etag` of the note's content. Pass that value as `if_match` on `write_note`, `patch_note`, `update_frontmatter`, `manage_tags`, `delete_note`, or `move_note` to make the write conditional — if the note has changed since you read it, the call fails with a `REVISION_CONFLICT` error instead of silently overwriting someone else's edit. `if_match` is optional everywhere; omitting it writes unconditionally, as before. Two edge cases: passing `if_match` for a note that doesn't exist yet is always a conflict (it never creates the note), and `move_note`'s `dryRun:true` preview does not enforce `if_match`.

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

### Install script

Downloads the right binary for your OS/architecture, verifies its checksum, and installs it to
`~/.local/bin` (override with a second argument):

```bash
curl -fsSL https://raw.githubusercontent.com/tylern91/obsidian-mcp-server/main/install.sh | sh
```

Pin a version instead of `latest`:

```bash
curl -fsSL https://raw.githubusercontent.com/tylern91/obsidian-mcp-server/main/install.sh | sh -s -- v0.2.0
```

### Claude Desktop (.mcpb bundle)

Download `obsidian-mcp-<version>.mcpb` from the [latest release](https://github.com/tylern91/obsidian-mcp-server/releases/latest) and double-click it — Claude Desktop installs the extension and prompts for your vault path. No manual JSON editing.

### Release binary

Download a prebuilt binary from the [latest release](https://github.com/tylern91/obsidian-mcp-server/releases/latest), verify its checksum, and install:

```bash
# macOS (Apple Silicon) — swap the asset name for your platform (darwin-amd64, linux-amd64, linux-arm64)
curl -fLO https://github.com/tylern91/obsidian-mcp-server/releases/latest/download/obsidian-mcp-<version>-darwin-arm64.tar.gz
curl -fLO https://github.com/tylern91/obsidian-mcp-server/releases/latest/download/obsidian-mcp-<version>-darwin-arm64.tar.gz.sha256
shasum -a 256 -c obsidian-mcp-<version>-darwin-arm64.tar.gz.sha256
tar -xf obsidian-mcp-<version>-darwin-arm64.tar.gz
install -m 0755 obsidian-mcp-<version>-darwin-arm64/obsidian-mcp ~/.local/bin/obsidian-mcp
```

### Homebrew (macOS/Linux)

```bash
brew tap tylern91/obsidian-mcp
brew install obsidian-mcp
```

### go install

Requires Go 1.27+ to build from source. Building with `GOTOOLCHAIN=local` on an older Go
requires upgrading first — `GOTOOLCHAIN=auto` (the default since Go 1.21) downloads a matching
toolchain automatically.

```bash
go install github.com/tylern91/obsidian-mcp-server/cmd/obsidian-mcp@latest
```

### Build from source

Requires Go 1.27+. Building with `GOTOOLCHAIN=local` on an older Go
requires upgrading first — `GOTOOLCHAIN=auto` (the default since Go 1.21) downloads a matching
toolchain automatically.

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

### Cursor

[![Install MCP Server](https://cursor.com/deeplink/mcp-install-dark.svg)](https://cursor.com/en/install-mcp?name=obsidian&config=eyJjb21tYW5kIjoib2JzaWRpYW4tbWNwIiwiYXJncyI6WyItLXZhdWx0IiwiL3BhdGgvdG8veW91ci92YXVsdCJdfQ%3D%3D)

Edit the vault path after install — the deeplink can't know it in advance.

### VS Code

[![Install in VS Code](https://img.shields.io/badge/VS_Code-Install_Server-0098FF?style=flat-square&logo=visualstudiocode&logoColor=white)](https://vscode.dev/redirect/mcp/install?name=obsidian&config=%7B%22command%22%3A%22obsidian-mcp%22%2C%22args%22%3A%5B%22--vault%22%2C%22%2Fpath%2Fto%2Fyour%2Fvault%22%5D%7D)
[![Install in VS Code Insiders](https://img.shields.io/badge/VS_Code_Insiders-Install_Server-24bfa5?style=flat-square&logo=visualstudiocode&logoColor=white)](https://insiders.vscode.dev/redirect/mcp/install?name=obsidian&config=%7B%22command%22%3A%22obsidian-mcp%22%2C%22args%22%3A%5B%22--vault%22%2C%22%2Fpath%2Fto%2Fyour%2Fvault%22%5D%7D)

Edit the vault path after install — the deeplink can't know it in advance.

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
| `--read-only` | `OBSIDIAN_READ_ONLY` | `false` | CLI: bare `--read-only` enables it. Env var: same `ParseBool` rules as `--pretty`. When enabled, mutating tools (`write_note`, `delete_note`, `move_note`, etc.) are not registered — they never appear in `tools/list`. |
| `--trash-retention-days` | `OBSIDIAN_TRASH_RETENTION_DAYS` | `30` | Integer ≥ `0`. Non-integer or negative causes a startup error. `delete_note` moves notes to `.obsidian-mcp/trash/<timestamp>/<path>` by default instead of hard-deleting; entries older than this many days are pruned once at startup. |
| `--vault-name` | `OBSIDIAN_VAULT_NAME` | the vault directory's basename | Used to build the `obsidian://open?vault=<name>&file=<path>` deep links in `search_notes`, `search_regex`, `list_directory`, and `get_recent_changes` results. Set explicitly if the vault directory's name doesn't match the name Obsidian shows for it. |
| `--transport` | `OBSIDIAN_TRANSPORT` | `stdio` | `stdio` or `http`. `http` starts a TLS-secured Streamable HTTP listener instead of speaking MCP over stdio — see `SECURITY.md` § HTTP transport. |
| `--http-bind` | `OBSIDIAN_HTTP_BIND` | `127.0.0.1` | Bind address for `--transport http`. Non-loopback addresses are refused unless `--allow-non-loopback` is also set. |
| `--http-port` | `OBSIDIAN_HTTP_PORT` | `8443` | Port for `--transport http`. |
| `--allow-non-loopback` | `OBSIDIAN_ALLOW_NON_LOOPBACK` | `false` | Allows `--http-bind` to a non-loopback address. Requires non-empty `--allowed-hosts` and `--allowed-origins` — an explicit, three-flag confirmation gate. |
| `--allowed-hosts` | `OBSIDIAN_ALLOWED_HOSTS` | *(empty)* | Comma-separated Host header allowlist for `--transport http`. Required with `--allow-non-loopback`. |
| `--allowed-origins` | `OBSIDIAN_ALLOWED_ORIGINS` | *(empty)* | Comma-separated Origin header allowlist for `--transport http`. Required with `--allow-non-loopback`. |
| `--client-ca` | `OBSIDIAN_CLIENT_CA` | *(none)* | Path to a PEM file of trusted client CAs. Enables mandatory mutual TLS for `--transport http` — connections without a valid client certificate are rejected. |

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

# Streamable HTTP transport instead of stdio (loopback only, TLS + bearer
# token auto-generated on first run — see SECURITY.md § HTTP transport)
obsidian-mcp --vault ./my-vault --transport http --http-port 8443
```

**Precedence in action**: with `OBSIDIAN_LOG_LEVEL=debug` exported, `obsidian-mcp --vault ... --log-level info` runs at `info` — the explicit flag wins. Unset flags inherit the env var; if neither is set, the default applies.

## Security

All paths are validated through a 4-layer security model before any filesystem operation:

1. **Lexical** — rejects absolute paths, `..` traversal, and null bytes
2. **Filter** — blocks ignored patterns (`.git`, `.obsidian`, etc.) and unapproved extensions
3. **Existence** — verifies the file exists with a case-insensitive fallback; rejects ambiguous matches
4. **Symlink** — resolves symlinks and verifies the target remains inside the vault root

The optional `--transport http` listener has its own security posture (TLS, bearer auth,
loopback-only default, session binding) — see [`SECURITY.md`](SECURITY.md) § HTTP transport.

## Project Structure

```text
cmd/obsidian-mcp/     Entry point, transport selection (stdio/http)
internal/
  config/             CLI flags, env vars, defaults
  vault/              Path security, CRUD, frontmatter, tags, links, mutations
  tools/              MCP tool registrations and handlers
  response/           Token counting, JSON formatting
  search/             BM25 ranked search, regex/glob
  periodic/           Periodic note resolution (Phase 4)
  prompts/            MCP Prompt templates
  resources/          MCP Resource registrations
  httptransport/      Streamable HTTP transport: TLS, bearer auth, session binding
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

[Apache-2.0](LICENSE)
