# -*- coding: utf-8 -*-
"""UI 自检截图:PrintWindow 抓 agentdeck-desktop 主窗口(高 DPI 感知)。
用法: python uishot.py <输出png> [宽] [高]
"""
import ctypes, ctypes.wintypes as wt, sys
import xml.etree.ElementTree as ET  # noqa: F401 (占位,避免误 import 项目模块)

import struct


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else r"D:\Temp\ui.png"
    rw = int(sys.argv[2]) if len(sys.argv) > 2 else 0
    rh = int(sys.argv[3]) if len(sys.argv) > 3 else 0

    u32 = ctypes.windll.user32
    g32 = ctypes.windll.gdi32
    u32.SetProcessDPIAware()

    import subprocess
    si = subprocess.STARTUPINFO()
    # 通过 powershell 单行拿到句柄最省事:避免 Python 端 EnumWindows
    r = subprocess.run(
        ["powershell", "-NoProfile", "-Command",
         "(Get-Process agentdeck-desktop).MainWindowHandle.ToString()"],
        capture_output=True, text=True)
    hwnd = int(r.stdout.strip())
    if not hwnd:
        raise SystemExit("no main window")

    u32.ShowWindow(hwnd, 9)  # SW_RESTORE
    if rw > 0 and rh > 0:
        u32.MoveWindow(hwnd, 0, 0, rw, rh, True)
        import time; time.sleep(0.8)
    u32.SetForegroundWindow(hwnd)
    import time; time.sleep(0.6)

    rect = wt.RECT()
    u32.GetWindowRect(hwnd, ctypes.byref(rect))
    w, h = rect.right - rect.left, rect.bottom - rect.top
    if w <= 0 or h <= 0:
        raise SystemExit(f"bad rect {w}x{h}")

    # 屏幕 BitBlt:WebView2 走 DirectComposition,PrintWindow 常抓不全合成层,
    # 窗口已前台时从屏幕 DC 抓更可靠。
    src_dc = u32.GetDC(0)
    hdc = u32.GetWindowDC(hwnd)
    mem = g32.CreateCompatibleDC(hdc)
    bmp = g32.CreateCompatibleBitmap(hdc, w, h)
    g32.SelectObject(mem, bmp)
    ok = g32.BitBlt(mem, 0, 0, w, h, src_dc, rect.left, rect.top, 0x00CC0020)  # SRCCOPY

    class BMPINFOHEADER(ctypes.Structure):
        _fields_ = [("biSize", ctypes.c_uint32), ("biWidth", ctypes.c_int32),
                    ("biHeight", ctypes.c_int32), ("biPlanes", ctypes.c_uint16),
                    ("biBitCount", ctypes.c_uint16), ("biCompression", ctypes.c_uint32),
                    ("biSizeImage", ctypes.c_uint32), ("biXPelsPerMeter", ctypes.c_int32),
                    ("biYPelsPerMeter", ctypes.c_int32), ("biClrUsed", ctypes.c_uint32),
                    ("biClrImportant", ctypes.c_uint32)]
    bi = BMPINFOHEADER()
    bi.biSize = ctypes.sizeof(BMPINFOHEADER)
    bi.biWidth = w
    bi.biHeight = -h
    bi.biPlanes = 1
    bi.biBitCount = 32
    bi.biCompression = 0
    buf = ctypes.create_string_buffer(w * h * 4)
    g32.GetDIBits(mem, bmp, 0, h, buf, ctypes.byref(bi), 0)

    # BGRA -> PNG(RGBA)。GDI 位图是 BGRA 且 alpha 常为 0,
    # 不换通道会让蓝色显示成橙色。
    def bgra_to_rgba(b):
        out = bytearray(len(b))
        out[0::4] = b[2::4]
        out[1::4] = b[1::4]
        out[2::4] = b[0::4]
        out[3::4] = b"\xff" * (len(b) // 4)
        return bytes(out)

    import zlib
    raw = b"".join(b"\x00" + bgra_to_rgba(buf[i * w * 4:(i + 1) * w * 4]) for i in range(h))
    comp = zlib.compress(raw, 6)

    def chunk(tag, data):
        c = struct.pack(">I", len(data)) + tag + data
        return c + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)

    png = (b"\x89PNG\r\n\x1a\n"
           + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0))
           + chunk(b"IDAT", comp) + chunk(b"IEND", b""))
    with open(out, "wb") as f:
        f.write(png)

    g32.DeleteObject(bmp); g32.DeleteDC(mem); u32.ReleaseDC(hwnd, hdc)
    if not ok:
        print("WARN: PrintWindow returned false")
    print(f"saved {out} ({w}x{h})")


if __name__ == "__main__":
    main()
