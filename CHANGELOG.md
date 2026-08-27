# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

**Vault write paths: size cap, TOCTOU races, and Windows-unsafe paths**
- The 16 MB size cap was enforced only by `ReadNote`; `update_frontmatter`, `patch_note`,
  `manage_tags`, and the vault-wide tag aggregation read files uncapped. All read paths now share
  a single capped `readNoteBytes` helper.
- `write_note` in append/prepend mode checked only the incoming content against the cap, not the
  combined result — repeated appends could grow a note without bound. Now checked after assembly.
- `move_note`, `manage_tags`, and `update_frontmatter` checked their target didn't already exist
  before acquiring the internal write lock. For `move_note` this was a genuine data-loss race: two
  concurrent moves to the same destination could both pass the check and both write, silently
  clobbering one. All three checks now run inside the lock.
- `manage_tags` and `update_frontmatter` did not check for request cancellation on entry.
- `sanitizePath` now rejects Windows-unsafe path forms regardless of build platform: drive-relative
  paths (`C:foo`), NTFS alternate data streams (`note.md:hidden`), reserved device names (`NUL`,
  `CON`, `COM1`, etc.), and trailing dots/spaces that Windows silently strips.

**Token counting no longer reaches the network**
- `CountTokens` loaded the `cl100k_base` rank table via `tiktoken-go`'s default HTTP loader, which
  fetched `openaipublic.blob.core.windows.net` on a cold cache — a filesystem MCP server silently
  reaching the network on every response. The rank table is now vendored
  (`internal/response/assets/cl100k_base.tiktoken`) and loaded via `go:embed`; token counting is
  fully offline. Removed the `len(text)/4` fallback that masked encoder-init failure — that path is
  now unreachable, so it panics loudly instead of silently returning a wrong count.

### Changed

**Minimum requirements**
- Go build/toolchain floor raised `1.23` → `1.27.0`. Building from source with `GOTOOLCHAIN=local`
  now requires Go 1.27+; `GOTOOLCHAIN=auto` (the Go 1.21+ default) upgrades automatically.
- `github.com/mark3labs/mcp-go` `v0.32.0` → `v0.58.0`
- `github.com/pkoukk/tiktoken-go` `v0.1.7` → `v0.1.8`
- `github.com/stretchr/testify` `v1.9.0` → `v1.12.1` (test-only)
- Added explicit `golang.org/x/text v0.41.0` requirement, overriding a vulnerable transitive
  version pulled in by `mcp-go` (GO-2026-5970, affected range `[0, 0.39.0)`)

---

## [0.1.0] - 2026-06-22
### Added

- `--version` flag prints the binary version to stdout and exits without requiring `--vault`.

### Changed

**Code quality / simplification pass**
- Eliminated double-vault-walk in `get_vault_stats` and vault resource handler — new `(*vault.Service).VaultStats` does a single walk returning all aggregate metrics (`NoteCount`, `TotalBytes`, `TotalLinks`, `TotalTokens`, `Oldest`/`Newest`, `TagCounts`)
- `list_all_tags` and `get_vault_stats` top-tags response: JSON key renamed `"tag"` → `"name"` (aligns with `vault.TagCount` type)
- Extracted `vault.Stem` / `vault.StemLower` helpers; eliminated 7 inline `strings.TrimSuffix(filepath.Base, filepath.Ext)` chains across audit, prompts, search, and links
- Extracted `vault.MergeNoteTags` helper; replaced 3 separate frontmatter+inline tag merge blocks
- Extracted `vault.TopTagsByCount` helper; replaced 4 inline sort-and-cap tag-ranking loops
- Extracted `tools.parseJSONArg[T]` generic helper; replaced 5 inline `json.Unmarshal` blocks in tool handlers
- Extracted `prompts.singleUserPrompt` helper; collapsed identical return-wrapping in 5 prompt handlers
- Collapsed 7-fold env-override copy-paste in `config.Load` into `envString`/`envBool`/`envInt`/`envStringSlice` helpers
- Collapsed 3 identical map-copy loops in `periodic.LoadConfig` into `mergeStringMap` helper
- Removed `vault.WriteMode("overwrite")` string cast in favour of `vault.WriteModeOverwrite` constant
- Deleted 18 narration comments across `tools/`, `search/`, and `vault/` packages

---

## [1.0.0] - 2026-04-27

### Added

**Phase 5 — MCP Prompts and Resources**
- `summarize_note` prompt — structures a note into bullets, entities, and open questions
- `daily_note_review` prompt — reviews today's and yesterday's daily notes; surfaces TODOs, link suggestions, and tag gaps
- `weekly_review` prompt — produces a weekly retrospective from the last 7 daily notes
- `find_related` prompt — suggests related notes grouped by tag-sibling, citation, topical, and bidirectional relationships
- `vault_health_check` prompt — audits orphans, dangling links, untagged notes, and duplicate titles; asks the LLM to prioritize fixes
- `obsidian://vault/stats` static resource — note count, total size, top tags
- `obsidian://vault/tags` static resource — full tag index sorted by frequency
- `obsidian://note/{path}` resource template — raw markdown content for any vault note
- `obsidian://periodic/{granularity}` resource template — current daily/weekly/monthly/quarterly/yearly note
- `obsidian://backlinks/{path}` resource template — backlink graph for any note with line numbers and snippets

**Phase 4 — Batch operations and vault intelligence**
- `read_multiple_notes` tool — batch-read up to `--max-batch` notes in one call
- `get_notes_info` tool — metadata (size, modTime, tagCount, linkCount) for multiple notes without reading full content
- `get_vault_stats` tool — aggregate vault statistics; optional token counting
- `get_periodic_note` tool — resolve and optionally create a periodic note by granularity and offset
- `get_recent_periodic_notes` tool — list the N most recent periodic notes with optional summaries
- `get_recent_changes` tool — vault-wide recent modifications, filterable by ISO-8601 date
- `audit_notes` tool — vault hygiene classes: orphans, dangling links, untagged notes, duplicate titles

**Phase 3 — Search**
- `search_notes` tool — BM25 Okapi ranked full-text search with match snippets and phrase bonus
- `search_regex` tool — RE2 regex and glob pattern search across note paths and content

**Phase 2 — Metadata and mutations**
- `get_frontmatter` tool — parse YAML frontmatter as structured data
- `update_frontmatter` tool — set or remove frontmatter keys with format-preserving rewrites
- `manage_tags` tool — add or remove tags from frontmatter or inline locations
- `list_all_tags` tool — vault-wide tag aggregation with counts
- `get_backlinks` tool — on-demand reverse link graph (wikilinks and markdown links)
- `patch_note` tool — heading-anchored content patch with before/after/replace_body positions
- `delete_note` tool — permanent deletion with confirmation guard
- `move_note` tool — rename or relocate a note within the vault

**Phase 1 — Core**
- `read_note` tool — read note content and metadata
- `write_note` tool — create or update a note (overwrite/append/prepend)
- `list_directory` tool — list vault files and subdirectories
- 4-layer path security: lexical validation, filter, case-insensitive existence, symlink escape prevention
- `cl100k_base` token counting on all responses
- Stdio transport compatible with Claude Code, Claude Desktop, and any MCP client
