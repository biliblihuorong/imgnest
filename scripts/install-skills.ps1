# Re-install the 31 third-party skills into .claude/skills/ (Windows).
# Default: exact commits from skills-lock.json. Use -Latest for each repo's default branch.
# Project skills (imgnest-image-pipeline, lsky-api-compat) are never touched.
param([switch]$Latest)
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Dest = Join-Path $Root ".claude/skills"
$Tmp = Join-Path ([IO.Path]::GetTempPath()) ("skills-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $Dest, $Tmp | Out-Null
function Fetch($Repo, $Commit, $Items) {
  $dir = Join-Path $Tmp ($Repo -replace "/", "_")
  git clone -q --filter=blob:none --no-checkout "https://github.com/$Repo" $dir
  $ref = $Commit
  if ($Latest) { $ref = (git -C $dir rev-parse origin/HEAD).Trim() }
  foreach ($it in $Items) {
    $name, $path = $it -split ":", 2
    git -C $dir checkout -q $ref -- $path
    $target = Join-Path $Dest $name
    if (Test-Path $target) { Remove-Item -Recurse -Force $target }
    Copy-Item -Recurse (Join-Path $dir $path) $target
    Write-Host "  $name  <- $Repo@$($ref.Substring(0,12))"
  }
}
try {
  Fetch "obra/superpowers" "8ca22dba9a94f28898bbce59f2537ff4d87c747d" @("brainstorming:skills/brainstorming", "writing-plans:skills/writing-plans", "executing-plans:skills/executing-plans", "test-driven-development:skills/test-driven-development", "systematic-debugging:skills/systematic-debugging", "verification-before-completion:skills/verification-before-completion", "requesting-code-review:skills/requesting-code-review")
  Fetch "samber/cc-skills-golang" "8e899e20ff0cd4dc524af3993e4c62d8ee8c5717" @("golang-project-layout:skills/golang-project-layout", "golang-code-style:skills/golang-code-style", "golang-lint:skills/golang-lint", "golang-error-handling:skills/golang-error-handling", "golang-database:skills/golang-database", "golang-concurrency:skills/golang-concurrency", "golang-security:skills/golang-security", "golang-testing:skills/golang-testing", "golang-performance:skills/golang-performance", "golang-swagger:skills/golang-swagger", "golang-continuous-integration:skills/golang-continuous-integration")
  Fetch "vuejs-ai/skills" "c9d355ff23f654309dd02006be671859df0a134c" @("vue-best-practices:skills/vue-best-practices", "vue-router-best-practices:skills/vue-router-best-practices", "vue-pinia-best-practices:skills/vue-pinia-best-practices", "vue-testing-best-practices:skills/vue-testing-best-practices")
  Fetch "antfu/skills" "e53a142a2420e8cd812cfe9ed0484ab01bc856aa" @("vite:skills/vite", "vitest:skills/vitest", "pnpm:skills/pnpm")
  Fetch "anthropics/skills" "8a1541c4a3ffa5a20a5a91de0dcf3f0bab1d1ef4" @("frontend-design:skills/frontend-design", "webapp-testing:skills/webapp-testing", "skill-creator:skills/skill-creator")
  Fetch "github/awesome-copilot" "143a3d976b3c1603cc8932984d5e1f28501cb5fc" @("multi-stage-dockerfile:skills/multi-stage-dockerfile")
  Fetch "trailofbits/skills" "82fe8226252622fa807643bdca1710901198553a" @("differential-review:plugins/differential-review/skills/differential-review", "semgrep:plugins/static-analysis/skills/semgrep")
} finally { Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue }
Write-Host "done: $((Get-ChildItem $Dest -Directory).Count) skills in $Dest"
