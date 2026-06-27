$ErrorActionPreference = 'Stop'

function Invoke-Step {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Label,
    [Parameter(Mandatory = $true)]
    [scriptblock]$Action
  )

  Write-Host "==> $Label"
  & $Action
}

function Get-CodeCliPath {
  $commands = @(
    'code',
    'code.cmd',
    'code-insiders',
    'code-insiders.cmd'
  )

  foreach ($commandName in $commands) {
    $command = Get-Command $commandName -ErrorAction SilentlyContinue
    if ($command) {
      return $command.Source
    }
  }

  $candidates = @(
    (Join-Path $env:LOCALAPPDATA 'Programs\Microsoft VS Code\bin\code.cmd'),
    (Join-Path $env:LOCALAPPDATA 'Programs\Microsoft VS Code Insiders\bin\code-insiders.cmd'),
    'C:\Program Files\Microsoft VS Code\bin\code.cmd',
    'C:\Program Files\Microsoft VS Code Insiders\bin\code-insiders.cmd'
  )

  foreach ($candidate in $candidates) {
    if ($candidate -and (Test-Path $candidate)) {
      return $candidate
    }
  }

  return $null
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$extensionDir = Join-Path $repoRoot 'extensions\tspack-vscode'
$packageLockPath = Join-Path $extensionDir 'package-lock.json'
$nodeModulesPath = Join-Path $extensionDir 'node_modules'
$vsixPath = Join-Path $extensionDir 'dist\tspack-vscode.vsix'

if (-not (Test-Path $nodeModulesPath)) {
  if (Test-Path $packageLockPath) {
    Invoke-Step 'Installing extension dependencies with npm ci' {
      npm --prefix $extensionDir ci
    }
  } else {
    Invoke-Step 'Installing extension dependencies with npm install' {
      npm --prefix $extensionDir install
    }
  }
} else {
  Write-Host '==> Reusing existing extension node_modules'
}

Invoke-Step 'Compiling VS Code extension' {
  npm --prefix $extensionDir run compile
}

Invoke-Step 'Running VS Code extension tests' {
  npm --prefix $extensionDir test
}

Invoke-Step 'Packaging VS Code extension VSIX' {
  npm --prefix $extensionDir run package
}

if (-not (Test-Path $vsixPath)) {
  throw "VSIX was not created at $vsixPath"
}

$codeCliPath = Get-CodeCliPath
if (-not $codeCliPath) {
  Write-Warning "VS Code CLI was not found. Install manually from VSIX: $vsixPath"
  Write-Host 'Open VS Code -> Extensions -> ... -> Install from VSIX...'
  exit 0
}

Invoke-Step "Installing VSIX with $codeCliPath" {
  & $codeCliPath --install-extension $vsixPath --force
}

Write-Host '==> Installed extension list match'
& $codeCliPath --list-extensions | Select-String -Pattern 'tspack' -CaseSensitive:$false

Write-Host "VSIX path: $vsixPath"
