# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
