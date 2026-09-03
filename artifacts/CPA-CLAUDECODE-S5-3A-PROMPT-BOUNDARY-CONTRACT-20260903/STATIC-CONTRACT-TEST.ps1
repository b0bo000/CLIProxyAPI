param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("BASELINE", "MODIFIED")]
    [string]$Mode,

    [Parameter(Mandatory = $true)]
    [string]$Root
)

$ErrorActionPreference = "Stop"
$rootPath = (Resolve-Path -LiteralPath $Root).Path
$contextFile = Join-Path $rootPath "sdk/cliproxy/executor/prompt_boundary.go"
$contextTestFile = Join-Path $rootPath "sdk/cliproxy/executor/prompt_boundary_test.go"
$helperFile = Join-Path $rootPath "internal/runtime/executor/helps/claude_prompt_state.go"
$executeFile = Join-Path $rootPath "internal/runtime/executor/claude_executor_execute.go"
$streamFile = Join-Path $rootPath "internal/runtime/executor/claude_executor_stream.go"

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw $Message
    }
}

$helper = Get-Content -Raw -LiteralPath $helperFile
$execute = Get-Content -Raw -LiteralPath $executeFile
$stream = Get-Content -Raw -LiteralPath $streamFile
$guardPattern = [regex]::Escape("helps.ResolveClaudePromptBoundary(ctx)")
$guardCount = ([regex]::Matches($execute, $guardPattern).Count + [regex]::Matches($stream, $guardPattern).Count)
$generatorPattern = 'uuid\.New|NewUUID|rand\.Read|CommitClaudePrompt|InsertClaudePromptID|GenerateClaudePrompt'

if ($Mode -eq "BASELINE") {
    Assert-True (-not (Test-Path -LiteralPath $contextFile)) "baseline unexpectedly contains prompt_boundary.go"
    Assert-True (-not (Test-Path -LiteralPath $contextTestFile)) "baseline unexpectedly contains prompt_boundary_test.go"
    Assert-True (-not $helper.Contains("func ResolveClaudePromptBoundary")) "baseline unexpectedly contains the boundary resolver"
    Assert-True ($guardCount -eq 0) "baseline executor guard count is $guardCount, want 0"
    Assert-True (-not [regex]::IsMatch($helper, $generatorPattern)) "baseline contains an unexpected prompt generator"
    Write-Output "CONTRACT_STATIC_OK MODE=BASELINE CONTEXT_API=0 RESOLVER=0 EXECUTOR_GUARDS=0 PROMPT_GENERATOR=0"
    exit 0
}

Assert-True (Test-Path -LiteralPath $contextFile) "modified tree is missing prompt_boundary.go"
Assert-True (Test-Path -LiteralPath $contextTestFile) "modified tree is missing prompt_boundary_test.go"
$contextSource = Get-Content -Raw -LiteralPath $contextFile
foreach ($kind in @('"absent"', '"new"', '"continue"', '"ambiguous"')) {
    Assert-True $contextSource.Contains($kind) "modified context contract is missing kind $kind"
}
Assert-True $contextSource.Contains("type promptBoundaryContextKey struct{}") "modified contract lacks a private context key"
Assert-True $helper.Contains("func ResolveClaudePromptBoundary") "modified tree is missing the boundary resolver"
Assert-True $helper.Contains("IsRequestScoped() bool") "modified resolver error is not request-scoped"
Assert-True ($guardCount -eq 2) "modified executor guard count is $guardCount, want 2"
Assert-True (-not [regex]::IsMatch(($contextSource + $helper), $generatorPattern)) "modified contract contains a prompt generator"

Write-Output "CONTRACT_STATIC_OK MODE=MODIFIED CONTEXT_API=1 RESOLVER=1 EXECUTOR_GUARDS=2 PROMPT_GENERATOR=0"
exit 0
