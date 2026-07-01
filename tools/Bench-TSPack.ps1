param(
    [string]$Root = ".",
    [ValidateSet("react", "react-library", "static")]
    [string]$Template = "react",
    [ValidateSet("suite", "cold-update", "first-sync", "warm-sync", "dry-run-update")]
    [string[]]$Scenario = @("suite"),
    [int]$Runs = 1,
    [switch]$Profile,
    [switch]$UseGoRun,
    [int]$StoreJobs = 0,
    [string]$OutputDir = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
if ($PSVersionTable.PSVersion.Major -ge 7) {
    $PSNativeCommandUseErrorActionPreference = $false
}

function Ensure-Directory {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Path $Path | Out-Null
    }
}

function Get-SafeFileName {
    param([string]$Value)

    return ($Value -replace '[^A-Za-z0-9._-]', '_')
}

function Get-DirectoryFileBytes {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return 0
    }

    $measure = Get-ChildItem -LiteralPath $Path -Recurse -Force -File | Measure-Object -Property Length -Sum
    if ($null -eq $measure.Sum) {
        return 0
    }
    return [int64]$measure.Sum
}

function Get-TSPackInvocation {
    param(
        [string]$RepoRoot,
        [string]$BuildDir,
        [switch]$UseGoRunFlag
    )

    if ($UseGoRunFlag) {
        return @{
            FilePath = "go"
            BaseArgs = @("run", "./cmd/tspack")
        }
    }

    $binaryPath = Join-Path $BuildDir "tspack.exe"
    Write-Host ("Building TSPack binary: " + $binaryPath)
    & go build -o $binaryPath ./cmd/tspack
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
    return @{
        FilePath = $binaryPath
        BaseArgs = @()
    }
}

function Invoke-TSPack {
    param(
        [hashtable]$Invocation,
        [string]$RepoRoot,
        [string]$WorkingDir,
        [string[]]$CommandArgs,
        [string]$StdoutLog,
        [string]$StderrLog,
        [hashtable]$EnvOverrides
    )

    $previous = @{}
    foreach ($entry in $EnvOverrides.GetEnumerator()) {
        $previous[$entry.Key] = [Environment]::GetEnvironmentVariable($entry.Key, "Process")
        [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, "Process")
    }

    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $process = Start-Process `
            -FilePath $Invocation.FilePath `
            -ArgumentList @($Invocation.BaseArgs + $CommandArgs) `
            -WorkingDirectory $WorkingDir `
            -NoNewWindow `
            -PassThru `
            -RedirectStandardOutput $StdoutLog `
            -RedirectStandardError $StderrLog

        $process.WaitForExit()
        $exitCode = if ($null -eq $process.ExitCode) { 0 } else { [int]$process.ExitCode }
    } finally {
        $stopwatch.Stop()
        foreach ($entry in $EnvOverrides.GetEnumerator()) {
            [Environment]::SetEnvironmentVariable($entry.Key, $previous[$entry.Key], "Process")
        }
    }

    return @{
        ExitCode = $exitCode
        Duration = $stopwatch.Elapsed
    }
}

function Read-PerfJson {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return $null
    }
    return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
}

function New-TemplateProject {
    param(
        [hashtable]$Invocation,
        [string]$RepoRoot,
        [string]$ProjectRoot,
        [string]$TemplateName,
        [string]$ProjectName
    )

    Ensure-Directory -Path (Split-Path -Parent $ProjectRoot)

    $projectParent = Split-Path -Parent $ProjectRoot
    $stdoutLog = Join-Path $projectParent "init.stdout.txt"
    $stderrLog = Join-Path $projectParent "init.stderr.txt"
    Ensure-Directory -Path (Split-Path -Parent $stdoutLog)

    $args = @("init", "--template", $TemplateName, "--name", $ProjectName, "--root", $ProjectRoot)
    $result = Invoke-TSPack -Invocation $Invocation -RepoRoot $RepoRoot -WorkingDir $RepoRoot -CommandArgs $args -StdoutLog $stdoutLog -StderrLog $stderrLog -EnvOverrides @{}
    if ($result.ExitCode -ne 0) {
        throw "tspack init failed; see $stderrLog"
    }
}

function Invoke-BenchStep {
    param(
        [hashtable]$Invocation,
        [string]$RepoRoot,
        [string]$ScenarioName,
        [string]$RunLabel,
        [string]$ProjectRoot,
        [string]$StoreRoot,
        [string[]]$StepArgs,
        [string]$OutputRoot,
        [switch]$EnableProfile,
        [int]$StoreJobsValue
    )

    $scenarioDir = Join-Path $OutputRoot $RunLabel
    Ensure-Directory -Path $scenarioDir

    $safe = Get-SafeFileName -Value $ScenarioName
    $stdoutLog = Join-Path $scenarioDir ($safe + ".stdout.txt")
    $stderrLog = Join-Path $scenarioDir ($safe + ".stderr.txt")
    $perfJson = Join-Path $scenarioDir ($safe + ".perf.json")

    $envOverrides = @{
        "TSPACK_TRACE_PERF_JSON" = $perfJson
    }
    if ($StoreJobsValue -gt 0) {
        $envOverrides["TSPACK_STORE_JOBS"] = $StoreJobsValue.ToString()
    }
    if ($EnableProfile) {
        $profileDir = Join-Path $scenarioDir "profiles"
        Ensure-Directory -Path $profileDir
        $envOverrides["TSPACK_CPU_PROFILE"] = Join-Path $profileDir ($safe + ".cpu.pprof")
        $envOverrides["TSPACK_MEM_PROFILE"] = Join-Path $profileDir ($safe + ".mem.pprof")
    }

    $args = @($StepArgs + @("--root", $ProjectRoot, "--store", $StoreRoot))
    $result = Invoke-TSPack -Invocation $Invocation -RepoRoot $RepoRoot -WorkingDir $RepoRoot -CommandArgs $args -StdoutLog $stdoutLog -StderrLog $stderrLog -EnvOverrides $envOverrides
    if ($result.ExitCode -ne 0) {
        throw ("scenario failed: " + $ScenarioName + " (see " + $stderrLog + ")")
    }

    $perf = Read-PerfJson -Path $perfJson
    $nodeModulesBytes = Get-DirectoryFileBytes -Path (Join-Path $ProjectRoot "node_modules")

    return [pscustomobject]@{
        Run                = $RunLabel
        Scenario           = $ScenarioName
        Seconds            = [math]::Round($result.Duration.TotalSeconds, 2)
        PerfJson           = $perfJson
        StdoutLog          = $stdoutLog
        StderrLog          = $stderrLog
        NodeModulesBytes   = $nodeModulesBytes
        MetadataRequests   = if ($null -ne $perf) { [int64]$perf.counters.metadataRequests } else { 0 }
        MetadataCacheHits  = if ($null -ne $perf) { [int64]$perf.counters.metadataCacheHits } else { 0 }
        TarballRequests    = if ($null -ne $perf) { [int64]$perf.counters.tarballRequests } else { 0 }
        CapturedArtifacts  = if ($null -ne $perf) { [int64]$perf.counters.artifactsCaptured } else { 0 }
        StoreNeed          = if ($null -ne $perf) { [int64]$perf.counters.artifactsNeedingStorePopulation } else { 0 }
        StoreSkipped       = if ($null -ne $perf) { [int64]$perf.counters.storePopulationSkipped } else { 0 }
        StoreFetched       = if ($null -ne $perf) { [int64]$perf.counters.storePopulationFetched } else { 0 }
        SyncHydrateSkipped = if ($null -ne $perf) { [int64]$perf.counters.syncHydrationSkipped } else { 0 }
        SyncHydrateFetched = if ($null -ne $perf) { [int64]$perf.counters.syncHydrationFetched } else { 0 }
        MaterializedPkgs   = if ($null -ne $perf) { [int64]$perf.counters.materializedPackages } else { 0 }
        MaterializedFiles  = if ($null -ne $perf) { [int64]$perf.counters.materializedFiles } else { 0 }
        Hardlinks          = if ($null -ne $perf) { [int64]$perf.counters.hardlinkCount } else { 0 }
        Copies             = if ($null -ne $perf) { [int64]$perf.counters.copyFallbackCount } else { 0 }
        LogicalBytes       = if ($null -ne $perf) { [int64]$perf.counters.logicalBytesMaterialized } else { 0 }
        CopiedBytes        = if ($null -ne $perf) { [int64]$perf.counters.bytesCopied } else { 0 }
        CPUProfile         = if ($EnableProfile) { Join-Path (Join-Path $scenarioDir "profiles") ($safe + ".cpu.pprof") } else { "" }
        MemProfile         = if ($EnableProfile) { Join-Path (Join-Path $scenarioDir "profiles") ($safe + ".mem.pprof") } else { "" }
    }
}

function Expand-ScenarioSet {
    param([string[]]$Names)

    $expanded = New-Object System.Collections.Generic.List[string]
    foreach ($name in $Names) {
        if ($name -eq "suite") {
            foreach ($item in @("cold-update", "first-sync", "warm-sync", "dry-run-update")) {
                if (-not $expanded.Contains($item)) {
                    $expanded.Add($item)
                }
            }
            continue
        }
        if (-not $expanded.Contains($name)) {
            $expanded.Add($name)
        }
    }
    return $expanded
}

$repoRoot = (Resolve-Path -LiteralPath $Root).Path
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $repoRoot ("dist/bench/" + $timestamp)
}

Ensure-Directory -Path $OutputDir
$buildDir = Join-Path $OutputDir "bin"
Ensure-Directory -Path $buildDir

Push-Location $repoRoot
try {
    $invocation = Get-TSPackInvocation -RepoRoot $repoRoot -BuildDir $buildDir -UseGoRunFlag:$UseGoRun
    $scenarioNames = Expand-ScenarioSet -Names $Scenario
    $results = New-Object System.Collections.Generic.List[object]

    for ($runIndex = 1; $runIndex -le $Runs; $runIndex++) {
        $runLabel = "run-" + $runIndex.ToString("00")
        $runDir = Join-Path $OutputDir $runLabel
        Ensure-Directory -Path $runDir

        if ($scenarioNames -contains "cold-update" -or $scenarioNames -contains "first-sync" -or $scenarioNames -contains "warm-sync") {
            $projectRoot = Join-Path $runDir "react-project"
            $storeRoot = Join-Path $runDir "react-store"
            New-TemplateProject -Invocation $invocation -RepoRoot $repoRoot -ProjectRoot $projectRoot -TemplateName $Template -ProjectName ("bench-" + $runLabel)

            if ($scenarioNames -contains "cold-update") {
                $results.Add((Invoke-BenchStep -Invocation $invocation -RepoRoot $repoRoot -ScenarioName "cold-update" -RunLabel $runLabel -ProjectRoot $projectRoot -StoreRoot $storeRoot -StepArgs @("update") -OutputRoot $OutputDir -EnableProfile:$Profile -StoreJobsValue $StoreJobs))
            } else {
                $null = Invoke-BenchStep -Invocation $invocation -RepoRoot $repoRoot -ScenarioName "bootstrap-update" -RunLabel $runLabel -ProjectRoot $projectRoot -StoreRoot $storeRoot -StepArgs @("update") -OutputRoot $OutputDir -EnableProfile:$false -StoreJobsValue $StoreJobs
            }

            if ($scenarioNames -contains "first-sync") {
                $results.Add((Invoke-BenchStep -Invocation $invocation -RepoRoot $repoRoot -ScenarioName "first-sync" -RunLabel $runLabel -ProjectRoot $projectRoot -StoreRoot $storeRoot -StepArgs @("sync") -OutputRoot $OutputDir -EnableProfile:$Profile -StoreJobsValue $StoreJobs))
            } else {
                $null = Invoke-BenchStep -Invocation $invocation -RepoRoot $repoRoot -ScenarioName "bootstrap-sync" -RunLabel $runLabel -ProjectRoot $projectRoot -StoreRoot $storeRoot -StepArgs @("sync") -OutputRoot $OutputDir -EnableProfile:$false -StoreJobsValue $StoreJobs
            }

            if ($scenarioNames -contains "warm-sync") {
                $results.Add((Invoke-BenchStep -Invocation $invocation -RepoRoot $repoRoot -ScenarioName "warm-sync" -RunLabel $runLabel -ProjectRoot $projectRoot -StoreRoot $storeRoot -StepArgs @("sync") -OutputRoot $OutputDir -EnableProfile:$Profile -StoreJobsValue $StoreJobs))
            }
        }

        if ($scenarioNames -contains "dry-run-update") {
            $dryProjectRoot = Join-Path $runDir "dry-run-project"
            $dryStoreRoot = Join-Path $runDir "dry-run-store"
            New-TemplateProject -Invocation $invocation -RepoRoot $repoRoot -ProjectRoot $dryProjectRoot -TemplateName $Template -ProjectName ("dry-" + $runLabel)
            $results.Add((Invoke-BenchStep -Invocation $invocation -RepoRoot $repoRoot -ScenarioName "dry-run-update" -RunLabel $runLabel -ProjectRoot $dryProjectRoot -StoreRoot $dryStoreRoot -StepArgs @("update", "--dry-run") -OutputRoot $OutputDir -EnableProfile:$Profile -StoreJobsValue $StoreJobs))
        }
    }

    $summaryPath = Join-Path $OutputDir "summary.json"
    $results | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath

    Write-Host ""
    $results |
        Select-Object Run, Scenario, Seconds, MetadataRequests, MetadataCacheHits, TarballRequests, CapturedArtifacts, StoreNeed, StoreSkipped, StoreFetched, SyncHydrateSkipped, SyncHydrateFetched, Hardlinks, Copies, NodeModulesBytes |
        Format-Table -AutoSize
    Write-Host ""
    Write-Host ("Summary written to " + $summaryPath)
    Write-Host ("Artifacts written to " + $OutputDir)
    Write-Host ("Store isolation: each run uses an explicit --store path under " + $OutputDir)
    if ($Profile) {
        Write-Host "Profiles: CPU and heap pprof files were written beside each measured scenario."
    }
    Write-Host "Note: tspack bench / *.benchmark.tsx is complementary for TypeScript-side microbenchmarks, but this harness measures Go command phases and pprof output."
} finally {
    Pop-Location
}
