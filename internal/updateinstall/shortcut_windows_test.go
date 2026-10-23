//go:build windows

package updateinstall

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// 用真实的 WScript.Shell 创建一个 .lnk，再走 RetargetShortcuts 改指向，
// 最后用 COM 读回目标，确认「真的改了」而不是「函数返回了 1」。
//
// 快捷方式放在临时目录，通过 retargetShortcutsIn 注入目录列表，不碰用户
// 真实的桌面与开始菜单。
func TestRetargetShortcutsReallyChangesTarget(t *testing.T) {
	dir := t.TempDir()
	oldExe := filepath.Join(dir, "AgentDeck-v0.14.0-win-x64.exe")
	newExe := filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.exe")
	for _, p := range []string{oldExe, newExe} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "AgentDeck.lnk")
	if !createShortcut(t, link, oldExe) {
		t.Skip("本机无法创建快捷方式（WScript.Shell 不可用），跳过")
	}
	// 先确认它确实指向旧文件。
	if got := shortcutTarget(t, link); !samePath(got, oldExe) {
		t.Fatalf("构造的快捷方式指向 %q，期望 %q", got, oldExe)
	}

	if n := retargetShortcutsIn([]string{dir}, oldExe, newExe); n != 1 {
		t.Fatalf("应当改指向 1 个快捷方式，实际 %d", n)
	}
	if got := shortcutTarget(t, link); !samePath(got, newExe) {
		t.Fatalf("改指向后是 %q，期望 %q", got, newExe)
	}

	// 已经指向新文件时再跑一次不应改动任何东西。
	if n := retargetShortcutsIn([]string{dir}, oldExe, newExe); n != 0 {
		t.Fatalf("已指向新文件时不应再改，实际改了 %d 个", n)
	}
}

// 指向别的程序的快捷方式一概不碰。
func TestRetargetLeavesUnrelatedShortcuts(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "OtherApp.exe")
	oldExe := filepath.Join(dir, "AgentDeck-v0.14.0-win-x64.exe")
	newExe := filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.exe")
	for _, p := range []string{other, oldExe, newExe} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "Other.lnk")
	if !createShortcut(t, link, other) {
		t.Skip("本机无法创建快捷方式，跳过")
	}
	if n := retargetShortcutsIn([]string{dir}, oldExe, newExe); n != 0 {
		t.Fatalf("不相关的快捷方式被改了：%d 个", n)
	}
	if got := shortcutTarget(t, link); !samePath(got, other) {
		t.Fatalf("不相关快捷方式被改动：%q", got)
	}
}

func createShortcut(t *testing.T, link, target string) bool {
	t.Helper()
	uninit, ok := comReady()
	if !ok {
		return false
	}
	defer uninit()
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return false
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return false
	}
	defer shell.Release()
	shortcut, err := oleutil.CallMethod(shell, "CreateShortcut", link)
	if err != nil {
		return false
	}
	dispatch := shortcut.ToIDispatch()
	if dispatch == nil {
		return false
	}
	defer dispatch.Release()
	if _, err = oleutil.PutProperty(dispatch, "TargetPath", target); err != nil {
		return false
	}
	_, _ = oleutil.PutProperty(dispatch, "WorkingDirectory", filepath.Dir(target))
	_, err = oleutil.CallMethod(dispatch, "Save")
	return err == nil
}

func shortcutTarget(t *testing.T, link string) string {
	t.Helper()
	uninit, ok := comReady()
	if !ok {
		return ""
	}
	defer uninit()
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return ""
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return ""
	}
	defer shell.Release()
	shortcut, err := oleutil.CallMethod(shell, "CreateShortcut", link)
	if err != nil {
		return ""
	}
	dispatch := shortcut.ToIDispatch()
	if dispatch == nil {
		return ""
	}
	defer dispatch.Release()
	value, err := oleutil.GetProperty(dispatch, "TargetPath")
	if err != nil {
		return ""
	}
	out := value.ToString()
	value.Clear()
	return out
}

// comOutcome 判定 CoInitializeEx 的结果：COM 是否可用，以及本次是否增加了
// 引用计数（只有增加过才该调 CoUninitialize）。
//
// S_FALSE 与 RPC_E_CHANGED_MODE 都表示「线程已初始化过」，本次没有增加计数：
// 前者是重复初始化，后者是线程已被初始化成另一种单元模型（WebView2 会把主线程
// 初始化成 MTA，这是 AgentDeck 里的常态）。两种情况都必须返回 counted=false，
// 否则调用方一执行 uninit 就减掉别人的计数。
func TestComOutcome(t *testing.T) {
	for _, c := range []struct {
		name    string
		hr      uintptr
		counted bool
		ok      bool
	}{
		{"成功初始化", 0, true, true},
		{"S_FALSE 重复初始化", 0x00000001, false, true},
		{"RPC_E_CHANGED_MODE 单元模型不符", 0x80010106, false, true},
		{"其他失败", 0x80004005, false, false},
	} {
		var err error
		if c.hr != 0 {
			err = ole.NewError(c.hr)
		}
		counted, ok := comOutcome(err)
		if counted != c.counted || ok != c.ok {
			t.Fatalf("%s：counted=%v ok=%v，期望 counted=%v ok=%v",
				c.name, counted, ok, c.counted, c.ok)
		}
	}
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	clean := func(p string) string { return filepath.Clean(filepath.FromSlash(p)) }
	return equalFoldPath(clean(a), clean(b))
}

func equalFoldPath(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
