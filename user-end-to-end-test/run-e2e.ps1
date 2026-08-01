[CmdletBinding()]
param(
    [switch]$NoBuild,
    [switch]$KeepContainer
)

$ErrorActionPreference = 'Stop'

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..")
$envPath = Join-Path $scriptDir ".env"

if (-not (Test-Path $envPath)) {
    throw "Missing .env file at $envPath. Copy sample.env to .env and fill values first."
}

function Read-DotEnv {
    param([Parameter(Mandatory = $true)][string]$Path)

    $values = @{}
    Get-Content -Path $Path | ForEach-Object {
        $line = $_.Trim()
        if (-not $line -or $line.StartsWith('#')) {
            return
        }

        $parts = $line.Split('=', 2)
        if ($parts.Count -ne 2) {
            return
        }

        $key = $parts[0].Trim()
        $value = $parts[1].Trim()

        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }

        $values[$key] = $value
    }

    return $values
}

function Require-Env {
    param(
        [Parameter(Mandatory = $true)]$Map,
        [Parameter(Mandatory = $true)][string]$Name
    )

    if (-not $Map.ContainsKey($Name) -or [string]::IsNullOrWhiteSpace($Map[$Name])) {
        throw "Missing required env var '$Name' in $envPath"
    }

    return $Map[$Name]
}

function Test-DockerReady {
    $dockerCmd = Get-Command docker -ErrorAction SilentlyContinue
    if (-not $dockerCmd) {
        throw "Docker CLI not found. Install Docker Desktop and ensure 'docker' is available in PATH."
    }

    $null = docker version --format '{{.Server.Version}}' 2>$null
    if ($LASTEXITCODE -eq 0) {
        return
    }

    $msg = "Docker daemon is not reachable."
    if ($IsWindows) {
        $msg += " Start Docker Desktop first and wait until it shows 'Engine running'."
        $msg += " If Docker Desktop is already open, restart it or run: com.docker.cli -SwitchLinuxEngine"
    } else {
        $msg += " Start the Docker daemon/service and retry."
    }

    throw $msg
}

$envMap = Read-DotEnv -Path $envPath

$null = Require-Env -Map $envMap -Name 'CF_API_TOKEN'
$null = Require-Env -Map $envMap -Name 'CF_ACCOUNT_ID'
$to = Require-Env -Map $envMap -Name 'EMAIL'

$smtpHost = if ($envMap.ContainsKey('SMTP_HOST') -and -not [string]::IsNullOrWhiteSpace($envMap['SMTP_HOST'])) { $envMap['SMTP_HOST'] } else { '127.0.0.1' }
$smtpPort = if ($envMap.ContainsKey('SMTP_PORT') -and -not [string]::IsNullOrWhiteSpace($envMap['SMTP_PORT'])) { [int]$envMap['SMTP_PORT'] } else { 2525 }
$from = if ($envMap.ContainsKey('FROM_EMAIL') -and -not [string]::IsNullOrWhiteSpace($envMap['FROM_EMAIL'])) { $envMap['FROM_EMAIL'] } else { 'relay-test@example.local' }

$containerName = if ($envMap.ContainsKey('E2E_CONTAINER_NAME') -and -not [string]::IsNullOrWhiteSpace($envMap['E2E_CONTAINER_NAME'])) { $envMap['E2E_CONTAINER_NAME'] } else { 'cf-smtp-relay-e2e' }
$imageName = if ($envMap.ContainsKey('E2E_DOCKER_IMAGE') -and -not [string]::IsNullOrWhiteSpace($envMap['E2E_DOCKER_IMAGE'])) { $envMap['E2E_DOCKER_IMAGE'] } else { 'cf-smtp-relay:e2e' }

Test-DockerReady

if (-not $NoBuild) {
    Write-Host "Building Docker image '$imageName' from $repoRoot ..."
    docker build -t $imageName $repoRoot | Out-Host
}

# Remove old test container if it exists.
$existing = docker ps -aq -f "name=^$containerName$"
if ($existing) {
    docker rm -f $containerName | Out-Host
}

Write-Host "Starting container '$containerName' ..."
docker run -d --name $containerName --env-file $envPath -p "${smtpPort}:2525" $imageName | Out-Host

try {
    Write-Host "Waiting for SMTP listener on ${smtpHost}:${smtpPort} ..."
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $tcp = New-Object System.Net.Sockets.TcpClient
            $tcp.Connect($smtpHost, $smtpPort)
            $tcp.Close()
            $ready = $true
            break
        } catch {
            Start-Sleep -Seconds 1
        }
    }

    if (-not $ready) {
        throw "SMTP listener did not become ready in time."
    }

    $subject = "cf-smtp-relay e2e $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"
    $body = "End-to-end test via Send-MailMessage and Docker relay container '$containerName'."

    $sendArgs = @{
        SmtpServer = $smtpHost
        Port       = $smtpPort
        From       = $from
        To         = $to
        Subject    = $subject
        Body       = $body
    }

    $hasUser = $envMap.ContainsKey('SMTP_USER') -and -not [string]::IsNullOrWhiteSpace($envMap['SMTP_USER'])
    $hasPass = $envMap.ContainsKey('SMTP_PASS') -and -not [string]::IsNullOrWhiteSpace($envMap['SMTP_PASS'])
    if ($hasUser -and $hasPass) {
        $securePass = ConvertTo-SecureString $envMap['SMTP_PASS'] -AsPlainText -Force
        $sendArgs['Credential'] = New-Object System.Management.Automation.PSCredential($envMap['SMTP_USER'], $securePass)
    }

    Write-Host "Sending test email to '$to' via Send-MailMessage ..."
    Send-MailMessage @sendArgs

    Write-Host ""
    Write-Host "E2E test completed successfully."
    Write-Host "Container logs:"
    docker logs --tail 50 $containerName | Out-Host
}
finally {
    if (-not $KeepContainer) {
        Write-Host "Stopping and removing test container '$containerName' ..."
        docker rm -f $containerName | Out-Host
    } else {
        Write-Host "Leaving container '$containerName' running because -KeepContainer was set."
    }
}
