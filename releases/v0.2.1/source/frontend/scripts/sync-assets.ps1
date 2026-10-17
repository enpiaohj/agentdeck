# Sync generated frontend assets. Keep this file ASCII-only for PowerShell 5.1.
$ErrorActionPreference = 'Stop'
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$dist = Join-Path $projectRoot 'frontend\dist'
$destination = Join-Path $projectRoot 'cmd\agentdeck-desktop\assets'
$bundles = [System.IO.Path]::GetFullPath((Join-Path $destination 'assets'))

# Validate build output before removing old, reproducible bundles.
if (-not (Test-Path -LiteralPath (Join-Path $dist 'index.html') -PathType Leaf)) {
    throw "Missing frontend/dist/index.html; run npm run build first."
}
if (-not $bundles.StartsWith($projectRoot + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Asset destination is outside the project: $bundles"
}
if (Test-Path -LiteralPath $bundles) {
    Remove-Item -LiteralPath $bundles -Recurse -Force
}
New-Item -ItemType Directory -Path $destination -Force | Out-Null
Get-ChildItem -LiteralPath $dist | Copy-Item -Destination $destination -Recurse -Force
