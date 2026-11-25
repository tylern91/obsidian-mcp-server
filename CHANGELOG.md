# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.4] - 2026-11-25

### Security

- A persisted self-signed TLS certificate that has expired, or expires within 30 days, is now
  regenerated at startup. It was reused forever, so a long-lived install eventually served an
  expired certificate. The auth token is untouched. A client that pinned the old certificate must
  trust the new one.
- A bearer token shorter than 32 characters now logs a startup warning with its length, never its
  value. Short tokens are still accepted.
- `--session-max-lifetime` (`OBSIDIAN_SESSION_MAX_LIFETIME`) sets an optional absolute cap on
  session age, so a stolen session ID cannot be kept alive by activity alone. It is off by default.
- GitHub Actions in every workflow are pinned to full commit SHAs, and the release workflow's App
  tokens request only the `contents` permission.

### Fixed

- Note writes are atomic: a same-directory temp file is synced and renamed onto the target, so a
  crash leaves the old or the new content and never a truncated note. Symlinked notes keep their
  link, existing files keep their mode, new files honor the umask, and a read-only note is still
  refused instead of being replaced.
- `vault/stats` and `vault/tags` are invalidated by every note write, move and delete. They keyed
  on the vault root mtime, which an in-place or nested edit never changes, so stale results were
  served for up to 30 seconds.
- `--ignore` patterns and paths are compared in Unicode NFC, so a pattern typed in NFC matches the
  NFD names macOS returns, and the reverse.
- The release workflow passed a `v`-prefixed version to the CHANGELOG provenance step, which
  expects a bare one, and hid the failure with `|| true`, so release notes never carried commit
  and PR links. The step now receives the bare version and fails the release when it errors.

### Changed

- `replace_in_note` rejects a regex that can match the empty string (`x*`, `^`, `\b`, `a?`) with a
  tool error. Such a pattern matched at every position and silently rewrote the whole note.
- The HTTP transport answers an oversized request body with `413 Request Entity Too Large` instead
  of `400`, following `mcp-go` 1.5.0 (bumped from 1.3.0).

## [0.3.3] - 2026-11-14

### Security

- The `--ignore` list and the internal `.obsidian-mcp/` state directory are now matched
  case-insensitively. On case-insensitive filesystems (macOS APFS, Windows NTFS) a path such as
  `.GIT/config` previously passed the guard and reached the real `.git` directory. The match is
  case-insensitive on every platform, so on a case-sensitive filesystem `--ignore archive` now
  also hides a sibling `Archive/`.
- Symlinks are re-checked against the ignore list and internal state after resolution, so a note
  that links to an ignored or internal path can no longer be read or written through the link.
  Writes through a symlinked directory into a not-yet-existing subdirectory get the same check,
  including the internal-state directory, and a read through a symlink re-checks the allowed
  extension of the target.
- `delete_note` and trash pruning refuse to run when `.obsidian-mcp` is a symlink that leaves the
  vault, instead of moving or deleting notes outside it.
- `WalkNotes` (search, audit, backlinks and the other vault walks) skips symlinked files whose
  target is outside the vault, ignored, internal, or not a regular file.
- `delete_note` and `move_note` reject directory paths (new `NOT_A_NOTE` error) instead of
  trashing or moving a whole folder in one call.

### Fixed

- `rename_tag` and link rewriting on `move_note` re-read each note under the vault write lock, so
  a concurrent edit is no longer overwritten with stale content.
- `move_note` reports success with `linksComplete: false` and `linkRewriteError` when the file was
  moved but a later link rewrite failed, instead of reporting a failure for a completed move.
- `move_note` no longer treats a dot in a note name as a file extension (moving `Release 1.md` no
  longer corrupts `[[Release 1.2]]`), and no longer rewrites a bare link to a basename that
  another note now shares; it writes the path form instead. Dry-run and real runs agree.
- `write_note` in `append` and `prepend` mode only treats a missing file as empty; any other read
  error is returned instead of overwriting the note.
- `update_frontmatter`, `rename_tag` and `manage_tags` handle CRLF notes instead of adding a
  second frontmatter block or mixing line endings. Frontmatter and tags in CRLF notes are now
  also recognised by the read paths (`get_frontmatter`, tag counts, search).
- `replace_in_note` rejects an empty pattern, accepts an optional `if_match` etag like the other
  single-note mutators, and no longer miscounts or fails context checks when re-running a regex.
- Tool arguments sent as native JSON arrays or objects (not JSON-encoded strings) are accepted for
  array and object parameters; a wrong type is now a tool error instead of being dropped silently.
- `search_regex` and `get_backlinks` no longer fail or silently drop results when a note has a
  line longer than 64 KiB.
- HTTP sessions now expire 30 minutes after last use, not 30 minutes after creation.
- The HTTP transport accepts bracketed IPv6 `Host` headers such as `[::1]:8080`.
- Resource reads no longer cache error results for the TTL window.
- `install.sh` falls back to `sha256sum` when `shasum` is not installed.
- CI and release builds now use Go `1.27.4` (was `1.27.3`). Go 1.27.3's `net/http` carries
  GO-2026-6859, reachable from the server binary and fixed in 1.27.4. This turned the
  `govulncheck` job red. The `go.mod` floor stays at `go 1.27.0`.

### Changed

- BM25 search builds snippets and token counts only for the notes it returns, not for every
  scored note.
- CI runs `shellcheck` over `install.sh` and the scripts.

## [0.3.2] - 2026-10-27

### Fixed

- CI and release builds now use Go `1.27.3` (was `1.27.2`). Go 1.27.2's `crypto/x509` carries
  GO-2026-6809 (compromised intermediate certificates are no longer distrusted), reachable
  from the server binary and fixed in 1.27.3. This turned the scheduled `govulncheck` job red,
  which in turn made the Trivy code-scanning configuration report "Workflow runs failing", and
  meant the `v0.3.1` binaries shipped the vulnerable standard library. The `go.mod` floor stays
  at `go 1.27.0`.

## [0.3.1] - 2026-10-13

### Changed

- Bumped `github.com/mark3labs/mcp-go` from `v1.0.0` to `v1.1.0`. Notable upstream fixes that
  reach this server: tool-schema property-order preservation through decode/encode, and
  cancellation of in-flight task execution on TTL expiration.

### Fixed

- CI and release builds now use Go `1.27.2` (was `1.27.0`). Go 1.27.0's standard library
  carries nine advisories (GO-2026-6603, -6605, -6607, -6608, -6610, -6611, -6612, -6613,
  -6617) in `crypto/tls`, `net/http`, `net/http/internal/http2` and `net/textproto`, all
  reachable through the HTTP transport and the tiktoken encoding loader, and all fixed in
  1.27.2. This turned the scheduled `govulncheck` job red and meant the `v0.3.0` binaries
  shipped the vulnerable standard library. `golangci-lint` moved `v2.13.1` → `v2.14.0` in the
  same change, because `v2.13.1` cannot read Go 1.27.2's compiler export data and failed the
  lint job with `typecheck` errors. The `go.mod` floor stays at `go 1.27.0`.
- `server.json` was left pointing at the `v0.2.0` `.mcpb` asset with a placeholder
  `fileSha256`, from before the `v0.3.0` release actually built and published that bundle.
  Synced to the real `v0.3.0` asset URL and its published SHA-256.
- Dependabot's own `minor`/`major` semver labels collided with the release workflow's
  identically-named bump labels, so merging a dependency PR attempted a release and failed
  the version gate (`internal/version/version.go says v0.3.0 but label 'minor' ... implies
  v0.4.0`). Renamed the release bump labels to `release:patch`/`release:minor`/
  `release:major`, excluded `dependabot[bot]` from the release job by author, and gave
  Dependabot's own PRs the `skip-release` label. The release job's `if` now also requires
  one of the `release:*` labels, so an unlabeled merge genuinely produces no release
  (previously it silently defaulted to `patch` and failed the same gate).
- `actions/create-github-app-token@v3`'s `app-id` input is deprecated in favor of
  `client-id`; switched both `release.yml` and `publish-assets.yml` to `client-id` (same
  `HOMEBREW_APP_ID` secret value — the action treats the two identically).

## [0.3.0] - 2026-09-02

### Added

- `install.sh` — curl-to-shell installer that resolves the right release asset for the caller's
  OS/architecture, verifies its checksum, and installs it to `~/.local/bin` (overridable).
- `.mcpb` bundle for one-click Claude Desktop install, built by `scripts/build-mcpb.sh` and
  published as a release asset via the new `build-mcpb` job in `publish-assets.yml`. The bundle
  packs all five release binaries behind a runtime arch-dispatch wrapper (`packaging/mcpb/launch.sh`)
  so darwin/linux resolve `arm64`/`amd64` at install time; Windows is pointed directly at its
  amd64 binary via `manifest.json`'s `platform_overrides`.
- `server.json` for the MCP registry (not yet submitted to the live registry).
- Cursor and VS Code one-click install badges in the README, using the config formats verified
  against `upstash/context7` and `github/github-mcp-server`'s own published badges.

## [0.2.0] - 2026-09-02

### Added

- `delete_note` now moves notes to `.obsidian-mcp/trash/<timestamp>/<path>` by default instead of
  hard-deleting, preserving vault-relative structure so a delete is recoverable. Pass
  `permanent: true` to hard-delete as before; `confirm` still guards both paths. Trash is pruned
  once at server startup via the new `--trash-retention-days` flag (`OBSIDIAN_TRASH_RETENTION_DAYS`,
  default 30).
- `--read-only` flag (`OBSIDIAN_READ_ONLY`) disables all mutating tools: they are omitted from
  `tools/list` entirely, not merely rejected at call time, so a client never sees them as an option.
- `.obsidian-mcp/` (the server's own state directory) is now unconditionally excluded from path
  resolution, directory listings, and search/audit walks — independent of `--ignore` — so overriding
  the ignore list can never resurface trashed notes as live content.
- `get_note_outline` — returns a note's heading tree (level, text, line number) without its body.
- `read_note_lines` — reads a bounded range of lines from a note (`startLine` + `lineCount`,
  default 200, capped at 2000), cheaper than `read_note` for a long note when only a section is
  needed.
- `search_notes`, `search_regex`, `list_directory`, and `get_recent_changes` results now include
  an `obsidian://open` `deepLink` field per note. Vault name derives from the vault directory's
  basename, overridable via `--vault-name` (`OBSIDIAN_VAULT_NAME`).
- `move_note` now rewrites inbound links (wikilinks and markdown links, including anchors, block
  refs, aliases, and URL-encoded paths) to referrers when a note moves. Ambiguous bare-basename
  targets are left untouched and reported in the response instead of guessed. New `updateLinks`
  (default `true`) and `dryRun` (default `false`, preview only) args on `move_note`.
- `rename_tag` — renames a tag vault-wide across both frontmatter (order-preserving) and inline
  occurrences, skipping fenced code blocks.
- `replace_in_note` — scoped search-and-replace within a single note, literal or regex, with an
  optional `maxOccurrences` cap; reports the true total found even when capped.
- Optimistic concurrency via ETags. `read_note`, `read_multiple_notes`, and `get_notes_info` now
  return a SHA-256 `etag` of note content. `write_note`, `patch_note`, `update_frontmatter`,
  `manage_tags`, `delete_note`, and `move_note` accept an optional `if_match`; a mismatch returns a
  `REVISION_CONFLICT` tool error instead of silently overwriting a concurrent edit. `if_match` is
  additive and optional everywhere — omitting it writes unconditionally, as before. Two edge cases:
  `if_match` against a note that doesn't exist yet is always a conflict, never an implicit create;
  and `move_note`'s `dryRun: true` preview does not enforce `if_match`.
- `--transport http` (default remains `stdio`) starts a Streamable HTTP transport secured by
  default: TLS 1.3 with an auto-generated, locally-persisted self-signed certificate; a
  `crypto/rand` bearer token (never printed, no `--auth-token <value>` argv form) with
  `OBSIDIAN_AUTH_TOKEN` as an env-var override; sessions bound to their issuing credential; and
  an explicit `--allow-non-loopback` + `--allowed-hosts` + `--allowed-origins` gate before binding
  any non-loopback address. Optional mutual TLS via `--client-ca`. New flags: `--transport`,
  `--http-bind` (default `127.0.0.1`), `--http-port` (default `8443`), `--allow-non-loopback`,
  `--allowed-hosts`, `--allowed-origins`, `--client-ca`; matching `OBSIDIAN_*` env vars. See
  `SECURITY.md` § HTTP transport for the full posture.

### Changed

- `delete_note`'s response now includes a `permanent` field reflecting how the delete was performed.

## [0.1.0] - 2026-08-29

Nothing was ever published under the previous `1.0.0`/`0.1.0` version strings this project carried
internally — this is the first real release, and it supersedes both.

### Added

- `--version` flag prints the binary version to stdout and exits without requiring `--vault`.

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

### Changed

- CI: hardened `go.yml`/`security.yml`/`publish-assets.yml` checkouts with
  `persist-credentials: false` and `fetch-depth: 1`, added job `timeout-minutes`
  across all workflows, pinned `actionlint`, `govulncheck`, and `golangci-lint`
  versions instead of floating on `latest`, bumped `aquasecurity/trivy-action` to
  `v0.36.0`, and added `concurrency` groups to `security.yml`/`publish-assets.yml`.
  `release.yml`'s checkout is intentionally unchanged (needs `fetch-depth: 0` and
  persisted App-token credentials to push a tag touching `.github/workflows/*`).
- Added `scripts/pre-push` (installed via `make install-hooks`) to block pushing a
  `v*` tag whose version doesn't match `internal/version/version.go`/`CHANGELOG.md`.
- Added `probity.config.mjs` (no-op) and `.gitattributes` (LF normalization).
- `release.yml`'s checkout now actually persists the App-token credential
  (`token: ${{ steps.app-token.outputs.token }}`) instead of the default,
  read-only `GITHUB_TOKEN` — the previous checkout order minted the token but
  never wired it into `actions/checkout`, so every tag push failed with a 403.
- `release.yml`'s `Compute next version` step now treats
  `internal/version/version.go` as the source of truth and uses
  `bump-version.sh` to validate, not derive, the next tag — failing fast with
  `::error::` on a mismatch instead of letting a wrong tag reach
  `publish-assets.yml`'s cross-compile matrix before it fails.

**`MaxResults` is now a hard ceiling on every result-bounded tool**
- Previously `deps.MaxResults` (`--max-results` / `OBSIDIAN_MAX_RESULTS`) behaved inconsistently:
  it was only a *fallback default* for `list_directory` (an explicit `limit` above it was honored
  in full), a real ceiling for `get_recent_changes` and `get_recent_periodic_notes`, and was not
  applied at all for `search_notes` / `search_regex`. All five tools now clamp their effective
  limit to `MaxResults` regardless of what the caller requests. If you were relying on an explicit
  `limit` to exceed `MaxResults` for `list_directory`, `search_notes`, or `search_regex`, raise
  `--max-results` instead.

**Minimum requirements**
- Go build/toolchain floor raised `1.23` → `1.27.0`. Building from source with `GOTOOLCHAIN=local`
  now requires Go 1.27+; `GOTOOLCHAIN=auto` (the Go 1.21+ default) upgrades automatically.
- `github.com/mark3labs/mcp-go` `v0.32.0` → `v0.58.0`
- `github.com/pkoukk/tiktoken-go` `v0.1.7` → `v0.1.8`
- `github.com/stretchr/testify` `v1.9.0` → `v1.12.1` (test-only)
- Added explicit `golang.org/x/text v0.41.0` requirement, overriding a vulnerable transitive
  version pulled in by `mcp-go` (GO-2026-5970, affected range `[0, 0.39.0)`)

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

### Security

- GitHub tag ruleset `release-tags-protection` now blocks `creation` and `update`
  on `refs/tags/v*` in addition to `deletion`/`non_fast_forward`/`required_signatures`
  — only the release workflow's GitHub App token can mint a `v*` tag.

### Fixed

- `release.yml`'s `Compute next version` step aborted on a repo with zero existing tags:
  `git tag | grep ... | head -1` exits 1 when `grep` matches nothing, and `set -o pipefail`
  turned that into a silent script abort before the `v0.0.0` fallback ever ran — blocking
  the very first release. The `grep` is now tolerant of no matches.
- `release.yml`'s `Build release notes` step always read `## [Unreleased]` regardless of
  `--fallback-unreleased`/`--prev`, because `--from-existing` was never passed to
  `build-release-notes.sh`. Once `[Unreleased]` was consolidated into `[0.1.0]`, this produced
  empty notes and silently skipped the release via the `Empty notes guard`. The step now passes
  `--from-existing` so it reads the version-matched section.

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

**`patch_note` heading matching is now consistent with the section-boundary scanner**
- `isHeadingLine` accepted any line whose `#`-trimmed text matched the target heading, including
  malformed ATX lines with no space after the `#` run (e.g. `#Introduction`). `headingLevel` — used
  immediately afterward to find the end of the section — requires that space and treats such a line
  as non-heading. The mismatch could return an empty level for the matched heading, corrupting the
  same-or-higher-level scan used by `before`/`after`/`replace_body` patches. `isHeadingLine` now
  requires the same ATX space rule as `headingLevel`.

**BM25 phrase-bigram scoring counted the wrong word pair**
- Multi-word search queries build a "phrase key" from the raw, pre-deduplication query tokens (e.g.
  `"cat cat dog"` → phrase key `cat\x00cat`), but the bigram counter scanned the document using the
  deduplicated term list (`["cat", "dog"]`), so it counted occurrences of "cat dog" and credited them
  to the "cat cat" phrase bonus. Any query with a repeated word scored the wrong bigram. The counter
  now derives the two words it counts from the phrase key itself.

**Token counting no longer reaches the network**
- `CountTokens` loaded the `cl100k_base` rank table via `tiktoken-go`'s default HTTP loader, which
  fetched `openaipublic.blob.core.windows.net` on a cold cache — a filesystem MCP server silently
  reaching the network on every response. The rank table is now vendored
  (`internal/response/assets/cl100k_base.tiktoken`) and loaded via `go:embed`; token counting is
  fully offline. Removed the `len(text)/4` fallback that masked encoder-init failure — that path is
  now unreachable, so it panics loudly instead of silently returning a wrong count.

**README manual-install instructions pointed at the wrong path**
- The release tarball extracts into a top-level `obsidian-mcp-<version>-<os>-<arch>/` directory
  (so `LICENSE`/`NOTICE` ship alongside the binary), but the manual-install snippet ran
  `install -m 0755 obsidian-mcp ...` from the extraction cwd, where no such file exists. Corrected
  to reference the binary inside the extracted directory.

### Changed

**`MaxResults` is now a hard ceiling on every result-bounded tool**
- Previously `deps.MaxResults` (`--max-results` / `OBSIDIAN_MAX_RESULTS`) behaved inconsistently:
  it was only a *fallback default* for `list_directory` (an explicit `limit` above it was honored
  in full), a real ceiling for `get_recent_changes` and `get_recent_periodic_notes`, and was not
  applied at all for `search_notes` / `search_regex`. All five tools now clamp their effective
  limit to `MaxResults` regardless of what the caller requests. If you were relying on an explicit
  `limit` to exceed `MaxResults` for `list_directory`, `search_notes`, or `search_regex`, raise
  `--max-results` instead.

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

Nothing was ever published under the previous `1.0.0`/`0.1.0` version strings this project carried
internally — this is the first real release, and it supersedes both.

### Added

- `--version` flag prints the binary version to stdout and exits without requiring `--vault`.

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
