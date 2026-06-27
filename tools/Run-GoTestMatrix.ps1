param(
    [string]$Root = ".",
    [int]$PerPackageTimeoutSeconds = 180,
    [int]$WaitBufferSeconds = 10,
    [string]$OutputDir = ".tmp/go-test-matrix",
    [switch]$StopOnFirstFailure
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Get-SafeFileName {
    param([string]$Value)

    return ($Value -replace '[^A-Za-z0-9._-]', '_')
}

Push-Location $Root
try {
    if (-not (Test-Path -LiteralPath $OutputDir)) {
        New-Item -ItemType Directory -Path $OutputDir | Out-Null
    }

    $packages = go list ./...
    $results = @()

    foreach ($package in $packages) {
        $safe = Get-SafeFileName -Value $package
        $stdoutLog = Join-Path $OutputDir ($safe + ".stdout.txt")
        $stderrLog = Join-Path $OutputDir ($safe + ".stderr.txt")
        $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()

        $process = Start-Process `
            -FilePath "go" `
            -ArgumentList @("test", $package, "-count=1", "-timeout", ($PerPackageTimeoutSeconds.ToString() + "s"), "-v") `
            -WorkingDirectory (Get-Location).Path `
            -NoNewWindow `
            -PassThru `
            -RedirectStandardOutput $stdoutLog `
            -RedirectStandardError $stderrLog

        $timedOut = -not $process.WaitForExit(($PerPackageTimeoutSeconds + $WaitBufferSeconds) * 1000)
        if ($timedOut) {
            try {
                Stop-Process -Id $process.Id -Force
            } catch {
            }
        } else {
            $process.WaitForExit()
        }

        $stopwatch.Stop()

        $exitCode = if ($timedOut) { $null } else { $process.ExitCode }
        $status = "FAIL"
        if ($timedOut) {
            $status = "TIMEOUT"
        } else {
            $combined = ""
            if (Test-Path -LiteralPath $stdoutLog) {
                $combined += Get-Content -LiteralPath $stdoutLog -Raw
            }
            if (Test-Path -LiteralPath $stderrLog) {
                $combined += Get-Content -LiteralPath $stderrLog -Raw
            }
            if ($combined -match "(?m)^FAIL\b") {
                $status = "FAIL"
            } elseif ($combined -match "(?m)^ok\s" -or $combined -match "\[no test files\]" -or $combined -match "(?m)^PASS\b") {
                $status = "PASS"
            }
        }

        $result = [pscustomobject]@{
            Package    = $package
            Status     = $status
            Seconds    = [math]::Round($stopwatch.Elapsed.TotalSeconds, 2)
            StdoutLog  = $stdoutLog
            StderrLog  = $stderrLog
            ExitCode   = $exitCode
            TimeoutSec = $PerPackageTimeoutSeconds
        }
        $results += $result

        Write-Host ($status + "`t" + $package + "`t" + $result.Seconds.ToString("0.00") + "s")

        if ($StopOnFirstFailure -and $status -ne "PASS") {
            break
        }
    }

    $summaryPath = Join-Path $OutputDir "summary.json"
    $results | ConvertTo-Json -Depth 4 | Set-Content -Path $summaryPath

    Write-Host ""
    $results | Format-Table -AutoSize
    Write-Host ""
    Write-Host ("Summary written to " + $summaryPath)
} finally {
    Pop-Location
}
