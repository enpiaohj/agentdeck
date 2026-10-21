//go:build !windows

package app

// exeVersion 在非 Windows 上没有 VERSIONINFO 可读，返回空串，由调用方
// 按「开发版本」处理。AgentDeck 的正式产物只有 Windows x64。
func exeVersion() string { return "" }
