# gen_appicon.ps1 -- draw the AgentDeck application icon and pack it.
#
# Design ("all your coding agents, one deck"): a rounded tile with an azure ->
# indigo gradient, a soft top gloss and a bottom vignette, holding four white
# panes of unequal width -- the app's four-column workspace -- with a spark in
# the content pane marking the agent that is working.
#
# This is the first design drawn for the app and the one kept (chosen over a
# two-card deck and a 2x2 grid: four panes of unequal width are what the
# workspace actually looks like, and the widow/column rhythm still reads at
# 32px). Everything is drawn here, at every size, so nothing is resampled and
# the 16px entry stays crisp. The 256px render is copied into the frontend so
# the in-app brand mark and the executable icon can never drift apart.
#
# Usage:  powershell -NoProfile -ExecutionPolicy Bypass -File tools\gen_appicon.ps1
# Output: build/appicon.png            1024px master
#         build/windows/icon.ico       multi-size icon (256..16)
#         frontend/public/appicon-256.png
#
# After changing this file the linker resource must be regenerated:
#     wails3 generate syso -icon build/windows/icon.ico -info build/windows/info.json `
#         -manifest build/windows/wails.exe.manifest -arch amd64 `
#         -out cmd/agentdeck-desktop/resource_windows_amd64.syso
#
# ASCII only on purpose: Windows PowerShell 5.1 reads .ps1 as the ANSI code page.

Add-Type -AssemblyName System.Drawing

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
if (-not $root) { $root = (Get-Location).Path }

$buildDir = Join-Path $root 'build'
$winDir   = Join-Path $buildDir 'windows'
$pubDir   = Join-Path $root 'frontend\public'
foreach ($d in @($buildDir, $winDir, $pubDir)) {
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

# Pane / tile geometry in the 1024 design grid (see the design note above).
$PANE_W = @(128.0, 176.0, 240.0, 176.0)   # nav | list | content | info
$PANE_X = @(114.0, 276.0, 486.0, 760.0)
$PANE_Y = 286.0
$PANE_H = 452.0
$PANE_R = 34.0

function New-IconBitmap {
  param([int]$Size)

  $bmp = [System.Drawing.Bitmap]::new($Size, $Size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.SmoothingMode     = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
  $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
  $g.PixelOffsetMode   = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
  $g.Clear([System.Drawing.Color]::Transparent)

  $u = $Size / 1024.0   # design units -> pixels

  # --- tile ---
  $tile = New-RoundedPath (40 * $u) (40 * $u) (944 * $u) (944 * $u) (210 * $u)
  $grad = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
    [System.Drawing.PointF]::new([float](64 * $u), [float](64 * $u)),
    [System.Drawing.PointF]::new([float](960 * $u), [float](960 * $u)),
    [System.Drawing.Color]::FromArgb(255, 92, 162, 255),
    [System.Drawing.Color]::FromArgb(255, 26, 52, 190))
  $g.FillPath($grad, $tile)

  $g.SetClip($tile)
  # top gloss
  $gloss = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
    [System.Drawing.PointF]::new(0, [float](40 * $u)),
    [System.Drawing.PointF]::new(0, [float](620 * $u)),
    [System.Drawing.Color]::FromArgb(66, 255, 255, 255),
    [System.Drawing.Color]::FromArgb(0, 255, 255, 255))
  $g.FillRectangle($gloss, [float](40 * $u), [float](40 * $u), [float](944 * $u), [float](580 * $u))
  # bottom vignette
  $vig = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
    [System.Drawing.PointF]::new(0, [float](660 * $u)),
    [System.Drawing.PointF]::new(0, [float](984 * $u)),
    [System.Drawing.Color]::FromArgb(0, 12, 20, 60),
    [System.Drawing.Color]::FromArgb(52, 12, 20, 60))
  $g.FillRectangle($vig, [float](40 * $u), [float](660 * $u), [float](944 * $u), [float](324 * $u))
  $g.ResetClip()

  # --- soft shadow lifting the pane row off the tile ---
  $shadow = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(26, 8, 20, 70))
  foreach ($dy in @(6.0, 12.0)) {
    $sp = New-RoundedPath (114 * $u) (($PANE_Y + $dy) * $u) (822 * $u) ($PANE_H * $u) ($PANE_R * $u)
    $g.FillPath($shadow, $sp)
    $sp.Dispose()
  }
  $shadow.Dispose()

  # --- four panes ---
  $white = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::White)
  for ($i = 0; $i -lt 4; $i++) {
    $p = New-RoundedPath ($PANE_X[$i] * $u) ($PANE_Y * $u) ($PANE_W[$i] * $u) ($PANE_H * $u) ($PANE_R * $u)
    $g.FillPath($white, $p)
    $p.Dispose()
  }

  # --- spark, centred in the content pane ---
  $blue = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(255, 46, 104, 226))
  $spark = New-SparkPath ((($PANE_X[2] + $PANE_W[2] / 2)) * $u) ((($PANE_Y + $PANE_H / 2)) * $u) (72 * $u) (17 * $u)
  $g.FillPath($blue, $spark)

  $spark.Dispose(); $blue.Dispose(); $white.Dispose()
  $grad.Dispose(); $gloss.Dispose(); $vig.Dispose(); $tile.Dispose()
  $g.Dispose()
  return $bmp
}

# --- render every size natively ---
$sizes = @(256, 128, 64, 48, 32, 24, 16)
$pngs = @{}
foreach ($s in $sizes) {
  $bmp = New-IconBitmap $s
  $ms = [System.IO.MemoryStream]::new()
  $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
  $pngs[$s] = $ms.ToArray()
  $bmp.Dispose(); $ms.Dispose()
}
[System.IO.File]::WriteAllBytes((Join-Path $pubDir 'appicon-256.png'), $pngs[256])

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

Write-Host ("icon:     {0} ({1} bytes; sizes {2})" -f (Join-Path $winDir 'icon.ico'), (Get-Item (Join-Path $winDir 'icon.ico')).Length, ($order -join ','))
Write-Host ("master:   {0}" -f (Join-Path $buildDir 'appicon.png'))
Write-Host ("frontend: {0}" -f (Join-Path $pubDir 'appicon-256.png'))
