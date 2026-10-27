package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
)

// powershellExe 是既有终端断言使用的默认 shell:引入可配置 shell 之前
// 写死的 powershell.exe,现在由解析结果给出。
const powershellExe = "powershell.exe"

// fakeVersion 是测试里的版本探测答案:不起真进程,也不依赖本机装了什么。
func fakeVersion(version string) func(string) (string, error) {
	return func(string) (string, error) { return version, nil }
}

// TestResolveShellExeCustomPath 自定义路径必须是存在的普通文件。
func TestResolveShellExeCustomPath(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "powershell-custom.exe")
	if err := os.WriteFile(exe, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveShellExe(exe)
	if err != nil || got != exe {
		t.Fatalf("自定义路径应原样返回: %q, %v", got, err)
	}
	// 目录不是可执行文件
	if _, err = resolveShellExe(t.TempDir()); err == nil {
		t.Fatal("目录应被拒绝")
	}
	// 不存在的路径
	if _, err = resolveShellExe(filepath.Join(t.TempDir(), "NoSuch.exe")); err == nil {
		t.Fatal("不存在的路径应被拒绝")
	}
}

// TestResolveShellExePresets 预置取值解析到各自的可执行文件;Windows
// PowerShell 解析不到时仍给裸名,保持引入本设置之前的行为。
func TestResolveShellExePresets(t *testing.T) {
	exe, err := resolveShellExe(appconfig.ShellPowerShell)
	if err != nil || filepath.Base(exe) != "powershell.exe" {
		t.Fatalf("5.1 解析 = %q, %v", exe, err)
	}
	exe, err = resolveShellExe(appconfig.ShellAuto)
	if err != nil || !strings.HasSuffix(strings.ToLower(exe), ".exe") {
		t.Fatalf("自动解析 = %q, %v", exe, err)
	}
	// 空值按默认(自动,优先 7+)处理,与「自动」解析到同一个可执行文件
	auto := exe
	exe, err = resolveShellExe("")
	if err != nil || exe != auto {
		t.Fatalf("空取值应与自动一致: %q, %v(自动=%q)", exe, err, auto)
	}
}

// TestTerminalCommandUsesChosenShell 选定的 shell 同时用于 wt 的内层命令
// 与无 wt 的回退进程——两条路径都必须走同一个可执行文件。
func TestTerminalCommandUsesChosenShell(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "pwsh.exe")
	name, args := terminalCommand(dir, "claude --resume x", exe, false, false)
	switch filepath.Base(name) {
	case "wt.exe":
		// wt 的参数形如 [-M] [-w 0 nt] -d <dir> <shell> ...，shell 的位置随选项浮动
		if !slices.Contains(args, exe) {
			t.Fatalf("wt 的内层 shell 未使用所选可执行文件: %v", args)
		}
	case "pwsh.exe":
		if name != exe {
			t.Fatalf("回退进程未使用所选可执行文件: %q", name)
		}
	default:
		t.Fatalf("终端 = %s", name)
	}
}

// TestSetTerminalShellRejectsInvalid 无效取值不写文件、不改内存:设置页
// 保存的必须是真正能启动的那个 shell。
func TestSetTerminalShellRejectsInvalid(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.probeShell = fakeVersion("7.6.7")
	svc := a.NewSettingsService()

	if _, err := svc.SetTerminalShell("bash"); err == nil {
		t.Fatal("非绝对路径的未知取值应被拒绝")
	}
	if got := a.terminalShellSetting(); got != appconfig.ShellAuto {
		t.Fatalf("拒绝后取值不应改变: %q", got)
	}
	missing := filepath.Join(t.TempDir(), "NoSuch.exe")
	if _, err := svc.SetTerminalShell(missing); err == nil {
		t.Fatal("不存在的自定义路径应被拒绝")
	}
}

// TestSetTerminalShellReportsView 保存成功后,返回的视图里当前取值与
// 可执行文件一致,预置选项各自带有版本或不可用原因。断言不依赖本机是否
// 装了 PowerShell 7:pwsh 选项允许不可用。
func TestSetTerminalShellReportsView(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.probeShell = fakeVersion("7.6.7")
	svc := a.NewSettingsService()

	view, err := svc.SetTerminalShell(appconfig.ShellAuto)
	if err != nil {
		t.Fatal(err)
	}
	if view.Setting != appconfig.ShellAuto || view.Exe == "" || view.Version != "7.6.7" {
		t.Fatalf("视图 = %+v", view)
	}
	if a.terminalShellSetting() != appconfig.ShellAuto {
		t.Fatalf("内存取值未更新: %q", a.terminalShellSetting())
	}
	// 写盘后重新读取必须一致(重启后生效的就是这个值)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TerminalShell != appconfig.ShellAuto {
		t.Fatalf("配置文件里的取值 = %q", cfg.TerminalShell)
	}
	if len(view.Options) != len(shellValues) {
		t.Fatalf("预置选项数 = %d", len(view.Options))
	}
	for _, o := range view.Options {
		// 两个预置值在 Windows 上一定有可执行文件:5.1 在系统目录里,
		// 自动会回退到它。版本探测已被换成假答案,于是只可能因解析失败
		// 而为空——那正是要抓的错。
		if o.Value == appconfig.ShellPowerShell || o.Value == appconfig.ShellAuto {
			if o.Exe == "" || o.Version != "7.6.7" {
				t.Fatalf("预置选项 %s 未解析出可执行文件: %+v", o.Value, o)
			}
		}
	}
}

// TestSetTerminalShellRefusesWhenConfigUnreadable 配置读不出来时不得回写,
// 否则会用默认值覆盖用户还没修复的文件(其中可能还有本版本不认识的字段)。
func TestSetTerminalShellRefusesWhenConfigUnreadable(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.probeShell = fakeVersion("7.6.7")
	a.settingsErr = errors.New("配置文件不是有效的 JSON")
	svc := a.NewSettingsService()

	// 用一个临时目录里的可执行文件:解析与探测都会成功,唯一会失败的就是
	// 写回配置这一步,断言才真的落在「配置不可读时拒绝写入」上。
	exe := filepath.Join(t.TempDir(), "powershell-custom.exe")
	if err := os.WriteFile(exe, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetTerminalShell(exe); err == nil {
		t.Fatal("配置不可读时应拒绝写入")
	}
	if got := a.terminalShellSetting(); got == exe {
		t.Fatal("被拒绝的设置不应生效")
	}
}

// TestTerminalCommandMaximize 最大化只改显示方式，不改恢复命令本身：
// 有 wt.exe 时交给它的 -M；回退到直接起 shell 时用 -WindowStyle Maximized。
// 两条路都必须仍然带 -EncodedCommand，且脚本原文不泄漏。
func TestTerminalCommandMaximize(t *testing.T) {
	dir := t.TempDir()
	script := "Set-Location -LiteralPath '" + dir + "'; codex resume 01a13d4d-0000-7000-8000-000000000000"
	for _, maximize := range []bool{false, true} {
		name, args := terminalCommand(dir, script, powershellExe, maximize, false)
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-EncodedCommand") {
			t.Fatalf("maximize=%v 必须用 -EncodedCommand: %v", maximize, args)
		}
		if got := args[len(args)-1]; got != encodedCommand(script) {
			t.Fatalf("maximize=%v 编码不一致", maximize)
		}
		for _, a := range args {
			if strings.Contains(a, ";") || strings.Contains(a, "resume") {
				t.Fatalf("maximize=%v 脚本原文泄漏到终端参数: %q", maximize, a)
			}
		}
		switch filepath.Base(name) {
		case "wt.exe":
			if maximize != (args[0] == "-M") {
				t.Fatalf("maximize=%v 时 wt 参数 = %v", maximize, args)
			}
			if maximize && (args[1] != "-d" || args[2] != dir) {
				t.Fatalf("-M 之后的 -d 必须紧跟目录: %v", args)
			}
		case "powershell.exe":
			has := args[0] == "-WindowStyle" && args[1] == "Maximized"
			if maximize != has {
				t.Fatalf("maximize=%v 时回退 shell 参数 = %v", maximize, args)
			}
		default:
			t.Fatalf("终端 = %s", name)
		}
	}
}

// TestResumeMaximizedDefaultOff 最大化默认关闭：旧配置没有该字段，新装的
// 用户也不该被替用户铺满屏幕。
func TestResumeMaximizedDefaultOff(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	if a.resumeMaximized() {
		t.Fatal("默认应为关闭")
	}
	if view := a.NewSettingsService().TerminalShell(); view.ResumeMaximized {
		t.Fatal("视图里的默认值应为关闭")
	}
}

// TestSetResumeMaximizedPersists 保存后立即生效并写盘，且不覆盖终端 shell
// 等其它设置。
func TestSetResumeMaximizedPersists(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.probeShell = fakeVersion("7.6.7")
	svc := a.NewSettingsService()
	if _, err := svc.SetTerminalShell(appconfig.ShellPwsh); err != nil {
		t.Fatal(err)
	}
	view, err := svc.SetResumeMaximized(true)
	if err != nil {
		t.Fatal(err)
	}
	if !view.ResumeMaximized || !a.resumeMaximized() {
		t.Fatalf("保存后未生效: %+v", view)
	}
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ResumeMaximized || cfg.TerminalShell != appconfig.ShellPwsh {
		t.Fatalf("配置文件 = %+v", cfg)
	}
	if view, err = svc.SetResumeMaximized(false); err != nil || view.ResumeMaximized {
		t.Fatalf("关闭后 = %+v, %v", view, err)
	}
}

// TestSetResumeMaximizedRefusesWhenConfigUnreadable 配置读不出来时不得
// 回写，否则会用默认值覆盖用户还没修复的文件。
func TestSetResumeMaximizedRefusesWhenConfigUnreadable(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.settingsErr = errors.New("配置文件不是有效的 JSON")
	if _, err := a.NewSettingsService().SetResumeMaximized(true); err == nil {
		t.Fatal("配置不可读时应拒绝写入")
	}
	if a.resumeMaximized() {
		t.Fatal("被拒绝的设置不应生效")
	}
}

// TestTerminalCommandTabMode 新标签模式用 wt 的 -w 0 nt：-w 0 指向「当前活动
// 窗口」，nt 是 new-tab。实测 3 次调用只多出 1 个窗口、窗口里 3 个标签。
// 关键约束是参数顺序：wt 先吃全局选项，-M 必须在 nt 之前才生效。
func TestTerminalCommandTabMode(t *testing.T) {
	dir := t.TempDir()
	script := "Set-Location -LiteralPath '" + dir + "'; claude --resume 01a13d4d-0000-7000-8000-000000000000"
	name, args := terminalCommand(dir, script, powershellExe, false, true)
	if filepath.Base(name) == "wt.exe" {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-w 0 nt") {
			t.Fatalf("新标签模式应带 -w 0 nt: %v", args)
		}
		if idx := slices.Index(args, "nt"); idx < 0 || args[idx+1] != "-d" {
			t.Fatalf("nt 之后应紧跟 -d: %v", args)
		}
	} else if filepath.Base(name) == "powershell.exe" {
		// 没有 wt.exe 时只能开新窗口：不得出现任何「复用窗口」的痕迹
		if slices.Contains(args, "nt") || slices.Contains(args, "-w") {
			t.Fatalf("回退路径不应带 wt 专用参数: %v", args)
		}
	} else {
		t.Fatalf("终端 = %s", name)
	}
	if got := args[len(args)-1]; got != encodedCommand(script) {
		t.Fatal("新标签模式仍必须是不可拆的 -EncodedCommand")
	}

	// 最大化 + 新标签：-M 必须排在 nt 之前，否则 wt 把它当成 new-tab 的参数
	name, args = terminalCommand(dir, script, powershellExe, true, true)
	if filepath.Base(name) == "wt.exe" {
		nt := slices.Index(args, "nt")
		m := slices.Index(args, "-M")
		if m < 0 || nt < 0 || m > nt {
			t.Fatalf("-M 必须出现在 nt 之前: %v", args)
		}
	}
}

// TestResumeOpenModeDefaultWindow 打开方式默认是「新窗口」，与引入设置之前一致。
func TestResumeOpenModeDefaultWindow(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	if a.resumeOpenInTab() {
		t.Fatal("默认应为新窗口")
	}
	view := a.NewSettingsService().TerminalShell()
	if view.ResumeOpenMode != appconfig.TerminalWindow || !view.TabSupported {
		t.Fatalf("视图 = %+v（本机有 wt.exe，TabSupported 应为真）", view)
	}
}

// TestSetResumeOpenModePersists 保存后立即生效并写盘，非法取值回退到新窗口。
func TestSetResumeOpenModePersists(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.probeShell = fakeVersion("7.6.7")
	svc := a.NewSettingsService()
	view, err := svc.SetResumeOpenMode(appconfig.TerminalTab)
	if err != nil {
		t.Fatal(err)
	}
	if view.ResumeOpenMode != appconfig.TerminalTab || !a.resumeOpenInTab() {
		t.Fatalf("保存后未生效: %+v", view)
	}
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ResumeOpenMode != appconfig.TerminalTab {
		t.Fatalf("配置文件 = %q", cfg.ResumeOpenMode)
	}
	// 无法识别的取值按「新窗口」处理，不写进一个界面不认识的值
	if view, err = svc.SetResumeOpenMode("bogus"); err != nil || view.ResumeOpenMode != appconfig.TerminalWindow {
		t.Fatalf("非法取值应回退新窗口: %+v, %v", view, err)
	}
}

// TestSetResumeOpenModeRejectsTabWithoutWT 没有 Windows Terminal 时拒绝保存
// 「新标签」：否则界面会显示一个永远不生效的选择。
func TestSetResumeOpenModeRejectsTabWithoutWT(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.probeShell = fakeVersion("7.6.7")
	// 用一个不含 wt.exe 的 PATH 让 LookPath 找不到它
	t.Setenv("PATH", t.TempDir())
	if _, err := a.NewSettingsService().SetResumeOpenMode(appconfig.TerminalTab); err == nil {
		t.Fatal("没有 wt.exe 时应拒绝保存新标签模式")
	}
	if a.resumeOpenInTab() {
		t.Fatal("被拒绝的设置不应生效")
	}
}
