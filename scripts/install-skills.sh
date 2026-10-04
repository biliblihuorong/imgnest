#!/usr/bin/env bash
# Re-install the 31 third-party skills into .claude/skills/.
# Default: the exact commits recorded in skills-lock.json (reproducible).
# --latest : take each repo's default branch instead (then review the diff
#            and update skills-lock.json / THIRD_PARTY_NOTICES.md).
# Project skills (imgnest-image-pipeline, lsky-api-compat) are never touched.
set -euo pipefail
LATEST=0; [[ "${1:-}" == "--latest" ]] && LATEST=1
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$ROOT/.claude/skills"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
mkdir -p "$DEST"

fetch() { # repo commit name:path...
  local repo=$1 commit=$2; shift 2
  local dir="$TMP/${repo//\//_}"
  git clone -q --filter=blob:none --no-checkout "https://github.com/$repo" "$dir"
  local ref=$commit; [[ $LATEST == 1 ]] && ref=$(git -C "$dir" rev-parse origin/HEAD)
  for item in "$@"; do
    local name=${item%%:*} path=${item#*:}
    git -C "$dir" checkout -q "$ref" -- "$path"
    rm -rf "$DEST/$name"; cp -r "$dir/$path" "$DEST/$name"
    echo "  $name  <- $repo@${ref:0:12}"
  done
}

fetch obra/superpowers 8ca22dba9a94f28898bbce59f2537ff4d87c747d \
  brainstorming:skills/brainstorming \
  writing-plans:skills/writing-plans \
  executing-plans:skills/executing-plans \
  test-driven-development:skills/test-driven-development \
  systematic-debugging:skills/systematic-debugging \
  verification-before-completion:skills/verification-before-completion \
  requesting-code-review:skills/requesting-code-review

fetch samber/cc-skills-golang 8e899e20ff0cd4dc524af3993e4c62d8ee8c5717 \
  golang-project-layout:skills/golang-project-layout \
  golang-code-style:skills/golang-code-style \
  golang-lint:skills/golang-lint \
  golang-error-handling:skills/golang-error-handling \
  golang-database:skills/golang-database \
  golang-concurrency:skills/golang-concurrency \
  golang-security:skills/golang-security \
  golang-testing:skills/golang-testing \
  golang-performance:skills/golang-performance \
  golang-swagger:skills/golang-swagger \
  golang-continuous-integration:skills/golang-continuous-integration

fetch vuejs-ai/skills c9d355ff23f654309dd02006be671859df0a134c \
  vue-best-practices:skills/vue-best-practices \
  vue-router-best-practices:skills/vue-router-best-practices \
  vue-pinia-best-practices:skills/vue-pinia-best-practices \
  vue-testing-best-practices:skills/vue-testing-best-practices

fetch antfu/skills e53a142a2420e8cd812cfe9ed0484ab01bc856aa \
  vite:skills/vite \
  vitest:skills/vitest \
  pnpm:skills/pnpm

fetch anthropics/skills 8a1541c4a3ffa5a20a5a91de0dcf3f0bab1d1ef4 \
  frontend-design:skills/frontend-design \
  webapp-testing:skills/webapp-testing \
  skill-creator:skills/skill-creator

fetch github/awesome-copilot 143a3d976b3c1603cc8932984d5e1f28501cb5fc \
  multi-stage-dockerfile:skills/multi-stage-dockerfile

fetch trailofbits/skills 82fe8226252622fa807643bdca1710901198553a \
  differential-review:plugins/differential-review/skills/differential-review \
  semgrep:plugins/static-analysis/skills/semgrep

echo "done: $(ls "$DEST" | wc -l) skills in $DEST"
