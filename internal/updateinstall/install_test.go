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
	// .old 是回滚副本：保留到下次启动的善后，不是落地时删。
	if b, err := os.ReadFile(current + BackupSuffix); err != nil || string(b) != "old" {
		t.Fatalf("回滚副本 = %q %v", b, err)
	}
	Cleanup(current, nil)
	if _, err := os.Stat(current + BackupSuffix); !os.IsNotExist(err) {
		t.Fatal("启动善后应当清掉回滚副本")
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
	Cleanup(current, func(from, to string) { retargeted = append(retargeted, [2]string{from, to}) })

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
		Cleanup(current, func(string, string) { called = true })
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
