package updateinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeExe 在 dir 下写一个假 exe 并返回路径。
func writeExe(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveTargetFollowsVersionName(t *testing.T) {
	dir := t.TempDir()
	// 发布产物命名：文件名跟随版本，落在同一目录。
	current := filepath.Join(dir, "AgentDeck-v0.14.0-win-x64.exe")
	got := ResolveTarget(current, filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.exe"))
	if want := filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.exe"); got != want {
		t.Fatalf("跟随版本 = %q，期望 %q", got, want)
	}
	// 用户改过名字：保持原路径原地替换。
	renamed := filepath.Join(dir, "AgentDeck.exe")
	if got := ResolveTarget(renamed, filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.exe")); got != renamed {
		t.Fatalf("改过名字应原地替换，得到 %q", got)
	}
	// 更新包名不合规范时也不改名，避免落到一个奇怪的路径。
	if got := ResolveTarget(current, filepath.Join(dir, "package.exe")); got != current {
		t.Fatalf("更新包名不规范应原地替换，得到 %q", got)
	}
}

func TestInstallSideBySide(t *testing.T) {
	dir := t.TempDir()
	current := writeExe(t, dir, "AgentDeck-v0.14.0-win-x64.exe", "old")
	// 更新包下载在另一个目录（数据目录下的 updates/），文件名是发布产物名。
	pkg := writeExe(t, t.TempDir(), "AgentDeck-v0.15.0-win-x64.exe", "new content")
	target := ResolveTarget(current, pkg)

	if !Install(current, target, pkg) {
		t.Fatal("落地失败")
	}
	// 旧文件原样留着（新版本启动后删除），新文件内容正确。
	if b, err := os.ReadFile(current); err != nil || string(b) != "old" {
		t.Fatalf("旧文件被改动：%q %v", b, err)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "new content" {
		t.Fatalf("新文件内容 = %q %v", b, err)
	}
	// 标记里记的是被取代的旧文件名；更新包本身已删除。
	if b, err := os.ReadFile(target + ReplacesSuffix); err != nil || strings.TrimSpace(string(b)) != filepath.Base(current) {
		t.Fatalf("替换标记 = %q %v", b, err)
	}
	if _, err := os.Stat(pkg); !os.IsNotExist(err) {
		t.Fatal("更新包应当已被移走")
	}
	if _, err := os.Stat(target + StagingSuffix); !os.IsNotExist(err) {
		t.Fatal("不应留下临时文件")
	}
}

func TestInstallInPlace(t *testing.T) {
	dir := t.TempDir()
	current := writeExe(t, dir, "AgentDeck.exe", "old")
	pkg := writeExe(t, dir, "download.part", "new content")

	if !Install(current, current, pkg) {
		t.Fatal("落地失败")
	}
	if b, err := os.ReadFile(current); err != nil || string(b) != "new content" {
		t.Fatalf("原地替换内容 = %q %v", b, err)
	}
	// 回滚副本保留到下次启动的善后，不是落地时删。
	if b, err := os.ReadFile(current + BackupSuffix); err != nil || string(b) != "old" {
		t.Fatalf("回滚副本 = %q %v", b, err)
	}
	Cleanup(current, nil)
	if _, err := os.Stat(current + BackupSuffix); !os.IsNotExist(err) {
		t.Fatal("启动善后应当清掉回滚副本")
	}
}

// 用户自己用 ".old" 做的备份不能被更新流程当成回滚副本删掉。
//
// 这是 .old 不能当回滚副本名的原因：同目录出现 AgentDeck.exe.old 的常见原因
// 恰恰是用户手动备份，而更新流程一旦认领这个名字就会覆盖它。
func TestInstallInPlaceKeepsUserOldBackup(t *testing.T) {
	dir := t.TempDir()
	current := writeExe(t, dir, "AgentDeck.exe", "old")
	pkg := writeExe(t, dir, "download.part", "new content")
	userBackup := current + ".old"
	if err := os.WriteFile(userBackup, []byte("USER BACKUP"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Install(current, current, pkg) {
		t.Fatal("落地失败")
	}
	if b, err := os.ReadFile(userBackup); err != nil || string(b) != "USER BACKUP" {
		t.Fatalf("用户自己的 .old 备份被改动：%q %v", b, err)
	}
	// 善后也不能碰它。
	Cleanup(current, nil)
	if b, err := os.ReadFile(userBackup); err != nil || string(b) != "USER BACKUP" {
		t.Fatalf("善后删掉了用户的 .old 备份：%q %v", b, err)
	}
}

func TestInstallFailureKeepsCurrent(t *testing.T) {
	dir := t.TempDir()
	current := writeExe(t, dir, "AgentDeck.exe", "old")
	// 更新包不存在：落地失败，原文件必须仍然可用。
	if Install(current, current, filepath.Join(dir, "missing.exe")) {
		t.Fatal("更新包不存在时不应报告成功")
	}
	if b, err := os.ReadFile(current); err != nil || string(b) != "old" {
		t.Fatalf("失败后原文件被破坏：%q %v", b, err)
	}
	if _, err := os.Stat(current + StagingSuffix); !os.IsNotExist(err) {
		t.Fatal("失败时不应留下临时文件")
	}
}

func TestProbeWritable(t *testing.T) {
	if err := Probe(filepath.Join(t.TempDir(), "AgentDeck.exe")); err != nil {
		t.Fatalf("可写目录应当通过：%v", err)
	}
}

func TestFailureMarkerConsumedOnce(t *testing.T) {
	current := filepath.Join(t.TempDir(), "AgentDeck.exe")
	if ConsumeFailureMarker(current) {
		t.Fatal("没有标记时不应报告失败")
	}
	MarkFailed(current)
	if !ConsumeFailureMarker(current) {
		t.Fatal("应当读到失败标记")
	}
	if ConsumeFailureMarker(current) {
		t.Fatal("标记只能提示一次")
	}
}

func TestCleanupRemovesPreviousVersion(t *testing.T) {
	dir := t.TempDir()
	old := writeExe(t, dir, "AgentDeck-v0.14.0-win-x64.exe", "old")
	current := writeExe(t, dir, "AgentDeck-v0.15.0-win-x64.exe", "new")
	if err := os.WriteFile(current+ReplacesSuffix, []byte(filepath.Base(old)), 0o600); err != nil {
		t.Fatal(err)
	}
	var retargeted [][2]string
	Cleanup(current, func(from, to string) bool {
		retargeted = append(retargeted, [2]string{from, to})
		return true
	})

	if len(retargeted) != 1 || retargeted[0][0] != old || retargeted[0][1] != current {
		t.Fatalf("改指向参数 = %v", retargeted)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("旧文件应当被删除")
	}
	if _, err := os.Stat(current + ReplacesSuffix); !os.IsNotExist(err) {
		t.Fatal("标记应当被清除")
	}
}

// 改指向没被确认时不能删旧文件：删掉一个还有东西指着的文件，用户就会留下
// 一个点不开的死链接。标记保留，下次启动再试。
func TestCleanupKeepsOldFileWhenRetargetUnconfirmed(t *testing.T) {
	dir := t.TempDir()
	old := writeExe(t, dir, "AgentDeck-v0.14.0-win-x64.exe", "old")
	current := writeExe(t, dir, "AgentDeck-v0.15.0-win-x64.exe", "new")
	if err := os.WriteFile(current+ReplacesSuffix, []byte(filepath.Base(old)), 0o600); err != nil {
		t.Fatal(err)
	}
	Cleanup(current, func(string, string) bool { return false })

	if _, err := os.Stat(old); err != nil {
		t.Fatal("改指向未确认时不该删旧文件")
	}
	if _, err := os.Stat(current + ReplacesSuffix); err != nil {
		t.Fatal("旧文件还在，标记就该保留")
	}
}

// 没有改指向能力（retarget 为 nil）时同样不删旧文件。
func TestCleanupWithoutRetargetKeepsOldFile(t *testing.T) {
	dir := t.TempDir()
	old := writeExe(t, dir, "AgentDeck-v0.14.0-win-x64.exe", "old")
	current := writeExe(t, dir, "AgentDeck-v0.15.0-win-x64.exe", "new")
	if err := os.WriteFile(current+ReplacesSuffix, []byte(filepath.Base(old)), 0o600); err != nil {
		t.Fatal(err)
	}
	Cleanup(current, nil)

	if _, err := os.Stat(old); err != nil {
		t.Fatal("没有改指向能力时不该删旧文件")
	}
	if _, err := os.Stat(current + ReplacesSuffix); err != nil {
		t.Fatal("旧文件还在，标记就该保留")
	}
}

// 标记内容不可信时什么都不动：既不删别的文件，也不改快捷方式。
func TestCleanupIgnoresUnsafeMarker(t *testing.T) {
	dir := t.TempDir()
	keep := writeExe(t, dir, "important.txt", "keep me")
	current := writeExe(t, dir, "AgentDeck-v0.15.0-win-x64.exe", "new")
	for _, marker := range []string{"../important.txt", `C:\Windows\notepad.exe`, filepath.Base(current), ""} {
		if err := os.WriteFile(current+ReplacesSuffix, []byte(marker), 0o600); err != nil {
			t.Fatal(err)
		}
		called := false
		Cleanup(current, func(string, string) bool { called = true; return true })
		if called {
			t.Fatalf("标记 %q 不应触发改指向", marker)
		}
		if _, err := os.Stat(keep); err != nil {
			t.Fatalf("标记 %q 时误删了别的文件", marker)
		}
	}
}

func TestCleanupKeepsMarkerWhenOldFileBusy(t *testing.T) {
	dir := t.TempDir()
	old := writeExe(t, dir, "AgentDeck-v0.14.0-win-x64.exe", "old")
	current := writeExe(t, dir, "AgentDeck-v0.15.0-win-x64.exe", "new")
	if err := os.WriteFile(current+ReplacesSuffix, []byte(filepath.Base(old)), 0o600); err != nil {
		t.Fatal(err)
	}
	// 模拟旧进程仍占用文件：删不掉的旧文件（这里用目录让 os.Remove 必然失败）。
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(old, 0o755); err != nil {
		t.Fatal(err)
	}
	Cleanup(current, nil)
	// 目录删不掉（Windows 上 os.Remove 对非空目录失败；空目录会被删掉），
	// 两种情况都不该清标记——标记只在旧文件确实消失后才清。
	if _, err := os.Stat(old); err == nil {
		if _, err := os.Stat(current + ReplacesSuffix); err != nil {
			t.Fatal("旧文件删不掉时应保留标记，下次启动再试")
		}
	}
}

func TestSafePreviousName(t *testing.T) {
	for _, c := range []struct {
		name, current string
		want          bool
	}{
		{"AgentDeck-v0.14.0-win-x64.exe", "AgentDeck-v0.15.0-win-x64.exe", true},
		{"AgentDeck-v0.15.0-win-x64.exe", "AgentDeck-v0.15.0-win-x64.exe", false},
		{"", "AgentDeck-v0.15.0-win-x64.exe", false},
		{`..\evil.exe`, "AgentDeck-v0.15.0-win-x64.exe", false},
		{"notepad.exe", "AgentDeck-v0.15.0-win-x64.exe", false},
	} {
		if got := safePreviousName(c.name, c.current); got != c.want {
			t.Fatalf("safePreviousName(%q, %q) = %v，期望 %v", c.name, c.current, got, c.want)
		}
	}
}

// side-by-side 落地在写 staging 之后、改名之前崩溃时，残留的 <新版本名>.new
// 不会被 current+".new" 那条清理命中。它必须在下一次启动时被清掉。
func TestCleanupRemovesStaleSideBySideStaging(t *testing.T) {
	dir := t.TempDir()
	current := writeExe(t, dir, "AgentDeck-v0.14.0-win-x64.exe", "old")
	residue := writeExe(t, dir, "AgentDeck-v0.15.0-win-x64.exe"+StagingSuffix, "partial")
	// 用户放在同目录、名字不像发布产物的 .new 不能碰。
	userFile := writeExe(t, dir, "notes.new", "keep me")

	Cleanup(current, nil)

	if _, err := os.Stat(residue); !os.IsNotExist(err) {
		t.Fatal("side-by-side 的 .new 残留应当被清掉")
	}
	if b, err := os.ReadFile(userFile); err != nil || string(b) != "keep me" {
		t.Fatalf("误删了同目录的普通 .new 文件：%q %v", b, err)
	}
	if b, err := os.ReadFile(current); err != nil || string(b) != "old" {
		t.Fatalf("当前程序被改动：%q %v", b, err)
	}
}
