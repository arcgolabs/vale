[CmdletBinding()]
param(
    [int]$TimeoutSeconds = 120,
    [string]$ValeImage = "ghcr.io/arcgolabs/vale:latest",
    [string]$TargetArch = "amd64",
    [string]$OutputDir = "",
    [switch]$LocalBuild,
    [switch]$SkipPull,
    [switch]$Keep
)

$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true

$BenchDir = $PSScriptRoot
$Root = Resolve-Path (Join-Path $BenchDir "..")
$ComposeFile = Join-Path $BenchDir "docker-compose.gossip.yml"
$LocalComposeFile = Join-Path $BenchDir "docker-compose.gossip.local.yml"
$ImageContext = Join-Path $BenchDir ".tmp\gossip-smoke-image"
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $Stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $OutputDir = Join-Path $BenchDir "results\gossip-smoke-$Stamp"
}
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

if ($LocalBuild -and -not $PSBoundParameters.ContainsKey("ValeImage")) {
    $ValeImage = "vale-gossip-smoke:latest"
}
$env:VALE_IMAGE = $ValeImage

$ComposeArgs = @("-f", $ComposeFile)
if ($LocalBuild) {
    $ComposeArgs += @("-f", $LocalComposeFile)
}

function Build-SmokeImageContext {
    if (-not $LocalBuild) {
        return
    }

    Write-SmokeLog "building linux/$TargetArch valed smoke binary"
    New-Item -ItemType Directory -Force -Path $ImageContext | Out-Null
    Copy-Item -Force (Join-Path $BenchDir "Dockerfile.smoke") (Join-Path $ImageContext "Dockerfile")

    $PreviousGOOS = $env:GOOS
    $PreviousGOARCH = $env:GOARCH
    $PreviousCGO = $env:CGO_ENABLED
    try {
        $env:GOOS = "linux"
        $env:GOARCH = $TargetArch
        $env:CGO_ENABLED = "0"
        go build -C cmd -trimpath -ldflags="-s -w" -o (Join-Path $ImageContext "valed") .
    } finally {
        $env:GOOS = $PreviousGOOS
        $env:GOARCH = $PreviousGOARCH
        $env:CGO_ENABLED = $PreviousCGO
    }
}

function Write-SmokeLog {
    param([string]$Message)

    Write-Host "smoke: $Message"
}

function Wait-SmokeEndpoint {
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

function Wait-RaftGroupVoters {
    param(
        [string]$Group,
        [int]$Expected = 3
    )

    $Url = "http://127.0.0.1:28090/admin/cluster/peers?group=$Group"
    $Deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $Deadline) {
        try {
            $Content = (Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2).Content
            $Peers = @($Content | ConvertFrom-Json)
            $Voters = @($Peers | Where-Object { $_.suffrage -eq "Voter" })
            $Ids = @($Voters | ForEach-Object { $_.id })
            if ($Voters.Count -eq $Expected -and
                $Ids -contains "node-1" -and
                $Ids -contains "node-2" -and
                $Ids -contains "node-3") {
                $Content | Out-File -FilePath (Join-Path $OutputDir "peers-$Group.json") -Encoding utf8
                return
            }
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    throw "raft group $Group did not converge to $Expected voters"
}

Push-Location $Root
try {
    Build-SmokeImageContext

    Write-SmokeLog "starting gossip cluster compose stack"
    if ($LocalBuild) {
        docker compose @ComposeArgs up -d --build
    } else {
        if (-not $SkipPull) {
            Write-SmokeLog "pulling vale image $ValeImage"
            docker compose @ComposeArgs pull vale-1 vale-2 vale-3
        }
        docker compose @ComposeArgs up -d
    }

    Write-SmokeLog "waiting for proxy endpoint"
    Wait-SmokeEndpoint -Name "vale-gossip" -Url "http://127.0.0.1:18080/"

    foreach ($Group in @("metadata", "data", "certificates")) {
        Write-SmokeLog "waiting for $Group raft voters"
        Wait-RaftGroupVoters -Group $Group
    }

    docker compose @ComposeArgs ps | Out-File -FilePath (Join-Path $OutputDir "containers.txt") -Encoding utf8
    docker compose @ComposeArgs logs --no-color | Out-File -FilePath (Join-Path $OutputDir "logs.txt") -Encoding utf8
    Write-SmokeLog "gossip cluster converged"
} finally {
    if (-not $Keep) {
        Write-SmokeLog "stopping gossip cluster compose stack"
        docker compose @ComposeArgs down -v
    }
    Pop-Location
}
