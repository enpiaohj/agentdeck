package app

import (
	"errors"
	"os"
	"path/filepath"
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
	name, args := terminalCommand(dir, "claude --resume x", exe)
	switch filepath.Base(name) {
	case "wt.exe":
		if args[2] != exe {
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
