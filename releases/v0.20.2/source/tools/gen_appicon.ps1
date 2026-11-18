# gen_appicon.ps1 -- package the generated master for Windows and the UI.
# Default: import build/appicon-source.png, preserving transparency.
# Explicit -Design rows|deck|panes renders the legacy candidates below.
#
# The legacy candidates share a rounded tile with an azure -> indigo gradient,
# a soft top gloss and a bottom vignette (that part is the brand). What changes
# between designs is the white mark in the middle:
#
#   rows   (legacy) the session list itself -- the row being worked on is a
#          full card with the spark at its trailing edge, two quieter rows
#          behind it. One bold white mass, so it still reads at 16px.
#   deck   a hand of cards fanned about its centre, the middle one face up with
#          the spark: "all your agents, one deck" taken literally. Prettier at
#          256px, but at 32px the wings are only a soft halo.
#   panes  the previous mark: four white bars of unequal width, the app's
#          four-column workspace (kept because it can be regenerated for
#          comparison).
#
# Designs that were drawn and dropped, for the record: a stack of three cards
# (ghost layers are invisible below 64px), a white workspace plate with the
# column gaps cut out of it (the gaps read as thin align-lines, not columns),
# and two card fans (the wings swing out below the front card like a bow tie).
#
# The default generated master is resampled to every size. Legacy candidates
# are drawn per size. The 256px render is copied into the frontend so the in-app brand
# mark and the executable icon can never drift apart.
#
# Usage:  powershell -NoProfile -ExecutionPolicy Bypass -File tools\gen_appicon.ps1
#         powershell ... -File tools\gen_appicon.ps1 -Design deck -Out _probe\x
#         powershell ... -File tools\gen_appicon.ps1 -Sheet _probe\sheet
# Output: <Out>/appicon.png          1024px master
#         <Out>/windows/icon.ico     multi-size icon (256..16)
#         <Out>/appicon-256.png
#         frontend/public/appicon-256.png   (only when -Out is the default build)
#         <Sheet>/<size>.png + <size>-zoom.png  (only with -Sheet, for review)
#
# After changing this file the linker resource must be regenerated:
#     wails3 generate syso -icon build/windows/icon.ico -info build/windows/info.json `
#         -manifest build/windows/wails.exe.manifest -arch amd64 `
#         -out cmd/agentdeck-desktop/resource_windows_amd64.syso
#
# ASCII only on purpose: Windows PowerShell 5.1 reads .ps1 as the ANSI code page.

param(
  [ValidateSet('rows', 'deck', 'panes')][string]$Design = 'rows',
  [string]$SourceImage = '',
  [string]$Out = '',
  [string]$Sheet = ''
)

Add-Type -AssemblyName System.Drawing

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
if (-not $root) { $root = (Get-Location).Path }

$defaultOut = [string]::IsNullOrEmpty($Out)
if ($defaultOut) { $Out = Join-Path $root 'build' }
$buildDir = $Out
$winDir   = Join-Path $buildDir 'windows'
$pubDir   = Join-Path $root 'frontend\public'
$sourceBitmap = $null
# Generated master is the default. Explicit -Design keeps legacy candidates reproducible.
if (-not $PSBoundParameters.ContainsKey('Design') -and [string]::IsNullOrEmpty($SourceImage)) {
  $SourceImage = Join-Path $root 'build\appicon-source.png'
}
if (-not [string]::IsNullOrEmpty($SourceImage)) {
  $sourceBitmap = [System.Drawing.Image]::FromFile((Resolve-Path -LiteralPath $SourceImage).Path)
  if ($sourceBitmap.Width -ne $sourceBitmap.Height) { $sourceBitmap.Dispose(); throw 'Icon master must be square.' }
}
$dirs = @($buildDir, $winDir)
if ($defaultOut) { $dirs += $pubDir }
foreach ($d in $dirs) {
  if (-not (Test-Path $d)) { New-Item -ItemType Directory -Force -Path $d | Out-Null }
}

function New-RoundedPath {
  param([double]$X, [double]$Y, [double]$W, [double]$H, [double]$R)
  $p = [System.Drawing.Drawing2D.GraphicsPath]::new()
  $d = $R * 2
  $p.AddArc([float]$X, [float]$Y, [float]$d, [float]$d, 180, 90)
  $p.AddArc([float]($X + $W - $d), [float]$Y, [float]$d, [float]$d, 270, 90)
  $p.AddArc([float]($X + $W - $d), [float]($Y + $H - $d), [float]$d, [float]$d, 0, 90)
  $p.AddArc([float]$X, [float]($Y + $H - $d), [float]$d, [float]$d, 90, 90)
  $p.CloseFigure()
  return $p
}

# Four-point star (concave sides), the "agent at work" accent.
function New-SparkPath {
  param([double]$Cx, [double]$Cy, [double]$R, [double]$Waist)
  $p = [System.Drawing.Drawing2D.GraphicsPath]::new()
  $w = $Waist
  $p.AddBezier([float]$Cx, [float]($Cy - $R), [float]($Cx + $w), [float]($Cy - $w), [float]($Cx + $w), [float]($Cy - $w), [float]($Cx + $R), [float]$Cy)
  $p.AddBezier([float]($Cx + $R), [float]$Cy, [float]($Cx + $w), [float]($Cy + $w), [float]($Cx + $w), [float]($Cy + $w), [float]$Cx, [float]($Cy + $R))
  $p.AddBezier([float]$Cx, [float]($Cy + $R), [float]($Cx - $w), [float]($Cy + $w), [float]($Cx - $w), [float]($Cy + $w), [float]($Cx - $R), [float]$Cy)
  $p.AddBezier([float]($Cx - $R), [float]$Cy, [float]($Cx - $w), [float]($Cy - $w), [float]($Cx - $w), [float]($Cy - $w), [float]$Cx, [float]($Cy - $R))
  $p.CloseFigure()
  return $p
}

# Shared brand base: gradient tile + gloss + vignette.
function Add-Tile {
  param($G, [double]$U)
  $tile = New-RoundedPath (40 * $U) (40 * $U) (944 * $U) (944 * $U) (210 * $U)
  $grad = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
    [System.Drawing.PointF]::new([float](64 * $U), [float](64 * $U)),
    [System.Drawing.PointF]::new([float](960 * $U), [float](960 * $U)),
    [System.Drawing.Color]::FromArgb(255, 92, 162, 255),
    [System.Drawing.Color]::FromArgb(255, 26, 52, 190))
  $G.FillPath($grad, $tile)

  $G.SetClip($tile)
  $gloss = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
    [System.Drawing.PointF]::new(0, [float](40 * $U)),
    [System.Drawing.PointF]::new(0, [float](620 * $U)),
    [System.Drawing.Color]::FromArgb(66, 255, 255, 255),
    [System.Drawing.Color]::FromArgb(0, 255, 255, 255))
  $G.FillRectangle($gloss, [float](40 * $U), [float](40 * $U), [float](944 * $U), [float](580 * $U))
  $vig = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
    [System.Drawing.PointF]::new(0, [float](660 * $U)),
    [System.Drawing.PointF]::new(0, [float](984 * $U)),
    [System.Drawing.Color]::FromArgb(0, 12, 20, 60),
    [System.Drawing.Color]::FromArgb(52, 12, 20, 60))
  $G.FillRectangle($vig, [float](40 * $U), [float](660 * $U), [float](944 * $U), [float](324 * $U))
  $G.ResetClip()

  $grad.Dispose(); $gloss.Dispose(); $vig.Dispose(); $tile.Dispose()
}

# Soft drop shadow, two passes, to lift a white shape off the tile.
function Add-Shadow {
  param($G, [double]$U, $Rect, [double]$R, [double]$Dy)
  $shadow = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(26, 8, 20, 70))
  foreach ($d in @($Dy, $Dy * 2)) {
    $p = New-RoundedPath ($Rect[0] * $U) (($Rect[1] + $d) * $U) ($Rect[2] * $U) ($Rect[3] * $U) ($R * $U)
    $G.FillPath($shadow, $p)
    $p.Dispose()
  }
  $shadow.Dispose()
}

function Add-MarkRows {
  param($G, [double]$U)
  # The session list itself: the row being worked on is a full card, the two
  # below it are quieter rows, and the spark sits at the trailing edge of the
  # active row -- where the app keeps its state marks.
  $h = @(164.0, 108.0, 108.0)
  $w = @(552.0, 448.0, 504.0)
  $y = @(280.0, 490.0, 644.0)
  # the lower rows sit in the tile's bottom vignette, so they need more white
  # than they would on the flat gradient to still show at 16px
  $a = @(255, 172, 132)
  $r = @(82.0, 54.0, 54.0)
  $x = 236.0
  Add-Shadow $G $U @($x, $y[0], $w[0], $h[0]) $r[0] 7
  for ($i = 0; $i -lt 3; $i++) {
    $white = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb($a[$i], 255, 255, 255))
    $p = New-RoundedPath ($x * $U) ($y[$i] * $U) ($w[$i] * $U) ($h[$i] * $U) ($r[$i] * $U)
    $G.FillPath($white, $p)
    $p.Dispose(); $white.Dispose()
  }
  $blue = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(255, 46, 104, 226))
  $spark = New-SparkPath (($x + $w[0] - 128) * $U) (($y[0] + $h[0] / 2) * $U) (64 * $U) (16 * $U)
  $G.FillPath($blue, $spark)
  $spark.Dispose(); $blue.Dispose()
}

function Add-MarkDeck {
  param($G, [double]$U)
  # A hand of cards, fanned about its centre so the wings come out symmetric,
  # the middle one face up with the spark.
  $cw = 404.0; $ch = 512.0
  $cx = 512.0; $cy = 512.0
  $r = 48.0
  $cards = @(
    @{ ang = -23.0; a = 148 },
    @{ ang =  23.0; a = 148 },
    @{ ang =   0.0; a = 255 }
  )
  foreach ($c in $cards) {
    $G.TranslateTransform([float]($cx * $U), [float]($cy * $U))
    $G.RotateTransform([float]$c.ang)
    $G.TranslateTransform([float](-$cx * $U), [float](-$cy * $U))
    if ($c.a -eq 255) { Add-Shadow $G $U @(($cx - $cw / 2), ($cy - $ch / 2), $cw, $ch) $r 7 }
    $white = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb($c.a, 255, 255, 255))
    $p = New-RoundedPath (($cx - $cw / 2) * $U) (($cy - $ch / 2) * $U) ($cw * $U) ($ch * $U) ($r * $U)
    $G.FillPath($white, $p)
    $p.Dispose(); $white.Dispose()
    $G.ResetTransform()
  }
  $blue = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(255, 46, 104, 226))
  $spark = New-SparkPath ($cx * $U) ($cy * $U) (108 * $U) (26 * $U)
  $G.FillPath($blue, $spark)
  $spark.Dispose(); $blue.Dispose()
}

function Add-MarkPanes {
  param($G, [double]$U)
  # nav | list | content | info -- what the workspace column layout looks like
  $w = @(128.0, 176.0, 240.0, 176.0)
  $x = @(114.0, 276.0, 486.0, 760.0)
  $y = 286.0
  $h = 452.0
  $r = 34.0
  Add-Shadow $G $U @(114.0, $y, 822.0, $h) $r 6
  $white = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::White)
  for ($i = 0; $i -lt 4; $i++) {
    $p = New-RoundedPath ($x[$i] * $U) ($y * $U) ($w[$i] * $U) ($h * $U) ($r * $U)
    $G.FillPath($white, $p)
    $p.Dispose()
  }
  $blue = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(255, 46, 104, 226))
  $spark = New-SparkPath ((($x[2] + $w[2] / 2)) * $U) ((($y + $h / 2)) * $U) (72 * $U) (17 * $U)
  $G.FillPath($blue, $spark)
  $spark.Dispose(); $blue.Dispose(); $white.Dispose()
}

function New-IconBitmap {
  param([int]$Size)

  $bmp = [System.Drawing.Bitmap]::new($Size, $Size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.SmoothingMode     = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
  $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
  $g.PixelOffsetMode   = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
  $g.Clear([System.Drawing.Color]::Transparent)

  if ($sourceBitmap) {
    $g.DrawImage($sourceBitmap, 0, 0, $Size, $Size)
    $g.Dispose()
    return $bmp
  }

  $u = $Size / 1024.0   # design units -> pixels
  Add-Tile $g $u
  switch ($Design) {
    'rows'  { Add-MarkRows  $g $u }
    'deck'  { Add-MarkDeck  $g $u }
    'panes' { Add-MarkPanes $g $u }
  }

  $g.Dispose()
  return $bmp
}

# --- derive each size from the master, or render a legacy candidate ---
$sizes = @(256, 128, 64, 48, 32, 24, 16)
$pngs = @{}
foreach ($s in $sizes) {
  $bmp = New-IconBitmap $s
  $ms = [System.IO.MemoryStream]::new()
  $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
  $pngs[$s] = $ms.ToArray()
  $bmp.Dispose(); $ms.Dispose()
}
if ($defaultOut) {
  [System.IO.File]::WriteAllBytes((Join-Path $pubDir 'appicon-256.png'), $pngs[256])
}
if (-not $defaultOut) {
  # a candidate run keeps its own 256px copy beside its master; in the build
  # dir the frontend's copy above is the only 256px file there is
  [System.IO.File]::WriteAllBytes((Join-Path $buildDir 'appicon-256.png'), $pngs[256])
}

$master = New-IconBitmap 1024
$master.Save((Join-Path $buildDir 'appicon.png'), [System.Drawing.Imaging.ImageFormat]::Png)
$master.Dispose()

# --- pack a multi-size .ico (PNG-compressed entries; Windows Vista+) ---
$order = @(256, 128, 64, 48, 32, 24, 16)
$ms = [System.IO.MemoryStream]::new()
$bw = [System.IO.BinaryWriter]::new($ms)
$bw.Write([UInt16]0)
$bw.Write([UInt16]1)
$bw.Write([UInt16]$order.Count)
$offset = 6 + 16 * $order.Count
foreach ($s in $order) {
  $data = $pngs[$s]
  $dim = [Byte]$(if ($s -ge 256) { 0 } else { $s })
  $bw.Write($dim); $bw.Write($dim)      # width, height (0 = 256)
  $bw.Write([Byte]0); $bw.Write([Byte]0)
  $bw.Write([UInt16]1); $bw.Write([UInt16]32)
  $bw.Write([UInt32]$data.Length)
  $bw.Write([UInt32]$offset)
  $offset += $data.Length
}
foreach ($s in $order) { $bw.Write($pngs[$s]) }
$bw.Flush()
[System.IO.File]::WriteAllBytes((Join-Path $winDir 'icon.ico'), $ms.ToArray())
$bw.Dispose(); $ms.Dispose()

# --- review sheet: single sizes, plus nearest-neighbour zooms of the small ones ---
if (-not [string]::IsNullOrEmpty($Sheet)) {
  if (-not (Test-Path $Sheet)) { New-Item -ItemType Directory -Force -Path $Sheet | Out-Null }
  foreach ($s in @(256, 64, 48, 32, 16)) {
    $bmp = New-IconBitmap $s
    $bmp.Save((Join-Path $Sheet ("{0}.png" -f $s)), [System.Drawing.Imaging.ImageFormat]::Png)
    if ($s -le 64) {
      $big = [System.Drawing.Bitmap]::new((256), (256), [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
      $bg = [System.Drawing.Graphics]::FromImage($big)
      $bg.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::NearestNeighbor
      $bg.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::Half
      $bg.Clear([System.Drawing.Color]::Transparent)
      $bg.DrawImage($bmp, 0, 0, 256, 256)
      $bg.Dispose()
      $big.Save((Join-Path $Sheet ("{0}-zoom.png" -f $s)), [System.Drawing.Imaging.ImageFormat]::Png)
      $big.Dispose()
    }
    $bmp.Dispose()
  }
}

Write-Host ("design:   {0}" -f $(if ($sourceBitmap) { 'generated deck master' } else { $Design }))
Write-Host ("icon:     {0} ({1} bytes; sizes {2})" -f (Join-Path $winDir 'icon.ico'), (Get-Item (Join-Path $winDir 'icon.ico')).Length, ($order -join ','))
Write-Host ("master:   {0}" -f (Join-Path $buildDir 'appicon.png'))
Write-Host ("frontend: {0}" -f $(if ($defaultOut) { Join-Path $pubDir 'appicon-256.png' } else { '(not written)' }))
if ($sourceBitmap) { $sourceBitmap.Dispose() }
