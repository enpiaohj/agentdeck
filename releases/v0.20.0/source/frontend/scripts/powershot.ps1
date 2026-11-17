# 截取 agentdeck-desktop 主窗口到 PNG(PrintWindow,无需前台)。
# 用法: powershot.ps1 <输出png> [宽] [高]
param([string]$Out = "D:\Temp\ui-now.png", [int]$W = 0, [int]$H = 0)
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win {
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int cmd);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int ht, bool repaint);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[Win]::SetProcessDPIAware() | Out-Null   # 必须在取坐标前:否则 GetWindowRect 返回虚拟化坐标,高 DPI 下截图被裁
$p = Get-Process agentdeck-desktop -ErrorAction Stop
$h = $p.MainWindowHandle
if ($h -eq [IntPtr]::Zero) { throw "no main window" }
[Win]::ShowWindow($h, 9) | Out-Null   # SW_RESTORE
if ($W -gt 0 -and $H -gt 0) {
  [Win]::MoveWindow($h, 0, 0, $W, $H, $true) | Out-Null
  Start-Sleep -Milliseconds 800
}
[Win]::SetForegroundWindow($h) | Out-Null
Start-Sleep -Milliseconds 600
$r = New-Object Win+RECT
[Win]::GetWindowRect($h, [ref]$r) | Out-Null
$w = $r.R - $r.L; $ht = $r.B - $r.T
if ($w -le 0 -or $ht -le 0) { throw "bad rect ${w}x${ht}" }
$bmp = New-Object System.Drawing.Bitmap($w, $ht)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc()
[Win]::PrintWindow($h, $dc, 2) | Out-Null   # PW_RENDERFULLCONTENT
$g.ReleaseHdc($dc); $g.Dispose()
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Host "saved $Out (${w}x${ht})"
