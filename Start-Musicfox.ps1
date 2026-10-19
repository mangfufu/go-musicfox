param(
    [switch]$Build,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$PlayerArgs
)

$ErrorActionPreference = 'Stop'
$projectRoot = $PSScriptRoot
$binaryPath = Join-Path $projectRoot 'bin\musicfox.exe'

if ($Build -or -not (Test-Path -LiteralPath $binaryPath)) {
    $goBinary = Join-Path $projectRoot '.tools\go\bin\go.exe'
    if (-not (Test-Path -LiteralPath $goBinary)) { $goBinary = 'go' }
    $compilerPath = Join-Path $projectRoot '.tools\mingw64\bin'
    $savedPath = $env:PATH
    $savedCGO = $env:CGO_ENABLED
    try {
        if (Test-Path -LiteralPath $compilerPath) { $env:PATH = "$compilerPath;$savedPath" }
        $env:CGO_ENABLED = '1'
        & $goBinary build -C $projectRoot -tags purego -ldflags '-s -w' -o $binaryPath ./cmd
        if ($LASTEXITCODE -ne 0) { throw 'Build failed. Go 1.26+ and a C compiler are required.' }
    } finally {
        $env:PATH = $savedPath
        $env:CGO_ENABLED = $savedCGO
    }
}

$savedRoot = $env:MUSICFOX_ROOT
try {
    if (-not $env:MUSICFOX_ROOT) { $env:MUSICFOX_ROOT = Join-Path $projectRoot '.local\user' }
    Write-Host 'F8: full-screen deck | F9: theme | Esc: back | Space: play/pause | Q: quit'
    & $binaryPath @PlayerArgs
    $playerExitCode = $LASTEXITCODE
} finally {
    $env:MUSICFOX_ROOT = $savedRoot
}
exit $playerExitCode
