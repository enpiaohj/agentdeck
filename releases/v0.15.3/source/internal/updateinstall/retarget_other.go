//go:build !windows

package updateinstall

// RetargetShortcuts 在非 Windows 平台什么都不做：这里只发布 Windows 产物，
// 快捷方式改指向是 Windows 的概念。
func RetargetShortcuts(string, string) int { return 0 }

// ShortcutsPointingTo 在非 Windows 平台没有快捷方式可言，一律报告「没有」。
func ShortcutsPointingTo(string) bool { return false }

// Retarget 在非 Windows 平台什么都不做。
func Retarget(string, string) {}
