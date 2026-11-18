//go:build windows

// 主屏工作区尺寸探测(窗口初始大小自适应用)。
package main

import "syscall"

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetDpiForSystem  = user32.NewProc("GetDpiForSystem")
)

const (
	smCXFullscreen = 16
	smCYFullscreen = 17
)

// workAreaLogical 返回主屏工作区逻辑尺寸(96 DPI 基准)。
// Wails 进程为 per-monitor DPI aware,GetSystemMetrics 返回的是物理像素,
// 需按系统 DPI 折算回逻辑值才能与 WebviewWindow 的 Width/Height(逻辑)对齐。
func workAreaLogical() (int, int) {
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		dpi = 96
	}
	w, _, _ := procGetSystemMetrics.Call(uintptr(smCXFullscreen))
	h, _, _ := procGetSystemMetrics.Call(uintptr(smCYFullscreen))
	return int(w) * 96 / int(dpi), int(h) * 96 / int(dpi)
}
