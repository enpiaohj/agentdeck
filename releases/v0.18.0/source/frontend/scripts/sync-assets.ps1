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
$expected = (Get-Content -LiteralPath $infoPath -Raw -Encoding UTF8 | ConvertFrom-Json).fixed.product_version
$bundleDir = Join-Path $dist 'assets'
$seen = @()
foreach ($file in Get-ChildItem -LiteralPath $bundleDir -Filter '*.js' -File) {
    $text = Get-Content -LiteralPath $file.FullName -Raw -Encoding UTF8
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
# Everything under assets/ is generated from frontend/dist and embedded whole
# (//go:embed all:assets). Clearing only assets/assets left files deleted from
# frontend/public behind in assets/agents, and they went into the exe: v0.9.0
# shipped the brand PNGs its CHANGELOG says were removed. Clear every generated
# entry (only .gitkeep is tracked), then require an exact mirror of dist.
New-Item -ItemType Directory -Path $destination -Force | Out-Null
foreach ($entry in Get-ChildItem -LiteralPath $destination -Force) {
    if ($entry.Name -ne '.gitkeep') {
        Remove-Item -LiteralPath $entry.FullName -Recurse -Force
    }
}
Get-ChildItem -LiteralPath $dist | Copy-Item -Destination $destination -Recurse -Force

function Get-RelativeFiles([string]$root) {
    $sep = [System.IO.Path]::DirectorySeparatorChar
    $full = [System.IO.Path]::GetFullPath($root).TrimEnd($sep) + $sep
    Get-ChildItem -LiteralPath $root -Recurse -File -Force |
        ForEach-Object { $_.FullName.Substring($full.Length) } |
        Where-Object { $_ -ne '.gitkeep' } | Sort-Object
}
$want = @(Get-RelativeFiles $dist)
$got = @(Get-RelativeFiles $destination)
$diff = @(Compare-Object -ReferenceObject $want -DifferenceObject $got)
if ($diff.Count -ne 0) {
    throw "cmd/agentdeck-desktop/assets does not mirror frontend/dist: $(($diff | ForEach-Object { $_.SideIndicator + ' ' + $_.InputObject }) -join '; ')"
}
