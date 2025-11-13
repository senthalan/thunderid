#!/usr/bin/env pwsh
# ----------------------------------------------------------------------------
# Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied. See the License for the
# specific language governing permissions and limitations
# under the License.
# ----------------------------------------------------------------------------

# Thunder Setup Script
# Orchestrates the complete setup lifecycle:
# 1. Starts Thunder server with security disabled
# 2. Executes bootstrap scripts (built-in + custom)
# 3. Stops Thunder server
# 4. Exits cleanly

# Exit on any error
$ErrorActionPreference = 'Stop'

# Default settings
$DEBUG_PORT = if ($env:DEBUG_PORT) { [int]$env:DEBUG_PORT } else { 2345 }
$DEBUG_MODE = if ($env:DEBUG_MODE -eq "true") { $true } else { $false }
$BOOTSTRAP_FAIL_FAST = if ($env:BOOTSTRAP_FAIL_FAST -eq "false") { $false } else { $true }
$BOOTSTRAP_SKIP_PATTERN = if ($env:BOOTSTRAP_SKIP_PATTERN) { $env:BOOTSTRAP_SKIP_PATTERN } else { "" }
$BOOTSTRAP_ONLY_PATTERN = if ($env:BOOTSTRAP_ONLY_PATTERN) { $env:BOOTSTRAP_ONLY_PATTERN } else { "" }
$BOOTSTRAP_DIR = if ($env:BOOTSTRAP_DIR) { $env:BOOTSTRAP_DIR } else { ".\bootstrap" }

# ============================================================================
# Logging Functions
# ============================================================================

function Log-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Blue
}

function Log-Success {
    param([string]$Message)
    Write-Host "[SUCCESS] ✓ $Message" -ForegroundColor Green
}

function Log-Warning {
    param([string]$Message)
    Write-Host "[WARNING] ⚠ $Message" -ForegroundColor Yellow
}

function Log-Error {
    param([string]$Message)
    Write-Host "[ERROR] ✗ $Message" -ForegroundColor Red
}

function Log-Debug {
    param([string]$Message)
    if ($env:DEBUG -eq "true") {
        Write-Host "[DEBUG] $Message" -ForegroundColor Cyan
    }
}

# ============================================================================
# API Call Helper Function
# ============================================================================

function Invoke-ThunderApi {
    param(
        [string]$Method,
        [string]$Endpoint,
        [string]$Data = ""
    )

    $url = "$script:THUNDER_API_BASE$Endpoint"

    Log-Debug "API Call: $Method $url"

    try {
        $headers = @{
            "Content-Type" = "application/json"
        }

        $params = @{
            Uri = $url
            Method = $Method
            Headers = $headers
            SkipCertificateCheck = $true
            ErrorAction = 'Stop'
        }

        if ($Data -and ($Method -eq "POST" -or $Method -eq "PUT" -or $Method -eq "PATCH")) {
            $params.Body = $Data
        }

        $response = Invoke-WebRequest @params
        $statusCode = $response.StatusCode
        $body = $response.Content

        return @{
            StatusCode = $statusCode
            Body = $body
        }
    }
    catch {
        $statusCode = if ($_.Exception.Response) {
            [int]$_.Exception.Response.StatusCode
        } else {
            0
        }

        $body = if ($_.Exception.Response) {
            try {
                $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
                $reader.ReadToEnd()
            }
            catch {
                ""
            }
        }
        else {
            ""
        }

        return @{
            StatusCode = $statusCode
            Body = $body
        }
    }
}

# ============================================================================
# Help Function
# ============================================================================

function Show-Help {
    Write-Host ""
    Write-Host "Thunder Setup Script"
    Write-Host ""
    Write-Host "Usage: .\setup.ps1 [options]"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  --debug                  Enable debug mode with remote debugging"
    Write-Host "  --debug-port PORT        Set debug port (default: 2345)"
    Write-Host "  --help                   Show this help message"
    Write-Host ""
    Write-Host "Description:"
    Write-Host "  This script performs initial setup by:"
    Write-Host "  1. Starting Thunder server temporarily with security disabled"
    Write-Host "  2. Running bootstrap scripts to create default resources"
    Write-Host "  3. Stopping the server cleanly"
    Write-Host ""
    Write-Host "  After setup completes, use '.\start.ps1' to start Thunder normally."
    Write-Host ""
}

# ============================================================================
# Parse Command Line Arguments
# ============================================================================

$i = 0
while ($i -lt $args.Count) {
    switch ($args[$i]) {
        '--debug' {
            $DEBUG_MODE = $true
            $i++
            break
        }
        '--debug-port' {
            $i++
            if ($i -lt $args.Count) {
                $DEBUG_PORT = [int]$args[$i]
                $i++
            }
            else {
                Write-Host "Missing value for --debug-port" -ForegroundColor Red
                exit 1
            }
            break
        }
        '--help' {
            Show-Help
            exit 0
        }
        default {
            Write-Host "Unknown option: $($args[$i])" -ForegroundColor Red
            Write-Host "Use --help for usage information"
            exit 1
        }
    }
}

# ============================================================================
# Read Configuration from deployment.yaml
# ============================================================================

$CONFIG_FILE = ".\repository\conf\deployment.yaml"

function Read-Config {
    $configFile = $CONFIG_FILE

    if (-not (Test-Path $configFile)) {
        # Try alternative path (for packaged distribution)
        $configFile = ".\backend\cmd\server\repository\conf\deployment.yaml"
    }

    if (-not (Test-Path $configFile)) {
        Log-Warning "Configuration file not found, using defaults"
        return $false
    }

    Log-Debug "Reading configuration from: $configFile"

    # Try yq first (YAML parser)
    if (Get-Command yq -ErrorAction SilentlyContinue) {
        $script:HOSTNAME = & yq eval '.server.hostname // "localhost"' $configFile 2>$null
        $script:PORT = & yq eval '.server.port // 8090' $configFile 2>$null
        $script:HTTP_ONLY = & yq eval '.server.http_only // false' $configFile 2>$null
    }
    else {
        # Fallback: basic parsing with Select-String
        $content = Get-Content $configFile -Raw

        # Parse hostname
        if ($content -match '(?m)^\s*hostname:\s*[''"]?([^''"\s]+)[''"]?') {
            $script:HOSTNAME = $matches[1]
        }
        else {
            $script:HOSTNAME = "localhost"
        }

        # Parse port
        if ($content -match '(?m)^\s*port:\s*(\d+)') {
            $script:PORT = [int]$matches[1]
        }
        else {
            $script:PORT = 8090
        }

        # Parse http_only
        if ($content -match '(?m)http_only:\s*true') {
            $script:HTTP_ONLY = "true"
        }
        else {
            $script:HTTP_ONLY = "false"
        }
    }

    # Determine protocol
    if ($script:HTTP_ONLY -eq "true") {
        $script:PROTOCOL = "http"
    }
    else {
        $script:PROTOCOL = "https"
    }

    return $true
}

# Read configuration
Read-Config | Out-Null

# Construct base URL
$BASE_URL = "$($script:PROTOCOL)://$($script:HOSTNAME):$($script:PORT)"
$script:THUNDER_API_BASE = $BASE_URL

Write-Host ""
Write-Host "========================================="
Write-Host "   Thunder Setup"
Write-Host "========================================="
Write-Host ""
Write-Host "Server URL: $BASE_URL" -ForegroundColor Blue
if ($DEBUG_MODE) {
    Write-Host "Debug: Enabled (port $DEBUG_PORT)" -ForegroundColor Blue
}
Write-Host ""

# ============================================================================
# Kill Existing Processes on Ports
# ============================================================================

function Stop-PortListener {
    param([int]$port)

    Write-Host "Checking for processes listening on TCP port $port..."

    try {
        $pids = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction Stop |
                Select-Object -ExpandProperty OwningProcess -Unique
    }
    catch {
        # Fallback to netstat parsing
        $pids = @()
        try {
            $netstat = & netstat -ano 2>$null | Select-String ":$port"
            foreach ($line in $netstat) {
                $parts = ($line -split '\s+') | Where-Object { $_ -ne '' }
                if ($parts.Count -ge 5) {
                    $procId = $parts[-1]
                    if ([int]::TryParse($procId, [ref]$null)) {
                        $pids += [int]$procId
                    }
                }
            }
        }
        catch { }
    }

    $pids = $pids | Where-Object { $_ -and ($_ -ne 0) } | Select-Object -Unique
    foreach ($procId in $pids) {
        try {
            Write-Host "Killing PID $procId that is listening on port $port"
            Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
        }
        catch {
            Write-Host "Unable to kill PID $procId : $_" -ForegroundColor Yellow
        }
    }
}

if ($DEBUG_MODE) {
    Stop-PortListener -port $DEBUG_PORT
}
Start-Sleep -Seconds 1

# Check for Delve if debug mode is enabled
if ($DEBUG_MODE -and -not (Get-Command dlv -ErrorAction SilentlyContinue)) {
    Write-Host "❌ Debug mode requires Delve debugger" -ForegroundColor Red
    Write-Host ""
    Write-Host "💡 Install Delve using:" -ForegroundColor Cyan
    Write-Host "   go install github.com/go-delve/delve/cmd/dlv@latest" -ForegroundColor Cyan
    exit 1
}

# ============================================================================
# Start Thunder Server with Security Disabled
# ============================================================================

Write-Host "⚠️  Starting temporary server with security disabled..." -ForegroundColor Yellow
Write-Host ""

# Export environment variable to skip security
$env:THUNDER_SKIP_SECURITY = "true"

# Resolve thunder executable path
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$possible = @(
    (Join-Path $scriptDir 'thunder.exe'),
    (Join-Path $scriptDir 'thunder')
)
$thunderPath = $possible | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $thunderPath) {
    $thunderPath = Join-Path $scriptDir 'thunder'
}

$proc = $null
try {
    if ($DEBUG_MODE) {
        $dlvArgs = @(
            'exec'
            "--listen=:$DEBUG_PORT"
            '--headless=true'
            '--api-version=2'
            '--accept-multiclient'
            '--continue'
            $thunderPath
        )
        $proc = Start-Process -FilePath dlv -ArgumentList $dlvArgs -WorkingDirectory $scriptDir -NoNewWindow -PassThru
    }
    else {
        $proc = Start-Process -FilePath $thunderPath -WorkingDirectory $scriptDir -NoNewWindow -PassThru
    }

    $THUNDER_PID = $proc.Id

    # Cleanup function
    $cleanup = {
        Write-Host ""
        Write-Host "🛑 Stopping temporary server..." -ForegroundColor Cyan
        if ($proc -and -not $proc.HasExited) {
            try {
                Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
            } catch { }
        }
    }

    # Register cleanup on exit
    Register-EngineEvent PowerShell.Exiting -Action $cleanup | Out-Null

    # ============================================================================
    # Wait for Server to be Ready
    # ============================================================================

    Write-Host "⏳ Waiting for server to be ready..." -ForegroundColor Blue
    $TIMEOUT = 60
    $ELAPSED = 0
    $RETRY_DELAY = 2

    while ($ELAPSED -lt $TIMEOUT) {
        try {
            $response = Invoke-WebRequest -Uri "$BASE_URL/health/readiness" -SkipCertificateCheck -TimeoutSec 2 -ErrorAction Stop
            if ($response.StatusCode -eq 200) {
                Write-Host ""
                Write-Host "✓ Server is ready" -ForegroundColor Green
                Write-Host ""
                break
            }
        }
        catch {
            # Server not ready yet
        }

        Start-Sleep -Seconds $RETRY_DELAY
        $ELAPSED += $RETRY_DELAY
        Write-Host "." -NoNewline
    }

    if ($ELAPSED -ge $TIMEOUT) {
        Write-Host ""
        Write-Host "❌ Server failed to start within $TIMEOUT seconds" -ForegroundColor Red
        Write-Host "Expected server at: $BASE_URL" -ForegroundColor Red
        exit 1
    }

    # ============================================================================
    # Run Bootstrap Scripts
    # ============================================================================

    # Check if bootstrap directory exists
    if (-not (Test-Path $BOOTSTRAP_DIR)) {
        Log-Warning "Bootstrap directory not found: $BOOTSTRAP_DIR"
        Log-Info "Skipping bootstrap execution"
    }
    else {
        Log-Info "========================================="
        Log-Info "Thunder Bootstrap Process"
        Log-Info "========================================="
        Log-Info "Bootstrap directory: $BOOTSTRAP_DIR"
        Log-Info "Fail fast: $BOOTSTRAP_FAIL_FAST"
        Log-Info "Started at: $(Get-Date)"
        Write-Host ""

        # Collect all scripts from both built-in and custom directories
        $scripts = @()

        # Find scripts in main bootstrap directory
        if (Test-Path $BOOTSTRAP_DIR) {
            $scripts += Get-ChildItem -Path $BOOTSTRAP_DIR -Filter "*.ps1" -File -ErrorAction SilentlyContinue
            $scripts += Get-ChildItem -Path $BOOTSTRAP_DIR -Filter "*.sh" -File -ErrorAction SilentlyContinue
        }

        # Find scripts in custom directory
        $customDir = Join-Path $BOOTSTRAP_DIR "custom"
        if (Test-Path $customDir) {
            $scripts += Get-ChildItem -Path $customDir -Filter "*.ps1" -File -ErrorAction SilentlyContinue
            $scripts += Get-ChildItem -Path $customDir -Filter "*.sh" -File -ErrorAction SilentlyContinue
        }

        # Sort scripts by filename (numeric prefix determines order)
        $sortedScripts = $scripts | Sort-Object Name

        if ($sortedScripts.Count -eq 0) {
            Log-Warning "No bootstrap scripts found"
        }
        else {
            Log-Info "Discovered $($sortedScripts.Count) script(s)"
            Write-Host ""

            # Execute scripts
            $scriptCount = 0
            $successCount = 0
            $failedCount = 0
            $skippedCount = 0

            foreach ($script in $sortedScripts) {
                $scriptName = $script.Name

                # Skip if matches skip pattern
                if ($BOOTSTRAP_SKIP_PATTERN -and ($scriptName -match $BOOTSTRAP_SKIP_PATTERN)) {
                    Log-Info "⊘ Skipping $scriptName (matches skip pattern)"
                    $skippedCount++
                    continue
                }

                # Skip if doesn't match only pattern
                if ($BOOTSTRAP_ONLY_PATTERN -and ($scriptName -notmatch $BOOTSTRAP_ONLY_PATTERN)) {
                    Log-Info "⊘ Skipping $scriptName (doesn't match only pattern)"
                    $skippedCount++
                    continue
                }

                Log-Info "▶ Executing: $scriptName"
                $scriptCount++

                # Execute script
                $startTime = Get-Date

                try {
                    if ($script.Extension -eq ".ps1") {
                        # PowerShell script
                        & $script.FullName
                        $exitCode = $LASTEXITCODE
                    }
                    else {
                        # Bash script - requires bash or WSL on Windows
                        if (Get-Command bash -ErrorAction SilentlyContinue) {
                            & bash $script.FullName
                            $exitCode = $LASTEXITCODE
                        }
                        else {
                            Log-Warning "$scriptName is a bash script but bash is not available"
                            Log-Info "Install Git Bash or WSL to run .sh scripts on Windows"
                            $skippedCount++
                            continue
                        }
                    }

                    $endTime = Get-Date
                    $duration = [math]::Round(($endTime - $startTime).TotalSeconds, 2)

                    if ($exitCode -eq 0 -or $null -eq $exitCode) {
                        Log-Success "$scriptName completed (${duration}s)"
                        $successCount++
                    }
                    else {
                        Log-Error "$scriptName failed with exit code $exitCode (${duration}s)"
                        $failedCount++

                        if ($BOOTSTRAP_FAIL_FAST) {
                            Log-Error "Stopping bootstrap (BOOTSTRAP_FAIL_FAST=true)"
                            exit 1
                        }
                    }
                }
                catch {
                    $endTime = Get-Date
                    $duration = [math]::Round(($endTime - $startTime).TotalSeconds, 2)

                    Log-Error "$scriptName failed with error: $_  (${duration}s)"
                    $failedCount++

                    if ($BOOTSTRAP_FAIL_FAST) {
                        Log-Error "Stopping bootstrap (BOOTSTRAP_FAIL_FAST=true)"
                        exit 1
                    }
                }

                Write-Host ""
            }

            # Summary
            Write-Host ""
            Log-Info "========================================="
            Log-Info "Bootstrap Summary"
            Log-Info "========================================="
            Log-Info "Total scripts discovered: $($sortedScripts.Count)"
            Log-Info "Executed: $scriptCount"
            Log-Success "Successful: $successCount"

            if ($failedCount -gt 0) {
                Log-Error "Failed: $failedCount"
            }

            if ($skippedCount -gt 0) {
                Log-Info "Skipped: $skippedCount"
            }

            Log-Info "Completed at: $(Get-Date)"
            Log-Info "========================================="

            if ($failedCount -gt 0) {
                exit 1
            }

            Log-Success "Bootstrap completed successfully!"
        }
    }

    # ============================================================================
    # Setup Completed
    # ============================================================================

    Write-Host ""
    Write-Host "========================================="
    Write-Host "✅ Setup completed successfully!" -ForegroundColor Green
    Write-Host "========================================="
    Write-Host ""
    Write-Host "💡 Next steps:"
    Write-Host "   1. Start the server: .\start.ps1" -ForegroundColor Cyan
    Write-Host "   2. Access Thunder at: $BASE_URL" -ForegroundColor Cyan
    Write-Host "   3. Login with admin credentials:"
    Write-Host "      Username: admin" -ForegroundColor Cyan
    Write-Host "      Password: admin" -ForegroundColor Cyan
    Write-Host ""
}
finally {
    # Cleanup
    Write-Host ""
    Write-Host "🛑 Stopping temporary server..." -ForegroundColor Cyan
    if ($proc -and -not $proc.HasExited) {
        try {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        } catch { }
    }
}

exit 0
