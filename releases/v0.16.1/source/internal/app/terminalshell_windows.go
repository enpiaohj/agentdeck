package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
	"github.com/enpiaohj/agentdeck/internal/proc"
)

// 本文件解析「设置 → 新建与启动 → 终端 PowerShell」：把配置取值变成实际
// 启动的可执行文件，并探测其版本供界面展示。
//
// 取值格式由 appconfig.NormalizeShell 定义：powershell（Windows PowerShell
// 5.1）、pwsh（PowerShell 7+）、auto（优先 7+，不可用时回退 5.1），以及
// 自定义可执行文件的绝对路径。

// psVersionCommand 让 shell 自报版本。两个版本都支持 -EncodedCommand 与
// 这段表达式；-NoProfile 避免加载用户配置拖慢或污染输出。
const psVersionCommand = "$PSVersionTable.PSVersion.ToString()"

// shellVersion 是一次版本探测的结果。
type shellVersion struct {
	version string
	err     error
}

// resolveShellExe 把终端 shell 取值解析成可执行文件路径。
//
// powershell 解析不到时仍返回裸名 powershell.exe：系统一定装有它，裸名交给
// 启动阶段解析即可，也保持了引入本设置之前的行为。pwsh 与自定义路径解析
// 不到则报错——用户明确选择了它们，静默换成别的 shell 与选择不符。
func resolveShellExe(setting string) (string, error) {
	switch appconfig.NormalizeShell(setting) {
	case appconfig.ShellPowerShell:
		if p := proc.FindTool("powershell"); p != "" {
			return p, nil
		}
		return "powershell.exe", nil
	case appconfig.ShellPwsh:
		if p := proc.FindTool("pwsh"); p != "" {
			return p, nil
		}
		return "", fmt.Errorf("未找到 PowerShell 7（pwsh.exe）；可在「设置 → 新建与启动」中改用 Windows PowerShell 5.1 或指定可执行文件路径")
	case appconfig.ShellAuto:
		if p := proc.FindTool("pwsh"); p != "" {
			return p, nil
		}
		if p := proc.FindTool("powershell"); p != "" {
			return p, nil
		}
		return "powershell.exe", nil
	}
	return customShellExe(setting)
}

// customShellExe 校验自定义的 shell 路径：必须是存在的普通文件。
func customShellExe(setting string) (string, error) {
	path := filepath.Clean(strings.TrimSpace(setting))
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("终端 shell 必须是绝对路径：%s", setting)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("终端 shell 路径不可用：%w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("终端 shell 路径是目录，不是可执行文件：%s", path)
	}
	return path, nil
}

// probeShellVersion 运行 shell 自报版本，超时视为不可用。
func probeShellVersion(exe string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := proc.CommandContext(ctx, exe, "-NoProfile", "-NoLogo", "-NonInteractive", "-Command", psVersionCommand).Output()
	if err != nil {
		return "", err
	}
	// 输出可能带 UTF-8 BOM（Windows PowerShell 5.1 在重定向输出时会写），
	// 不去掉会让版本号多一个不可见字符。用字节转义写，避免源码里出现裸 BOM。
	const utf8BOM = "\xef\xbb\xbf"
	version := strings.TrimSpace(strings.TrimPrefix(string(out), utf8BOM))
	if version == "" {
		return "", fmt.Errorf("未返回版本号")
	}
	return version, nil
}

// terminalShellSetting 返回终端 shell 的当前取值。
func (a *App) terminalShellSetting() string {
	a.shellMu.Lock()
	defer a.shellMu.Unlock()
	return a.shellSetting
}

// setTerminalShell 更新内存中的终端 shell 取值；写文件由 SettingsService 负责。
func (a *App) setTerminalShell(value string) {
	a.shellMu.Lock()
	defer a.shellMu.Unlock()
	a.shellSetting = appconfig.NormalizeShell(value)
}

// shellExe 返回启动终端时使用的 shell 可执行文件。
func (a *App) shellExe() (string, error) {
	return resolveShellExe(a.terminalShellSetting())
}

// resumeMaximized 报告「继续会话」是否让终端最大化打开。
//
// 取值随应用配置一起由 settingsMu 保护；配置读取失败时 settings 是 Default()，
// 这里得到 false——降级状态下不擅自替用户把窗口铺满。
func (a *App) resumeMaximized() bool {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.settings.ResumeMaximized
}

// resumeOpenMode 返回继续会话时终端的打开方式取值（已归一化）。
func (a *App) resumeOpenMode() string {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return appconfig.NormalizeOpenMode(a.settings.ResumeOpenMode)
}

// resumeOpenInTab 报告继续会话时是否在活动终端窗口里开新标签。
func (a *App) resumeOpenInTab() bool {
	return a.resumeOpenMode() == appconfig.TerminalTab
}

// tabSupported 报告当前环境能否把命令开进已有终端窗口的新标签。
//
// 只有 Windows Terminal（wt.exe）能复用窗口；没有它时回退路径只会开新窗口，
// 所以设置页要把「新标签」选项置灰，而不是让用户选了却不生效。
func tabSupported() bool {
	_, err := proc.LookPath("wt.exe")
	return err == nil
}

// shellVersionOf 返回 exe 的版本，成功结果在进程内缓存。失败不缓存：用户
// 可能正在安装或修复 shell，缓存失败会让重试永远失败。
func (a *App) shellVersionOf(exe string) (string, error) {
	a.shellMu.Lock()
	if v, ok := a.shellProbes[exe]; ok && v.err == nil {
		a.shellMu.Unlock()
		return v.version, nil
	}
	probe := a.probeShell
	a.shellMu.Unlock()
	if probe == nil {
		probe = probeShellVersion
	}
	version, err := probe(exe)
	if err != nil {
		return "", err
	}
	a.shellMu.Lock()
	if a.shellProbes == nil {
		a.shellProbes = map[string]shellVersion{}
	}
	a.shellProbes[exe] = shellVersion{version: version}
	a.shellMu.Unlock()
	return version, nil
}
