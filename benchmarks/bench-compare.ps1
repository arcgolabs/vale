[CmdletBinding()]
param(
    [string]$Duration = "15s",
    [string]$Warmup = "3s",
    [int]$Concurrency = 32,
    [int]$TimeoutSeconds = 60,
    [string]$OutputDir = "",
    [switch]$Keep
)

$ErrorActionPreference = "Stop"

$BenchDir = $PSScriptRoot
$Root = Resolve-Path (Join-Path $BenchDir "..")
$ComposeFile = Join-Path $BenchDir "docker-compose.yml"
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $Stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $OutputDir = Join-Path $BenchDir "results\$Stamp"
}
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

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

Push-Location $Root
try {
    docker compose -f $ComposeFile up -d --build

    Wait-BenchEndpoint -Name "vale" -Url "http://127.0.0.1:18080/"
    Wait-BenchEndpoint -Name "traefik" -Url "http://127.0.0.1:18081/"
    Wait-BenchEndpoint -Name "caddy" -Url "http://127.0.0.1:18082/"

    docker compose -f $ComposeFile images | Out-File -FilePath (Join-Path $OutputDir "images.txt") -Encoding utf8

    go run ./benchmarks/cmd/proxybench `
        -duration $Duration `
        -warmup $Warmup `
        -concurrency $Concurrency `
        -target "vale=http://127.0.0.1:18080,traefik=http://127.0.0.1:18081,caddy=http://127.0.0.1:18082" `
        -json (Join-Path $OutputDir "proxybench.json") `
        -markdown (Join-Path $OutputDir "proxybench.md")
} finally {
    if (-not $Keep) {
        docker compose -f $ComposeFile down -v
    }
    Pop-Location
}
