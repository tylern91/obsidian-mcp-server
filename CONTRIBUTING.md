# Contributing to obsidian-mcp-server

Thanks for considering a contribution. This document covers everything a new
contributor needs to land a correct PR without having to ask.

## ⚠️ Before your first commit: GPG signing is mandatory

`main` is protected by a repository ruleset that requires every commit to be
GPG-signed (`required_signatures`, `bypass_actors: []`) — an unsigned commit
is rejected at push, not just at merge. Set up commit signing before you push
anything:

```sh
git config commit.gpgsign true
git config user.signingkey <your-key-id>
```

The same ruleset also enforces:

- **Squash-merge only** — the target branch never sees your individual
  commits, only one squashed commit per PR.
- **Linear history** — no merge commits.
- No force-push, no branch deletion on `main`.

## Design principles

These aren't aspirational — they're enforced by the code shape or by CI.
Know them before proposing a change that cuts against one:

- **Filesystem-based, no Obsidian app dependency.** The server operates
  directly on a vault directory. Don't add a code path that requires the
  Obsidian desktop app, a plugin API, or any process other than this binary.
- **Path security is the contract.** `sanitizePath` is purely lexical;
  `checkSymlinksForWrite` and the write syscall run inside `s.mu.Lock()`.
  Read paths resolve case-insensitively (`ResolvePath` → `existenceCheck`);
  write paths use `existsStrict` and never fall back case-insensitively — see
  `.claude/CLAUDE.md` § Critical conventions and `SECURITY.md` before
  touching anything under `internal/vault/`.
- **Stdout is reserved for JSON-RPC.** Never `fmt.Println` from server code;
  logs go to stderr via the slog JSON handler. A stray stdout write breaks
  every client's transport.
- **No telemetry.** The server makes no outbound network calls of its own —
  BM25 search is computed per-query, in-process, with no persistent index and
  no external service call.
- **Single static Go binary.** No cgo, no native-module rebuild per platform.
  A new dependency needs to justify itself.

## Ways to contribute

| Type | How |
|---|---|
| Report a bug | Open an issue with repro steps, `obsidian-mcp --version`, and OS/arch |
| Fix a bug | See the local gate below, then open a PR |
| Add a feature | Consider opening an issue first for anything touching path security or the tool surface |
| Review a PR | Check it against the design principles above, not just style |
| Improve docs | README, `.claude/CLAUDE.md`, and this file all welcome fixes |

## Commit convention

[Conventional Commits](https://www.conventionalcommits.org/):

- **Types:** `feat`, `fix`, `chore`, `ci`, `docs`, `refactor`, `test`, `perf`
- **Scopes:** match the package touched — `vault`, `search`, `periodic`,
  `tools`, `response`, `config`, `prompts`, `release`
- Bare `docs:` / `chore:` with no scope are fine.

**Because merges are squash-only, your PR title becomes the commit message.**
Release automation reads the title for breaking-change escalation (see
below), so title it as you'd want it to read in `CHANGELOG.md`.

## Branch naming

`<type>/<kebab-case-slug>` — `fix/`, `feat/`, `docs/`, `ci/`, and `chore/` are
all in use. This is a convention, not an enforced gate.

## Local gate before pushing

Run the exact commands CI runs, before you push:

```sh
make lint     # go vet + gofmt -l check
make test     # go test -race ./...
make build    # go build -o obsidian-mcp ./cmd/obsidian-mcp/
```

CI runs the same three on every PR that touches Go source, plus a
`govulncheck ./...` dependency-vulnerability scan.

## Go toolchain floor

`go.mod` pins `go 1.27.0` as the actual floor of the resolved dependency
graph, not an arbitrary or aspirational number. Don't raise it casually to
pick up a new API; if a dependency bump pushes the floor higher, that's when
it moves. Raising it is a **minor** version bump — it can break users on
older toolchains.

## Version + CHANGELOG convention

A PR that should ship in a release **finalizes its own release section** in
`CHANGELOG.md`:

1. Add `## [X.Y.Z] - YYYY-MM-DD` above the previous release heading.
2. Bump `const Version` in `internal/version/version.go` to match `X.Y.Z`
   exactly.
3. Leave `## [Unreleased]` in the file — **empty**, as a permanent
   placeholder. Never delete it: the release workflow's empty-notes guard
   silently no-ops a release if `## [Unreleased]` is missing entirely.
4. Use the existing buckets — `### Added` / `Changed` / `Fixed` /
   `Documentation` — followed by a `---`. Write bullets as prose explaining
   *why* the change matters.

## Semver labels

Apply exactly one on your PR: `patch`, `minor`, `major`, or `skip-release`.
**A PR with no label produces no release** — the release workflow simply
doesn't run.

Docs-only changes use `skip-release`, **not** `patch` — `patch` fails the
changelog gate on a PR that adds no `CHANGELOG.md` entry.

## Breaking changes

Escalate a `minor`/`major` label to a major version bump with any of:

- A PR title matching `^[a-z]+(\([^)]+\))?!:` (e.g. `feat(vault)!: ...`)
- A `BREAKING CHANGE:` line in the PR body
- The `breaking-change` label

## No CLA, no DCO, no sign-off

Your PR is not gated on signing a Contributor License Agreement or adding a
`Signed-off-by` trailer. Submitting a PR is enough.

## Don't touch — looks usable, isn't

- **`testdata/vault/`** — load-bearing fixture. Tag counts, link graphs, and
  `.obsidian/plugins/periodic-notes/data.json` back real test assertions.
  Use `t.TempDir()` and copy fixtures rather than mutate this directory.
