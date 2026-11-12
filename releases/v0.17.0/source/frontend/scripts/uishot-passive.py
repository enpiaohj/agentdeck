# -*- coding: utf-8 -*-
"""Screenshot the app window WITHOUT clicking anywhere.

uishot.py clicks the title bar first (to dismiss the IME floating bar), which
closes an open dialog. For dialog checks use this: same capture path, no input.
"""
import ctypes
import ctypes.wintypes as wt
import os
import struct
import subprocess
import sys
import time


def find_hwnd():
    ps = ("(Get-Process | Where-Object { $_.ProcessName -like 'agentdeck*' -and "
          "$_.MainWindowHandle -ne 0 } | Select-Object -First 1 -Expand MainWindowHandle)")
    r = subprocess.run(["powershell", "-NoProfile", "-Command", ps],
                       capture_output=True, text=True)
    s = r.stdout.strip()
    return int(s) if s else 0


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.environ.get("TEMP", "."), "ui-passive.png")
    u32 = ctypes.windll.user32
    g32 = ctypes.windll.gdi32
    u32.SetProcessDPIAware()

    hwnd = find_hwnd()
    if not hwnd:
        raise SystemExit("no main window")
    for _ in range(24):
        if u32.IsWindowVisible(hwnd):
            break
        time.sleep(0.25)

    rect = wt.RECT()
    u32.GetWindowRect(hwnd, ctypes.byref(rect))
    w, h = rect.right - rect.left, rect.bottom - rect.top
    if w <= 0 or h <= 0:
        raise SystemExit(f"bad rect {w}x{h}")

    src = u32.GetWindowDC(0)
    mem = g32.CreateCompatibleDC(src)
    bmp = g32.CreateCompatibleBitmap(src, w, h)
    g32.SelectObject(mem, bmp)
    g32.BitBlt(mem, 0, 0, w, h, src, rect.left, rect.top, 0x00CC0020)

    class BMIH(ctypes.Structure):
        _fields_ = [("biSize", wt.DWORD), ("biWidth", wt.LONG), ("biHeight", wt.LONG),
                    ("biPlanes", wt.WORD), ("biBitCount", wt.WORD),
                    ("biCompression", wt.DWORD), ("biSizeImage", wt.DWORD),
                    ("biXPelsPerMeter", wt.LONG), ("biYPelsPerMeter", wt.LONG),
                    ("biClrUsed", wt.DWORD), ("biClrImportant", wt.DWORD)]

    bi = BMIH()
    bi.biSize = ctypes.sizeof(BMIH)
    bi.biWidth, bi.biHeight = w, -h
    bi.biPlanes, bi.biBitCount = 1, 32
    bi.biCompression = 0
    buf = ctypes.create_string_buffer(w * h * 4)
    g32.GetDIBits(mem, bmp, 0, h, buf, ctypes.byref(bi), 0)

    # GDI hands back BGRA; PNG wants RGBA
    data = bytearray(buf.raw)
    data[0::4], data[2::4] = data[2::4], data[0::4]

    def chunk(tag, body):
        crc = zlib.crc32(tag + body) & 0xFFFFFFFF
        return (struct.pack(">I", len(body)) + tag + body + struct.pack(">I", crc))

    import zlib
    raw = b"".join(b"\x00" + bytes(data[y * w * 4:(y + 1) * w * 4]) for y in range(h))
    png = (b"\x89PNG\r\n\x1a\n" +
           chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0)) +
           chunk(b"IDAT", zlib.compress(raw, 6)) + chunk(b"IEND", b""))
    open(out, "wb").write(png)
    g32.DeleteObject(bmp)
    g32.DeleteDC(mem)
    u32.ReleaseDC(0, src)
    print(f"saved {out} ({w}x{h})")


main()
