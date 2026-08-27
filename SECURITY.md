# Security Policy

## Supported versions

obsidian-mcp-server releases from `main` only; there are no maintained
release branches. Security fixes land in the latest release.

## Reporting a vulnerability

Please report suspected vulnerabilities privately, using
[GitHub's private vulnerability reporting](https://github.com/tylern91/obsidian-mcp-server/security/advisories/new)
for this repository, rather than opening a public issue. This lets a fix
land before the details are public.

Include what you'd include in a bug report: repro steps, affected version
(`obsidian-mcp --version`), and impact. Response time isn't guaranteed —
this is a single-maintainer project — but reports will be triaged.

## Automated scanning

Every PR that touches Go source runs `govulncheck ./...` against the
dependency tree. A known-vulnerable dependency with no available fix blocks
the merge.

## Threat model: vault filesystem access

This server gives any connected MCP client (an editor, an agent, a chat
client) read/write access to a filesystem-based Obsidian vault. The trust
boundary is: **the process that spawns this server is trusted; the vault
contents and any tool-call arguments derived from an LLM are not.**

The security-relevant guarantees, in `internal/vault/`:

- **Path confinement.** `sanitizePath` lexically resolves every incoming
  path and rejects anything that would escape the configured vault root
  (`..` traversal, absolute paths outside the root, etc.) before any
  filesystem call is made.
- **Symlink escapes are blocked on write.** `checkSymlinksForWrite` walks
  the resolved path's symlink chain inside the same lock that performs the
  write, closing the TOCTOU window between the check and the syscall — see
  `.claude/CLAUDE.md` § Critical conventions for the exact ordering
  invariant. This defends against races *within this process*; a local
  attacker who already has independent write access to the vault directory
  can still race the check against an external write to that directory —
  that's a filesystem-level threat outside a single process's control, not
  a gap in this defense.
- **Size caps.** Reads and writes are capped at 16 MB to bound memory use
  against a maliciously large or crafted file.
- **Windows-unsafe path rejection.** Reserved device names (`CON`, `NUL`,
  trailing dots/spaces, etc.) are rejected so a vault note can't collide
  with a Windows filesystem special path.
- **Read/write resolution asymmetry is deliberate.** Reads resolve
  case-insensitively (`ResolvePath` → `existenceCheck`, returning
  `ErrAmbiguousPath` on a case collision); writes use `existsStrict` and
  never fall back case-insensitively. A write needs certainty about exactly
  which file is being mutated — see `.claude/CLAUDE.md` § Path security.

## HTTP transport

Not yet implemented. This server currently speaks MCP over stdio only —
there is no network listener, so there is no remote attack surface beyond
the local process boundary described above. When an HTTP transport is
added, its authentication and default-bind posture will be documented here
as a secured default, not deferred to a "known limitation" note.
