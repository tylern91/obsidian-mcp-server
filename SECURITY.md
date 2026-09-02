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

`--transport http` starts a Streamable HTTP listener (`internal/httptransport/`)
in addition to the default stdio transport. It is secured by default:

- **Loopback-only unless explicitly widened.** The listener refuses to bind
  a non-loopback address unless `--allow-non-loopback` is passed together
  with non-empty `--allowed-hosts` and `--allowed-origins` — an explicit,
  three-flag confirmation gate, not a single switch.
- **TLS by default, TLS 1.3 minimum.** A self-signed certificate and key
  are generated on first run and persisted (0600) under the OS user config
  directory (`os.UserConfigDir()/obsidian-mcp`) — **never under the vault
  path**, since a key stored there would be exfiltratable through the very
  read/search API it protects. Because TLS is always on, Go negotiates
  HTTP/2 via ALPN for free; there is no plaintext fallback.
- **Bearer token authentication.** A 32-byte token (`crypto/rand`) is
  generated on first run and written to a 0600 file in the same state
  directory. It is never printed to stderr and has no `--auth-token
  <value>` flag form (both would leak it — log persistence and
  `/proc/<pid>/cmdline`/`ps` respectively); `OBSIDIAN_AUTH_TOKEN` is
  available as an explicit opt-in override. Only the token file's path and
  `sha256(token)[:8]` are ever logged. Comparison hashes both sides to a
  fixed width before `subtle.ConstantTimeCompare`, so a length mismatch
  can't be timed.
- **Sessions are bound to their issuing credential.** The custom
  `SessionIdManager` (`internal/httptransport/session.go`) rejects a
  session ID presented with a different bearer token or client
  certificate than the one that created it.
- **Optional mutual TLS** via `--client-ca <path>`, enforced with
  `tls.RequireAndVerifyClientCert` — never the fail-open
  `VerifyClientCertIfGiven`.
- **Request bodies are capped** before authentication or handler logic
  runs, and Host/Origin are checked against the configured allowlists
  before any body is read.
- **No secrets in logs, ever**, including at `--log-level debug`: the
  Authorization header and request/response bodies are never logged.
  Stdout stays reserved for JSON-RPC; all listener logs go to stderr.

**Caveat:** the certificate is self-signed and not issued by a public CA.
A client connecting for the first time has no independent way to confirm
it's talking to the right server beyond a manual fingerprint or SPKI pin
check outside this document's scope — treat the generated cert the same
way you'd treat an SSH host key on first connect.
