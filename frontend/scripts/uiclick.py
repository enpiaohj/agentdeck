# -*- coding: utf-8 -*-
"""模拟鼠标点击(物理像素坐标)+ 截图,验证详情渲染。"""
import ctypes, time, subprocess, sys

x, y = int(sys.argv[1]), int(sys.argv[2])
out = sys.argv[3] if len(sys.argv) > 3 else r"D:\Temp\ui-click.png"

u32 = ctypes.windll.user32
u32.SetProcessDPIAware()

r = subprocess.run(["powershell", "-NoProfile", "-Command",
                    "(Get-Process agentdeck-desktop).MainWindowHandle.ToString()"],
                   capture_output=True, text=True)
hwnd = int(r.stdout.strip())
rect = ctypes.wintypes.RECT() if False else None
import ctypes.wintypes as wt
rect = wt.RECT()
u32.GetWindowRect(hwnd, ctypes.byref(rect))
u32.SetForegroundWindow(hwnd)
time.sleep(0.5)

u32.SetCursorPos(rect.left + x, rect.top + y)
# 第一次点击可能只用于激活窗口,连点两次确保传到 React
for _ in range(2):
    time.sleep(0.25)
    u32.mouse_event(2, 0, 0, 0, 0)   # LEFTDOWN
    u32.mouse_event(4, 0, 0, 0, 0)   # LEFTUP
time.sleep(2.5)

subprocess.run(["python", "frontend/scripts/uishot.py", out], cwd=r"D:\Work\AgentDeck")
