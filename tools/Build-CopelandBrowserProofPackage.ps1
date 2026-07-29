[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$OutputDirectory,

    [string]$Version = "0.1.7"
)

$ErrorActionPreference = "Stop"

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$frontendRoot = Join-Path $repositoryRoot "manifest-frontend"
$outputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$packageRoot = Join-Path $outputDirectory "tspack-windows-amd64"
$nodeModules = Join-Path $packageRoot "node_modules"
$browserCache = Join-Path $packageRoot "playwright-browsers"

function Invoke-NativeCommand {
    param(
        [Parameter(Mandatory)]
        [string]$FilePath,

        [string[]]$Arguments = @()
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed ($LASTEXITCODE): $FilePath $($Arguments -join ' ')"
    }
}

Push-Location $repositoryRoot
try {
    Invoke-NativeCommand npm @("--prefix", $frontendRoot, "run", "build")
    Invoke-NativeCommand go @("run", "./tools/generate-embedded-bridges")

    if (Test-Path -LiteralPath $packageRoot) {
        Remove-Item -LiteralPath $packageRoot -Recurse -Force
    }

    New-Item -ItemType Directory -Force -Path $nodeModules, (Join-Path $packageRoot "tools"), $browserCache | Out-Null
    Invoke-NativeCommand go @(
        "build",
        "-tags", "tspack_embedded_bridges",
        "-trimpath",
        "-ldflags", "-X github.com/yuechen-li-dev/tspack/internal/version.Version=$Version",
        "-o", (Join-Path $packageRoot "tspack.exe"),
        "./cmd/tspack"
    )

    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "Prove-CopelandReactPlaywright.mjs") -Destination (Join-Path $packageRoot "tools\Prove-CopelandReactPlaywright.mjs")
    Copy-Item -LiteralPath (Join-Path $frontendRoot "node_modules\playwright") -Destination (Join-Path $nodeModules "playwright") -Recurse
    Copy-Item -LiteralPath (Join-Path $frontendRoot "node_modules\playwright-core") -Destination (Join-Path $nodeModules "playwright-core") -Recurse

    $sourceBrowserCache = Join-Path $env:LOCALAPPDATA "ms-playwright"
    $requiredBrowsers = @("chromium-1228", "chromium_headless_shell-1228", "ffmpeg-1011")
    foreach ($browser in $requiredBrowsers) {
        $source = Join-Path $sourceBrowserCache $browser
        if (-not (Test-Path -LiteralPath $source)) {
            throw "Required Playwright browser payload is unavailable: $source"
        }

        Copy-Item -LiteralPath $source -Destination (Join-Path $browserCache $browser) -Recurse
    }

    $manifest = [ordered]@{
        version = $Version
        platform = "windows-amd64"
        tspack = "tspack.exe"
        browserProof = "tools/Prove-CopelandReactPlaywright.mjs"
        embeddedManifestFrontend = $true
        playwright = "chromium-1228"
    } | ConvertTo-Json
    Set-Content -LiteralPath (Join-Path $packageRoot "package-manifest.json") -Value $manifest -NoNewline

    $archivePath = Join-Path $outputDirectory "TSPack.Tool.$Version-win-x64.zip"
    Remove-Item -LiteralPath $archivePath -Force -ErrorAction SilentlyContinue
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::Open($archivePath, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($file in Get-ChildItem -LiteralPath $packageRoot -File -Recurse | Sort-Object FullName) {
            $relativePath = [IO.Path]::GetRelativePath($outputDirectory, $file.FullName).Replace("\\", "/")
            $entry = $archive.CreateEntry($relativePath, [IO.Compression.CompressionLevel]::Optimal)
            $entry.LastWriteTime = [DateTimeOffset]::new(1980, 1, 1, 0, 0, 0, [TimeSpan]::Zero)
            $entryStream = $entry.Open()
            $fileStream = $file.OpenRead()
            try {
                $fileStream.CopyTo($entryStream)
            }
            finally {
                $fileStream.Dispose()
                $entryStream.Dispose()
            }
        }
    }
    finally {
        $archive.Dispose()
    }

    Write-Output $packageRoot
    Write-Output $archivePath
}
finally {
    Pop-Location
}
