//go:build !windows

package updateinstall

// RetargetShortcuts 在非 Windows 平台什么都不做：这里只发布 Windows 产物，
// 快捷方式改指向是 Windows 的概念。
func RetargetShortcuts(string, string) int { return 0 }

// Retarget 在非 Windows 平台什么都不做。
func Retarget(string, string) {}
