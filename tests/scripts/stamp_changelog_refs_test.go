// Package scripts_test exercises the release scripts against throwaway git repositories.
package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const stubGH = `#!/bin/sh
# gh pr view <n> --repo <slug> --json author -q .author.login
case "$3" in
  1) echo octocat ;;
  2) echo app/dependabot ;;
esac
`

// fixture builds a repo with two PR commits between v0.1.0 and v0.2.0 and returns the
// repo path, the two short hashes, and an env that puts a stub gh first on PATH.
func fixture(t *testing.T) (dir, alpha, bump string, env []string) {
	t.Helper()
	for _, bin := range []string{"git", "bash"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	dir = t.TempDir()
	bin := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(stubGH), 0o755))

	env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"REPO_SLUG=acme/widgets",
	)
	run := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	run("git", "init", "-q", "-b", "main")
	write("README.md", "start\n")
	run("git", "add", ".")
	run("git", "commit", "-q", "-m", "chore: start")
	run("git", "tag", "v0.1.0")

	write("alpha.go", "package alpha\n\nfunc alphaFunc() {}\n")
	run("git", "add", ".")
	run("git", "commit", "-q", "-m", "feat(alpha): add the alpha helper (#1)")
	alpha = run("git", "rev-parse", "--short", "HEAD")

	write("go.mod", "module x\n\nrequire foo/bar v1.2.0\n")
	run("git", "add", ".")
	run("git", "commit", "-q", "-m", "chore(deps): bump foo/bar to v1.2.0 (#2)")
	bump = run("git", "rev-parse", "--short", "HEAD")
	run("git", "tag", "v0.2.0")
	return dir, alpha, bump, env
}

func stamp(t *testing.T, dir string, env []string, changelog string) (stampedLines []string, stderr string) {
	t.Helper()
	path := filepath.Join(dir, "CHANGELOG.md")
	require.NoError(t, os.WriteFile(path, []byte(changelog), 0o644))
	script, err := filepath.Abs("../../scripts/stamp-changelog-refs.sh")
	require.NoError(t, err)

	cmd := exec.Command("bash", script, "0.2.0")
	cmd.Dir = dir
	cmd.Env = append(env, "CHANGELOG="+path)
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	require.NoError(t, cmd.Run(), errBuf.String())

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Split(string(body), "\n"), errBuf.String()
}

func lineWith(t *testing.T, lines []string, needle string) string {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(l, needle) {
			return l
		}
	}
	t.Fatalf("no line contains %q in:\n%s", needle, strings.Join(lines, "\n"))
	return ""
}

const changelog = "# Changelog\n\n## [0.2.0] - 2026-01-01\n\n### Added\n\n" +
	"- `alphaFunc` does the alpha thing.\n" +
	"- Bump `foo/bar` to v1.2.0.\n" +
	"- Something vague that names nothing in particular.\n\n" +
	"## [0.1.0] - 2025-12-01\n\n- start\n"

func TestStampMatchesEachBulletToItsOwnCommit(t *testing.T) {
	dir, alpha, bump, env := fixture(t)
	lines, stderr := stamp(t, dir, env, changelog)

	one := lineWith(t, lines, "alphaFunc")
	require.Contains(t, one, "/commit/"+alpha)
	require.NotContains(t, one, "/commit/"+bump)
	require.Contains(t, one, "[#1](https://github.com/acme/widgets/pull/1)")
	require.Contains(t, one, "[@octocat](https://github.com/octocat)")

	two := lineWith(t, lines, "Bump `foo/bar`")
	require.Contains(t, two, "/commit/"+bump)
	require.NotContains(t, two, "/commit/"+alpha)

	require.Contains(t, stderr, "MATCH: v0.2.0: "+alpha)
}

func TestStampLinksBotAuthorsToTheAppPage(t *testing.T) {
	dir, _, _, env := fixture(t)
	lines, _ := stamp(t, dir, env, changelog)

	two := lineWith(t, lines, "Bump `foo/bar`")
	require.Contains(t, two, "[@dependabot](https://github.com/apps/dependabot)")
	require.NotContains(t, two, "github.com/app/")
}

func TestStampFallsBackToAllRefsWhenNothingMatches(t *testing.T) {
	dir, alpha, bump, env := fixture(t)
	lines, stderr := stamp(t, dir, env, changelog)

	vague := lineWith(t, lines, "Something vague")
	require.Contains(t, vague, "/commit/"+alpha)
	require.Contains(t, vague, "/commit/"+bump)
	require.Contains(t, stderr, "REVIEW: v0.2.0: - Something vague")
}

func TestStampIsIdempotent(t *testing.T) {
	dir, _, _, env := fixture(t)
	first, _ := stamp(t, dir, env, changelog)
	body := strings.Join(first, "\n")
	second, _ := stamp(t, dir, env, body)
	require.Equal(t, first, second)
}
