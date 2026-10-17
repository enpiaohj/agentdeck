# AgentDeck desktop rebuild + self-check screenshot.
#
# Two mandatory steps (learned the hard way):
#  1. Wipe cmd/agentdeck-desktop/assets/assets before copying. WebView2 caches
#     index.html, and an append-only assets dir keeps stale js reachable, so the
#     window would keep running old code.
#  2. Kill leftover instances before launching - the exe allows multiple windows
#     and a screenshot may capture the stale one.
#
# Usage: powershell -File frontend/scripts/rebuild.ps1 [-NoShot]
# NOTE: keep this file ASCII-only; Windows PowerShell 5.1 mangles CJK here.
param([switch]$NoShot)
$ErrorActionPreference = 'Stop'
Set-Location D:\Work\AgentDeck

$go = "C:\Program Files\Go\bin\go.exe"

Write-Host "[1/5] npm build..." -ForegroundColor Cyan
Push-Location frontend
npm run build
if ($LASTEXITCODE -ne 0) { Pop-Location; throw "npm build failed" }
Pop-Location

Write-Host "[2/5] sync assets (wipe first)..." -ForegroundColor Cyan
& "$PSScriptRoot\sync-assets.ps1"

Write-Host "[3/5] go build..." -ForegroundColor Cyan
& $go build -tags production -ldflags "-H windowsgui -s -w" -o bin\agentdeck-desktop.exe ./cmd/agentdeck-desktop
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Write-Host "[4/5] restart app..." -ForegroundColor Cyan
Get-Process agentdeck-desktop -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 2
Start-Process -FilePath bin\agentdeck-desktop.exe
Start-Sleep 9

if (-not $NoShot) {
  Write-Host "[5/5] screenshot..." -ForegroundColor Cyan
  python frontend\scripts\uishot.py D:\Temp\agentdeck-ui.png
}
Write-Host "done" -ForegroundColor Green
