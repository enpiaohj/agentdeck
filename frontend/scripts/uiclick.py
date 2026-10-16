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
import ctypes.wintypes as wt
rect = wt.RECT()
u32.GetWindowRect(hwnd, ctypes.byref(rect))
# 置前台:先发 ALT 解除系统对后台进程抢前台的限制,再轮询确认(否则点击
# 会落到压在应用上的终端窗口里)
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
u32.mouse_event(2, 0, 0, 0, 0)   # LEFTDOWN
u32.mouse_event(4, 0, 0, 0, 0)   # LEFTUP
time.sleep(2.0)

subprocess.run(["python", "-I", "frontend/scripts/uishot.py", out], cwd=r"D:\Work\AgentDeck")
