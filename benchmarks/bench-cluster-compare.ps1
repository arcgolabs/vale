[CmdletBinding()]
param(
    [string]$Duration = "15s",
    [string]$Warmup = "3s",
    [int]$Concurrency = 32,
    [int]$Repeat = 1,
    [int]$TimeoutSeconds = 90,
    [string]$ValeImage = "ghcr.io/arcgolabs/vale:latest",
    [string]$LogLevel = "info",
    [string]$OutputDir = "",
    [switch]$LocalBuild,
    [switch]$SkipPull,
    [switch]$Keep
)

$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

if ($Repeat -le 0) {
    throw "Repeat must be positive"
}

$BenchDir = $PSScriptRoot
$Root = Resolve-Path (Join-Path $BenchDir "..")
$ComposeFile = Join-Path $BenchDir "docker-compose.cluster.yml"
$LocalComposeFile = Join-Path $BenchDir "docker-compose.cluster.local.yml"
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $Stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $OutputDir = Join-Path $BenchDir "results\cluster-$Stamp"
}
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
if ($LocalBuild -and -not $PSBoundParameters.ContainsKey("ValeImage")) {
    $ValeImage = "vale-bench-vale:latest"
}
$env:VALE_IMAGE = $ValeImage

$ComposeArgs = @("-f", $ComposeFile)
if ($LocalBuild) {
    $ComposeArgs += @("-f", $LocalComposeFile)
}

function Write-BenchLog {
    param([string]$Message)

    Write-Host "bench: $Message"
}

function Wait-BenchEndpoint {
    param(
        [string]$Name,
        [string]$Url
    )

    $Deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $Deadline) {
        try {
            $Response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2
            if ($Response.StatusCode -ge 200 -and $Response.StatusCode -lt 500) {
                return
            }
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    throw "$Name did not become ready at $Url"
}

function Wait-ValeClusterLeader {
    param([string]$Url)

    $Deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $Deadline) {
        try {
            $Content = (Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2).Content
            if ($Content -match '"leader_ready"\s*:\s*true') {
                return
            }
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    throw "vale cluster did not elect a ready leader at $Url"
}

Push-Location $Root
try {
    Write-BenchLog "starting cluster compose stack"
    if ($LocalBuild) {
        docker compose @ComposeArgs up -d --build
    } else {
        if (-not $SkipPull) {
            Write-BenchLog "pulling vale image $ValeImage"
            docker compose @ComposeArgs pull vale-1 vale-2 vale-3
        }
        docker compose @ComposeArgs up -d
    }

    Write-BenchLog "waiting for vale cluster node"
    Wait-BenchEndpoint -Name "vale-cluster" -Url "http://127.0.0.1:18080/"
    Write-BenchLog "waiting for traefik single node"
    Wait-BenchEndpoint -Name "traefik-single" -Url "http://127.0.0.1:18081/"
    Write-BenchLog "waiting for vale raft leader"
    Wait-ValeClusterLeader -Url "http://127.0.0.1:28090/admin/cluster/status"

    Write-BenchLog "recording metadata in $OutputDir"
    docker compose @ComposeArgs images | Out-File -FilePath (Join-Path $OutputDir "images.txt") -Encoding utf8
    docker compose @ComposeArgs ps | Out-File -FilePath (Join-Path $OutputDir "containers.txt") -Encoding utf8
    Invoke-WebRequest -Uri "http://127.0.0.1:28090/admin/cluster/status" -UseBasicParsing |
        Select-Object -ExpandProperty Content |
        Out-File -FilePath (Join-Path $OutputDir "vale-cluster-status.json") -Encoding utf8

    for ($Run = 1; $Run -le $Repeat; $Run++) {
        $RunName = "run-{0:D2}" -f $Run
        $JsonPath = Join-Path $OutputDir "$RunName-proxybench.json"
        $MarkdownPath = Join-Path $OutputDir "$RunName-proxybench.md"
        if ($Repeat -eq 1) {
            $JsonPath = Join-Path $OutputDir "proxybench.json"
            $MarkdownPath = Join-Path $OutputDir "proxybench.md"
        }
        Write-BenchLog "running proxybench $RunName of $Repeat"
        go run ./benchmarks/cmd/proxybench `
            -duration $Duration `
            -warmup $Warmup `
            -concurrency $Concurrency `
            -log-level $LogLevel `
            -target "vale-cluster=http://127.0.0.1:18080,traefik-single=http://127.0.0.1:18081" `
            -json $JsonPath `
            -markdown $MarkdownPath
    }
} finally {
    if (-not $Keep) {
        Write-BenchLog "stopping cluster compose stack"
        docker compose @ComposeArgs down -v
    }
    Pop-Location
}
