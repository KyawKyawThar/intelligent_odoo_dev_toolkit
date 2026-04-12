# install-agent.ps1 — OdooDevTools Agent installer (Windows)
#
# Usage (copy from your dashboard — run in an elevated PowerShell window):
#   $env:AGENT_CLOUD_URL="wss://YOUR_API_DOMAIN/api/v1/agent/ws"
#   $env:AGENT_REGISTRATION_TOKEN="reg_YOUR_TOKEN_HERE"
#   irm https://YOUR_API_DOMAIN/install.ps1 | iex
#
# Optional environment variables (set before running):
#   $env:AGENT_VERSION          — pin a specific release tag (default: latest)
#   $env:INSTALL_DIR            — binary directory  (default: C:\Program Files\OdooDevTools)
#   $env:CONFIG_DIR             — config directory  (default: C:\ProgramData\OdooDevTools)
#   $env:ODOO_URL               — Odoo server URL
#   $env:ODOO_DB                — Odoo database name
#   $env:ODOO_ADMIN_USER        — Odoo admin username
#   $env:ODOO_ADMIN_PASSWORD    — Odoo admin password
#
#Requires -Version 5.1
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ─── Config ───────────────────────────────────────────────────────────────────
$BinaryName   = "odoodevtools-agent.exe"
$ServiceName  = "OdooDevToolsAgent"
$InstallDir   = if ($env:INSTALL_DIR)  { $env:INSTALL_DIR }  else { "C:\Program Files\OdooDevTools" }
$ConfigDir    = if ($env:CONFIG_DIR)   { $env:CONFIG_DIR }   else { "$env:ProgramData\OdooDevTools" }
$ConfigFile   = Join-Path $ConfigDir "agent.env"
$LogFile      = Join-Path $ConfigDir "agent.log"

# ─── Colours ──────────────────────────────────────────────────────────────────
function Write-Info  { param($m) Write-Host "[INFO]  $m" -ForegroundColor Green  }
function Write-Warn  { param($m) Write-Host "[WARN]  $m" -ForegroundColor Yellow }
function Write-Step  { param($m) Write-Host "`n> $m"     -ForegroundColor Cyan   }
function Write-Fatal { param($m) Write-Host "[ERROR] $m" -ForegroundColor Red; exit 1 }

# ─── Required parameters ──────────────────────────────────────────────────────
$AgentCloudUrl         = $env:AGENT_CLOUD_URL
$AgentRegistrationToken = $env:AGENT_REGISTRATION_TOKEN

if (-not $AgentCloudUrl)          { Write-Fatal "AGENT_CLOUD_URL is required (e.g. wss://api.yourdomain.com/api/v1/agent/ws)" }
if (-not $AgentRegistrationToken) { Write-Fatal "AGENT_REGISTRATION_TOKEN is required (copy from the dashboard)" }

# Derive HTTPS base URL: wss://host/path → https://host
$AgentApiUrl = $AgentCloudUrl -replace '^wss?://([^/]+).*', 'https://$1'

# ─── Detect architecture ──────────────────────────────────────────────────────
Write-Step "Detecting platform"
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64"  { "amd64"  }
    "ARM64"  { "arm64"  }
    default  { Write-Fatal "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}
$Platform = "windows-$Arch"
Write-Info "Platform: $Platform"

# ─── Resolve version ──────────────────────────────────────────────────────────
Write-Step "Resolving agent version"
$AgentVersion = $env:AGENT_VERSION
if (-not $AgentVersion) {
    try {
        $resp = Invoke-RestMethod "$AgentApiUrl/api/v1/agent/version"
        $AgentVersion = $resp.latest
    } catch {
        Write-Fatal "Could not fetch latest agent version from $AgentApiUrl. Check your internet connection."
    }
}
Write-Info "Installing version: $AgentVersion"

$BinaryFilename = "odoodevtools-agent-$Platform.exe"
$DownloadUrl    = "$AgentApiUrl/api/v1/agent/download?version=$AgentVersion&platform=$Platform"
$ChecksumUrl    = "$AgentApiUrl/api/v1/agent/checksums?version=$AgentVersion"

# ─── Download binary ──────────────────────────────────────────────────────────
Write-Step "Downloading agent binary"
$TmpDir = Join-Path $env:TEMP "odoodevtools-install-$([System.IO.Path]::GetRandomFileName())"
New-Item -ItemType Directory -Path $TmpDir | Out-Null

$TmpBinary = Join-Path $TmpDir $BinaryFilename
Write-Info "URL: $DownloadUrl"
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $TmpBinary -UseBasicParsing
} catch {
    Write-Fatal "Download failed. Is version $AgentVersion available? Check your dashboard."
}

# ─── Verify checksum ──────────────────────────────────────────────────────────
Write-Step "Verifying checksum"
$TmpChecksums = Join-Path $TmpDir "checksums.txt"
try {
    Invoke-WebRequest -Uri $ChecksumUrl -OutFile $TmpChecksums -UseBasicParsing
} catch {
    Write-Warn "checksums.txt not found — skipping verification"
}

if (Test-Path $TmpChecksums) {
    $Expected = (Get-Content $TmpChecksums | Where-Object { $_ -match $BinaryFilename }) -split '\s+' | Select-Object -First 1
    if ($Expected) {
        $Actual = (Get-FileHash $TmpBinary -Algorithm SHA256).Hash.ToLower()
        if ($Expected.ToLower() -eq $Actual) {
            Write-Info "Checksum OK: $Actual"
        } else {
            Remove-Item $TmpDir -Recurse -Force
            Write-Fatal "Checksum mismatch!`n  Expected: $Expected`n  Actual:   $Actual`nDownload may be corrupted."
        }
    } else {
        Write-Warn "No checksum entry for $BinaryFilename — skipping"
    }
}

# ─── Install binary ───────────────────────────────────────────────────────────
Write-Step "Installing binary to $InstallDir\$BinaryName"
if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir | Out-Null }
Copy-Item $TmpBinary (Join-Path $InstallDir $BinaryName) -Force
Remove-Item $TmpDir -Recurse -Force
Write-Info "Binary installed: $InstallDir\$BinaryName"

# Add install dir to system PATH if not already present
$SysPath = [System.Environment]::GetEnvironmentVariable("Path", "Machine")
if ($SysPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable("Path", "$SysPath;$InstallDir", "Machine")
    Write-Info "Added $InstallDir to system PATH"
}

# ─── Write config file ────────────────────────────────────────────────────────
Write-Step "Writing config to $ConfigFile"
if (-not (Test-Path $ConfigDir)) { New-Item -ItemType Directory -Path $ConfigDir | Out-Null }

$OdooUrl      = if ($env:ODOO_URL)           { $env:ODOO_URL }           else { "http://localhost:8069" }
$OdooDb       = if ($env:ODOO_DB)            { $env:ODOO_DB }            else { "odoo" }
$OdooUser     = if ($env:ODOO_ADMIN_USER)    { $env:ODOO_ADMIN_USER }    else { "admin" }
$OdooPassword = if ($env:ODOO_ADMIN_PASSWORD){ $env:ODOO_ADMIN_PASSWORD} else { "admin" }
$Timestamp    = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

@"
# OdooDevTools Agent configuration
# Generated by install-agent.ps1 on $Timestamp

# Cloud server — copy from your dashboard
AGENT_CLOUD_URL=$AgentCloudUrl
AGENT_REGISTRATION_TOKEN=$AgentRegistrationToken

# Odoo server — edit these to match your Odoo instance
ODOO_URL=$OdooUrl
PG_ODOO_DB=$OdooDb
ODOO_ADMIN_USER=$OdooUser
ODOO_ADMIN_PASSWORD=$OdooPassword

# Optional tuning
APP_ENV=production
AGENT_SAMPLER_MODE=sampled
AGENT_SAMPLER_RATE=0.1
AGENT_SLOW_THRESHOLD_MS=200
"@ | Set-Content $ConfigFile -Encoding UTF8

# Restrict config file to Administrators + SYSTEM only
$Acl = Get-Acl $ConfigFile
$Acl.SetAccessRuleProtection($true, $false)
$Acl.Access | ForEach-Object { $Acl.RemoveAccessRule($_) | Out-Null }
foreach ($Identity in @("BUILTIN\Administrators", "NT AUTHORITY\SYSTEM")) {
    $Rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
        $Identity, "FullControl", "Allow")
    $Acl.AddAccessRule($Rule)
}
Set-Acl $ConfigFile $Acl
Write-Info "Config written. Edit $ConfigFile to set your Odoo credentials."

# ─── Windows Service via Task Scheduler ───────────────────────────────────────
Write-Step "Registering as a Windows Scheduled Task (runs at system startup)"

$BinaryPath = Join-Path $InstallDir $BinaryName

# Remove existing task if present
if (Get-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue) {
    Stop-ScheduledTask  -TaskName $ServiceName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $ServiceName -Confirm:$false
    Write-Info "Removed previous scheduled task"
}

$Action   = New-ScheduledTaskAction -Execute $BinaryPath
$Trigger  = New-ScheduledTaskTrigger -AtStartup
$Settings = New-ScheduledTaskSettingsSet `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -RestartCount 5 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -StartWhenAvailable `
    -MultipleInstances IgnoreNew
$Principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

Register-ScheduledTask `
    -TaskName    $ServiceName `
    -Action      $Action `
    -Trigger     $Trigger `
    -Settings    $Settings `
    -Principal   $Principal `
    -Description "OdooDevTools Agent — monitors your Odoo instance" | Out-Null

# Start it immediately
Start-ScheduledTask -TaskName $ServiceName
Start-Sleep -Seconds 2

$State = (Get-ScheduledTask -TaskName $ServiceName).State
if ($State -eq "Running") {
    Write-Info "Scheduled task is running"
} else {
    Write-Warn "Task state: $State — check Event Viewer for details"
}

Write-Info "Task management commands:"
Write-Host "  Stop  : Stop-ScheduledTask  -TaskName '$ServiceName'"
Write-Host "  Start : Start-ScheduledTask -TaskName '$ServiceName'"
Write-Host "  Logs  : Get-EventLog -LogName Application -Source '$ServiceName'"

# ─── Done ─────────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "✓ OdooDevTools Agent $AgentVersion installed successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "  Next steps:"
Write-Host "  1. Edit $ConfigFile — set ODOO_URL, PG_ODOO_DB, ODOO_ADMIN_USER, ODOO_ADMIN_PASSWORD"
Write-Host "  2. Stop-ScheduledTask -TaskName '$ServiceName'"
Write-Host "     Start-ScheduledTask -TaskName '$ServiceName'"
Write-Host "  3. Check your dashboard — the agent will appear as 'online' within ~30 seconds"
Write-Host ""
