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

# The title bar version is baked into the bundle from build/windows/info.json
# (App.tsx imports it), while Settings reads the same value from the exe's
# VERSIONINFO. A dist built before a version bump ships the old number: v0.9.0
# said 0.9.0 in its properties but 0.8.0 in the title bar. Fail before touching
# cmd/agentdeck-desktop/assets so a stale dist cannot be packaged.
$infoPath = Join-Path $projectRoot 'build\windows\info.json'
$expected = (Get-Content -LiteralPath $infoPath -Raw | ConvertFrom-Json).fixed.product_version
$bundleDir = Join-Path $dist 'assets'
$seen = @()
foreach ($file in Get-ChildItem -LiteralPath $bundleDir -Filter '*.js' -File) {
    $text = Get-Content -LiteralPath $file.FullName -Raw
    foreach ($match in [regex]::Matches($text, 'product_version:"([^"]+)"')) {
        $seen += $match.Groups[1].Value
    }
}
$seen = @($seen | Sort-Object -Unique)
if ($seen.Count -eq 0) {
    throw "No product_version found under frontend/dist/assets; rebuild the frontend before syncing."
}
if ($seen.Count -ne 1 -or $seen[0] -ne $expected) {
    throw "frontend/dist was built with product_version $($seen -join ', ') but build/windows/info.json says $expected; run npm run build again before syncing."
}

if (-not $bundles.StartsWith($projectRoot + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Asset destination is outside the project: $bundles"
}
if (Test-Path -LiteralPath $bundles) {
    Remove-Item -LiteralPath $bundles -Recurse -Force
}
New-Item -ItemType Directory -Path $destination -Force | Out-Null
Get-ChildItem -LiteralPath $dist | Copy-Item -Destination $destination -Recurse -Force
