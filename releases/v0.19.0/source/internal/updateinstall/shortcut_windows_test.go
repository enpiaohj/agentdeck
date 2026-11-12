//go:build windows

package updateinstall

import (
	"bytes"
	"errors"
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

// 开始菜单「所有应用」显示的是 .lnk 的文件名。只改目标会留下一个「指向
// v0.15.0、名字还写着 v0.14.0」的条目，用户看到的就是版本没更新。
func TestRetargetRenamesVersionedShortcut(t *testing.T) {
	dir := t.TempDir()
	oldExe := filepath.Join(dir, "AgentDeck-v0.14.0-win-x64.exe")
	newExe := filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.exe")
	for _, p := range []string{oldExe, newExe} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "AgentDeck-v0.14.0-win-x64.lnk")
	if !createShortcut(t, link, oldExe) {
		t.Skip("本机无法创建快捷方式（WScript.Shell 不可用），跳过")
	}
	if n := retargetShortcutsIn([]string{dir}, oldExe, newExe); n != 1 {
		t.Fatalf("应当改指向 1 个快捷方式，实际 %d", n)
	}
	renamed := filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.lnk")
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("版本派生的快捷方式应改名为 %q: %v", filepath.Base(renamed), err)
	}
	if _, err := os.Stat(link); !os.IsNotExist(err) {
		t.Fatalf("旧名字应当已经不存在: %v", err)
	}
	if got := shortcutTarget(t, renamed); !samePath(got, newExe) {
		t.Fatalf("改名后目标是 %q，期望 %q", got, newExe)
	}
}

// 用户自己起的名字不跟着版本改——那是他选的叫法。
func TestRetargetKeepsCustomShortcutName(t *testing.T) {
	dir := t.TempDir()
	oldExe := filepath.Join(dir, "AgentDeck-v0.14.0-win-x64.exe")
	newExe := filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.exe")
	for _, p := range []string{oldExe, newExe} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "我的工作台.lnk")
	if !createShortcut(t, link, oldExe) {
		t.Skip("本机无法创建快捷方式，跳过")
	}
	if n := retargetShortcutsIn([]string{dir}, oldExe, newExe); n != 1 {
		t.Fatalf("应当改指向 1 个快捷方式，实际 %d", n)
	}
	if _, err := os.Stat(link); err != nil {
		t.Fatalf("自定义名字不应被改动: %v", err)
	}
	if got := shortcutTarget(t, link); !samePath(got, newExe) {
		t.Fatalf("目标是 %q，期望 %q", got, newExe)
	}
}

// 目标已是最新、名字还停在旧版本：改指向那条路匹配不到（oldPath 早不是
// 「上一版」），只能靠启动时的名字对齐。
func TestAlignShortcutNamesHealsStaleName(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "AgentDeck-v0.16.0-win-x64.exe")
	if err := os.WriteFile(current, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "AgentDeck-v0.15.3-win-x64.lnk")
	if !createShortcut(t, stale, current) {
		t.Skip("本机无法创建快捷方式，跳过")
	}
	if n := alignShortcutNamesIn([]string{dir}, current); n != 1 {
		t.Fatalf("应当对齐 1 个快捷方式，实际 %d", n)
	}
	want := filepath.Join(dir, "AgentDeck-v0.16.0-win-x64.lnk")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("快捷方式应改名为 %q: %v", filepath.Base(want), err)
	}
	// 再跑一次不应再改动任何东西。
	if n := alignShortcutNamesIn([]string{dir}, current); n != 0 {
		t.Fatalf("已对齐时不应再改，实际改了 %d 个", n)
	}
}

// 已存在同名快捷方式时不覆盖：那可能是用户自己又建的一个。
func TestRenameShortcutDoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "AgentDeck-v0.16.0-win-x64.exe")
	if err := os.WriteFile(current, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "AgentDeck-v0.16.0-win-x64.lnk")
	stale := filepath.Join(dir, "AgentDeck-v0.15.3-win-x64.lnk")
	for _, link := range []string{existing, stale} {
		if !createShortcut(t, link, current) {
			t.Skip("本机无法创建快捷方式，跳过")
		}
	}
	before, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	renameShortcutForVersion(stale, current)
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("目标名字被占用时不应改名: %v", err)
	}
	after, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("已存在的同名快捷方式被覆盖了")
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

// 两个辅助函数同样走 withShell：COM 单元是线程本地状态，测试里不绑线程一样会
// 偶发失败（表现为读回空路径、测试在整包并发跑时才红）。
func createShortcut(t *testing.T, link, target string) bool {
	t.Helper()
	err := withShell(func(shell *ole.IDispatch) error {
		shortcut, err := oleutil.CallMethod(shell, "CreateShortcut", link)
		if err != nil {
			return err
		}
		dispatch := shortcut.ToIDispatch()
		if dispatch == nil {
			return errors.New("取不到快捷方式的 IDispatch")
		}
		defer dispatch.Release()
		if _, err = oleutil.PutProperty(dispatch, "TargetPath", target); err != nil {
			return err
		}
		_, _ = oleutil.PutProperty(dispatch, "WorkingDirectory", filepath.Dir(target))
		_, err = oleutil.CallMethod(dispatch, "Save")
		return err
	})
	return err == nil
}

func shortcutTarget(t *testing.T, link string) string {
	t.Helper()
	var out string
	_ = withShell(func(shell *ole.IDispatch) error {
		value, ok := shortcutTargetOf(shell, link)
		if !ok {
			return errors.New("读不到快捷方式的目标")
		}
		out = value
		return nil
	})
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
