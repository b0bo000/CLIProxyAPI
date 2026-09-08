param(
    [string]$Manifest = "docs/deployment/CLAUDE-COMPATIBILITY-RELEASE.md"
)

$ErrorActionPreference = "Stop"

$branch = (git rev-parse --abbrev-ref HEAD).Trim()
if ($branch -notlike "release/*") {
    throw "release preflight requires a release/* branch; current branch is '$branch'"
}

$dirty = @(git status --porcelain)
if ($dirty.Count -ne 0) {
    throw "release preflight requires a clean worktree"
}

if (-not (Test-Path -LiteralPath $Manifest -PathType Leaf)) {
    throw "release manifest not found: $Manifest"
}

$required = @(
    "docs/change-cards/CC-20260908-S6D-COMPACT-529-RECOVERY.md",
    "docs/change-cards/CC-20260908-S7-TLS-SESSION-POLICY.md",
    "docs/change-cards/CC-20260908-S8-RELEASE-INTEGRATION.md"
)
foreach ($path in $required) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "required change card not found: $path"
    }
}

Write-Output "RELEASE_PREFLIGHT=PASS"
Write-Output "BRANCH=$branch"
Write-Output "MANIFEST=$Manifest"
