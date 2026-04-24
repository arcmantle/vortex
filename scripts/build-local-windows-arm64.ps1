param(
    [string]$Version = "dev",
    [switch]$SkipUI
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$toolchainVersion = '20260324'
$toolchainRoot = Join-Path $repoRoot '.tools\llvm-mingw-local'
$archiveName = "llvm-mingw-$toolchainVersion-ucrt-x86_64.zip"
$archivePath = Join-Path $toolchainRoot $archiveName
$toolchainDir = Join-Path $toolchainRoot "llvm-mingw-$toolchainVersion-ucrt-x86_64"
$toolchainBin = Join-Path $toolchainDir 'bin'
$clangPath = Join-Path $toolchainBin 'aarch64-w64-mingw32-clang.exe'
$releaseUrl = "https://github.com/mstorsjo/llvm-mingw/releases/download/$toolchainVersion/$archiveName"

if (-not (Test-Path $toolchainRoot)) {
    New-Item -ItemType Directory -Path $toolchainRoot -Force | Out-Null
}

if (-not (Test-Path $archivePath)) {
    Write-Host "Downloading llvm-mingw $toolchainVersion"
    Invoke-WebRequest -Uri $releaseUrl -OutFile $archivePath
}

if (-not (Test-Path $clangPath)) {
    if (Test-Path $toolchainDir) {
        Remove-Item $toolchainDir -Recurse -Force
    }
    Write-Host "Extracting llvm-mingw $toolchainVersion"
    tar -xf $archivePath -C $toolchainRoot
}

$env:PATH = "$toolchainBin;$env:PATH"
$env:CGO_ENABLED = '1'
$env:CC = 'aarch64-w64-mingw32-clang'
$env:CXX = 'aarch64-w64-mingw32-clang++'

& $clangPath --version

$buildArgs = @('.\build.go', '-local', '-os', 'windows', '-arch', 'arm64', '-version', $Version)
if (-not $SkipUI) {
    $buildArgs += '-ui'
}

go run @buildArgs