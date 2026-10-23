# -*- coding: utf-8 -*-
"""Click (optionally right-click) at window-relative physical pixels, then take
a PASSIVE screenshot so an open dialog / context menu survives.

uishot.py clicks the title bar to dismiss the IME bar, which closes any dialog.
"""
import ctypes
import ctypes.wintypes as wt
import os
import subprocess
import sys
import time

x, y = int(sys.argv[1]), int(sys.argv[2])
out = sys.argv[3] if len(sys.argv) > 3 else os.path.join(os.environ.get("TEMP", "."), "ui-click.png")
button = sys.argv[4] if len(sys.argv) > 4 else "left"

u32 = ctypes.windll.user32
u32.SetProcessDPIAware()

r = subprocess.run(["powershell", "-NoProfile", "-Command",
                    "(Get-Process | Where-Object { $_.ProcessName -like 'agentdeck*' -and "
                    "$_.MainWindowHandle -ne 0 } | Select-Object -First 1 -Expand MainWindowHandle)"],
                   capture_output=True, text=True)
hwnd = int(r.stdout.strip())
rect = wt.RECT()
u32.GetWindowRect(hwnd, ctypes.byref(rect))

VK_MENU, KEYUP = 0x12, 0x0002
for _ in range(12):
    u32.keybd_event(VK_MENU, 0, 0, 0)
    u32.keybd_event(VK_MENU, 0, KEYUP, 0)
    u32.SetForegroundWindow(hwnd)
    time.sleep(0.25)
    if u32.GetForegroundWindow() == hwnd:
        break
else:
    raise SystemExit("window not foreground")
time.sleep(0.4)

u32.SetCursorPos(rect.left + x, rect.top + y)
time.sleep(0.25)
if button == "right":
    u32.mouse_event(0x0008, 0, 0, 0, 0)   # RIGHTDOWN
    u32.mouse_event(0x0010, 0, 0, 0, 0)   # RIGHTUP
else:
    u32.mouse_event(0x0002, 0, 0, 0, 0)   # LEFTDOWN
    u32.mouse_event(0x0004, 0, 0, 0, 0)   # LEFTUP
time.sleep(1.2)

subprocess.run(["python", "-I", "frontend/scripts/uishot-passive.py", out],
               cwd=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
