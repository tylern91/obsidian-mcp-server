#!/usr/bin/env bash
# stamp-changelog-refs.sh — append commit/PR/author provenance to CHANGELOG bullets.
#
# Usage:
#   stamp-changelog-refs.sh <bare-version>
#
# Args:
#   bare-version  — e.g. 0.8.0 (no leading "v"); must already exist as a
#                   "## [<version>]" heading in CHANGELOG.md and as tag v<version>.
#
# Environment:
#   CHANGELOG     — path to CHANGELOG.md (default: ./CHANGELOG.md)
#   REPO_SLUG     — owner/repo (default: derived from `git remote get-url origin`)
#
# Behavior:
#   - Every "- " bullet in the version's section gets a trailing ref:
#       ([`<hash>`](.../commit/<hash>)) by [@user](.../<user>) in [#NN](.../pull/NN)
#   - Bullets spanning multiple lines (continuation lines indented 2 spaces) are
#     stamped on their LAST physical line, never mid-sentence.
#   - Idempotent: a bullet whose last line already contains "/commit/" is left alone.
#   - Single-commit release (the common case): every bullet gets that commit's ref.
#   - Multi-commit release: each bullet is matched to ONE commit, first hit wins:
#       1. its leading backtick token equals exactly one commit scope (`type(scope): ...`);
#       2. the names it cites (backticked spans, GO-/CVE- ids) appear in exactly one
#          commit's added lines or message (the highest distinct-name count, no tie);
#       3. its own words (5+ letters) overlap one commit message clearly more than the
#          others (at least 3 shared words and 2 more than the runner-up).
#     Tiers 2 and 3 print a MATCH line to stderr so the choice can be audited.
#     No unique match falls back to stamping ALL of the release's refs and prints the
#     bullet to stderr as REVIEW.
#   - A bot author (gh reports "app/<name>" or "<name>[bot]") links to github.com/apps/<name>.
#
# Environment for the author lookup: gh needs a token with pull-requests: read; without
# one the bullet is stamped with the commit and PR only, and a warning goes to stderr.
#
# Output: rewrites CHANGELOG in place; MATCH and REVIEW lines go to stderr.
set -Eeuo pipefail

version="${1:?usage: stamp-changelog-refs.sh <bare-version>}"
CHANGELOG="${CHANGELOG:-CHANGELOG.md}"
tag="v${version}"

if [ ! -f "$CHANGELOG" ]; then
  printf 'stamp-changelog-refs: CHANGELOG not found at %s\n' "$CHANGELOG" >&2
  exit 1
fi

# Normally the tag must already exist (we're backfilling a published release). The one
# exception is release.yml's own `release` job: it stamps CHANGELOG.md in its ephemeral
# checkout *before* the tag is created, so the body it builds from --from-existing carries
# refs. STAMP_HEAD_REF opts into that: point the range end at a ref that DOES exist (HEAD)
# instead of the not-yet-created tag. Unset/empty leaves today's exact behavior untouched.
range_end="$tag"
head_override=false
if ! git rev-parse "$tag" >/dev/null 2>&1; then
  if [ -n "${STAMP_HEAD_REF:-}" ] && git rev-parse "${STAMP_HEAD_REF}" >/dev/null 2>&1; then
    range_end="$STAMP_HEAD_REF"
    head_override=true
  else
    printf 'stamp-changelog-refs: tag %s not found\n' "$tag" >&2
    exit 1
  fi
fi

# Handles both "git@github.com:owner/repo.git" and "https://github.com/owner/repo.git" —
# take the last two "/"-or-":"-delimited fields, whichever separator the URL used.
if [ -z "${REPO_SLUG:-}" ]; then
  origin_url="$(git remote get-url origin 2>/dev/null || true)"
  origin_url="${origin_url%.git}"
  REPO_SLUG="$(printf '%s' "$origin_url" | awk -F'[/:]' '{print $(NF-1)"/"$NF}')"
fi

# --- Resolve the previous tag, and the commit range for this release --------
all_tags="$(git tag --sort=v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$')"
if [ "$head_override" = true ]; then
  # $tag doesn't exist in $all_tags yet (we're stamping pre-tag-creation) — the current
  # latest real tag is this release's predecessor by definition, so grep -B1 -x "$tag"
  # (which needs $tag present in the list to find its neighbor) can't be used here.
  prev_tag="$(printf '%s\n' "$all_tags" | tail -1)"
else
  prev_tag="$(printf '%s\n' "$all_tags" | grep -B1 -F -x "$tag" | head -1)"
  [ "$prev_tag" = "$tag" ] && prev_tag=""
fi

if [ -n "$prev_tag" ]; then
  range="${prev_tag}..${range_end}"
else
  range="$range_end"
fi

all_commit_lines=()
while IFS= read -r line || [ -n "$line" ]; do
  all_commit_lines+=("$line")
done < <(git log --reverse --pretty='%h%x09%s' "$range")

# `chore(release): ...` commits are tag/version housekeeping (e.g. consolidating a
# mis-cut version back into this one) — they never contribute CHANGELOG bullets, so
# including them in the ref pool would attach a meaningless link to every bullet in
# a release that otherwise has a clean single-commit or scope-matched attribution.
re_chore_release='^chore\(release\):'
commit_lines=()
# bash 3.2 (macOS system /bin/bash) throws "unbound variable" under `set -u`
# when expanding "${arr[@]}" on a declared-but-empty array — the
# "${arr[@]+"${arr[@]}"}" idiom expands to nothing instead of erroring in
# that case (e.g. a commit range with zero commits, such as the very first
# release), and is a no-op for non-empty arrays on any bash version.
for entry in "${all_commit_lines[@]+"${all_commit_lines[@]}"}"; do
  subject="${entry#*$'\t'}"
  if [[ "$subject" =~ $re_chore_release ]]; then
    continue
  fi
  commit_lines+=("$entry")
done
if [ "${#commit_lines[@]}" -eq 0 ]; then
  if [ "${#all_commit_lines[@]}" -gt 0 ]; then
    printf 'stamp-changelog-refs: every commit in %s is chore(release) — using the unfiltered pool\n' "$range" >&2
    commit_lines=("${all_commit_lines[@]}")
  else
    printf 'stamp-changelog-refs: no commits in %s — skipping stamping\n' "$range" >&2
    exit 0
  fi
fi

# bash 3.2 (macOS system /bin/bash) has no ${var,,} lowercasing operator, and its
# =~ parser mishandles literal parens written inline — both patterns below must
# be held in a variable first, then matched unquoted, or bash 3.2 throws a
# "syntax error in conditional expression" at parse time.
lc() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }
re_scope='^[a-z]+\(([^)]+)\)!?:'
re_pr='\(#([0-9]+)\)[[:space:]]*$'

declare -a c_hash c_scope c_pr c_ref
for entry in "${commit_lines[@]}"; do
  hash="${entry%%$'\t'*}"
  subject="${entry#*$'\t'}"

  scope=""
  if [[ "$subject" =~ $re_scope ]]; then
    scope="$(lc "${BASH_REMATCH[1]}")"
  fi

  pr=""
  if [[ "$subject" =~ $re_pr ]]; then
    pr="${BASH_REMATCH[1]}"
  fi

  author=""
  if [ -n "$pr" ]; then
    author="$(gh pr view "$pr" --repo "$REPO_SLUG" --json author -q .author.login 2>/dev/null || true)"
    if [ -z "$author" ]; then
      printf 'stamp-changelog-refs: no author for #%s (gh needs a token with pull-requests: read)\n' "$pr" >&2
    fi
  fi

  ref="([\`${hash}\`](https://github.com/${REPO_SLUG}/commit/${hash}))"
  if [ -n "$author" ]; then
    # gh reports a GitHub App as "app/<name>" (or "<name>[bot]"); neither is a profile URL.
    app_name=""
    case "$author" in
      app/*) app_name="${author#app/}" ;;
      *"[bot]") app_name="${author%\[bot\]}" ;;
    esac
    if [ -n "$app_name" ]; then
      ref="${ref} by [@${app_name}](https://github.com/apps/${app_name})"
    else
      ref="${ref} by [@${author}](https://github.com/${author})"
    fi
  fi
  [ -n "$pr" ] && ref="${ref} in [#${pr}](https://github.com/${REPO_SLUG}/pull/${pr})"

  c_hash+=("$hash")
  c_scope+=("$scope")
  c_pr+=("$pr")
  c_ref+=("$ref")
done

multi=false
[ "${#c_hash[@]}" -gt 1 ] && multi=true

all_refs_joined=""
for r in "${c_ref[@]}"; do
  all_refs_joined="${all_refs_joined:+${all_refs_joined} · }${r}"
done

# Each commit's added lines and full message, for matching a bullet to the change behind it.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
if [ "$multi" = true ]; then
  for i in "${!c_hash[@]}"; do
    git show --format= -U0 "${c_hash[$i]}" 2>/dev/null | grep '^+' | grep -v '^+++' > "$work/$i.diff" || true
    git show -s --format=%B "${c_hash[$i]}" > "$work/$i.msg" 2>/dev/null || true
  done
fi

# Names a bullet cites: backticked spans of 3 to 60 characters and advisory ids (GO-/CVE-).
bullet_tokens() {
  local rest="$1" tok
  local re_bt='`([^`]+)`' re_adv='(GO|CVE)-[0-9]{4}-[0-9]+'
  while [[ "$rest" =~ $re_bt ]]; do
    tok="${BASH_REMATCH[1]}"
    rest="${rest#*"${BASH_REMATCH[0]}"}"
    if [ "${#tok}" -ge 3 ] && [ "${#tok}" -le 60 ]; then
      printf '%s\n' "$tok"
    fi
  done
  rest="$1"
  while [[ "$rest" =~ $re_adv ]]; do
    tok="${BASH_REMATCH[0]}"
    rest="${rest#*"${BASH_REMATCH[0]}"}"
    printf '%s\n' "$tok"
  done
}

# Distinct lowercase words of 5+ letters, minus filler, for the commit-message overlap tier.
bullet_words() {
  printf '%s' "$1" | sed 's/`[^`]*`//g' | tr -cs 'a-zA-Z' '\n' | tr '[:upper:]' '[:lower:]' |
    awk 'length($0) >= 5 && !seen[$0]++ && $0 !~ /^(which|their|there|these|those|would|could|should|before|after|about|other|every|still|where|while|being|using|being|without|instead|because)$/'
}

# pick_winner MIN MARGIN SCORE... — index of the single highest score, only when it is at
# least MIN and beats the runner-up by MARGIN; prints nothing otherwise.
pick_winner() {
  local min="$1" margin="$2" best=-1 best_s=-1 second_s=-1 idx=0 s
  shift 2
  for s in "$@"; do
    if [ "$s" -gt "$best_s" ]; then
      second_s="$best_s"; best_s="$s"; best="$idx"
    elif [ "$s" -gt "$second_s" ]; then
      second_s="$s"
    fi
    idx=$((idx + 1))
  done
  if [ "$best_s" -ge "$min" ] && [ $((best_s - second_s)) -ge "$margin" ]; then
    printf '%s' "$best"
  fi
}

resolve_ref() {
  local first_line="$1" text="$2"
  if [ "$multi" = false ]; then
    printf '%s' "${c_ref[0]}"
    return
  fi

  # Tier 1: the bullet's leading backtick token equals exactly one commit scope.
  local token=""
  local re_token='`([a-zA-Z0-9_./-]+)`'
  if [[ "$first_line" =~ $re_token ]]; then
    token="$(lc "${BASH_REMATCH[1]}")"
  fi
  local match_idx=-1 match_count=0
  if [ -n "$token" ]; then
    for i in "${!c_scope[@]}"; do
      if [ -n "${c_scope[$i]}" ] && [ "${c_scope[$i]}" = "$token" ]; then
        match_idx=$i
        match_count=$((match_count + 1))
      fi
    done
  fi
  if [ "$match_count" -eq 1 ]; then
    printf '%s' "${c_ref[$match_idx]}"
    return
  fi

  # Tier 2: the names the bullet cites show up in exactly one commit's diff or message.
  local toks=() t s idx scores=() win
  while IFS= read -r t; do
    [ -n "$t" ] && toks+=("$t")
  done < <(bullet_tokens "$text")
  if [ "${#toks[@]}" -gt 0 ]; then
    for i in "${!c_hash[@]}"; do
      s=0
      for t in "${toks[@]}"; do
        if grep -q -F -- "$t" "$work/$i.diff" "$work/$i.msg" 2>/dev/null; then
          s=$((s + 1))
        fi
      done
      scores+=("$s")
    done
    win="$(pick_winner 1 1 "${scores[@]}")"
    if [ -n "$win" ]; then
      printf 'MATCH: %s: %s <- %s (cited names): %s\n' "$tag" "${c_hash[$win]}" "${scores[$win]}" "${first_line:0:60}" >&2
      printf '%s' "${c_ref[$win]}"
      return
    fi
  fi

  # Tier 3: the bullet's own words overlap one commit message clearly more than the others.
  local words=() w
  scores=()
  while IFS= read -r w; do
    [ -n "$w" ] && words+=("$w")
  done < <(bullet_words "$text")
  if [ "${#words[@]}" -gt 0 ]; then
    for i in "${!c_hash[@]}"; do
      s=0
      for w in "${words[@]}"; do
        if grep -q -i -w -- "$w" "$work/$i.msg" 2>/dev/null; then
          s=$((s + 1))
        fi
      done
      scores+=("$s")
    done
    win="$(pick_winner 3 2 "${scores[@]}")"
    if [ -n "$win" ]; then
      printf 'MATCH: %s: %s <- %s (message words): %s\n' "$tag" "${c_hash[$win]}" "${scores[$win]}" "${first_line:0:60}" >&2
      printf '%s' "${c_ref[$win]}"
      return
    fi
  fi

  printf 'REVIEW: %s: %s\n' "$tag" "${first_line:0:80}" >&2
  printf '%s' "$all_refs_joined"
}

# --- Locate the version's section in CHANGELOG -------------------------------
sec_start="$(grep -n -F "## [${version}]" "$CHANGELOG" | head -1 | cut -d: -f1)"
if [ -z "$sec_start" ]; then
  printf 'stamp-changelog-refs: no "## [%s]" heading in %s\n' "$version" "$CHANGELOG" >&2
  exit 1
fi
total_lines="$(wc -l < "$CHANGELOG" | tr -d ' ')"
sec_end="$(awk -v start="$sec_start" -v total="$total_lines" '
  /^## \[/ && NR > start { print NR - 1; found=1; exit }
  END { if (!found) print total }
' "$CHANGELOG")"

section_lines=()
while IFS= read -r line || [ -n "$line" ]; do
  section_lines+=("$line")
done < <(sed -n "${sec_start},${sec_end}p" "$CHANGELOG")

out=()
i=0
n=${#section_lines[@]}
while (( i < n )); do
  line="${section_lines[$i]}"
  if [[ "$line" =~ ^-\  ]]; then
    bullet=("$line")
    j=$((i + 1))
    while (( j < n )); do
      nxt="${section_lines[$j]}"
      if [[ "$nxt" =~ ^-\  ]] || [[ "$nxt" =~ ^#+\  ]] || [[ "$nxt" == "---" ]] || [[ -z "$nxt" ]]; then
        break
      fi
      bullet+=("$nxt")
      j=$((j + 1))
    done
    last_idx=$(( ${#bullet[@]} - 1 ))
    if [[ "${bullet[$last_idx]}" == *"/commit/"* ]]; then
      out+=("${bullet[@]}")
    else
      ref="$(resolve_ref "${bullet[0]}" "${bullet[*]}")"
      bullet[$last_idx]="${bullet[$last_idx]} ${ref}"
      out+=("${bullet[@]}")
    fi
    i=$j
  else
    out+=("$line")
    i=$((i + 1))
  fi
done

{
  head -n "$((sec_start - 1))" "$CHANGELOG"
  printf '%s\n' "${out[@]}"
  tail -n "+$((sec_end + 1))" "$CHANGELOG"
} > "${CHANGELOG}.tmp"
mv "${CHANGELOG}.tmp" "$CHANGELOG"
