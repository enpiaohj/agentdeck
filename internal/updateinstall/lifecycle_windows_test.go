//go:build windows

package updateinstall

import (
	"os"
	"path/filepath"
	"testing"
)

// 真实环境验证：用两个假的 exe 文件走一遍「退出时落地 + 启动新版本 + 下次启动善后」，
// 确认文件名跟随版本、启动项改指向、旧文件删除都按设计发生。
//
// 只在能写注册表的环境里跑（这里是 HKCU，普通用户即可）；写失败就跳过。
func TestEndToEndInstallLifecycle(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "AgentDeck-v0.14.0-win-x64.exe")
	next := filepath.Join(dir, "AgentDeck-v0.15.0-win-x64.exe")
	if err := os.WriteFile(current, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkgDir := t.TempDir()
	pkg := filepath.Join(pkgDir, "AgentDeck-v0.15.0-win-x64.exe")
	if err := os.WriteFile(pkg, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Probe(current); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	target := ResolveTarget(current, pkg)
	if target != next {
		t.Fatalf("落地路径 = %q，期望 %q", target, next)
	}
	if !Install(current, target, pkg) {
		t.Fatal("Install 失败")
	}
	if b, err := os.ReadFile(next); err != nil || string(b) != "new binary" {
		t.Fatalf("新版本内容 = %q %v", b, err)
	}
	if b, err := os.ReadFile(current); err != nil || string(b) != "old binary" {
		t.Fatalf("旧文件应原样保留到下次启动：%q %v", b, err)
	}

	// 下次启动：旧文件被删除，标记被清除。
	var retargeted bool
	Cleanup(next, func(from, to string) bool {
		if from == current && to == next {
			retargeted = true
		}
		return true
	})
	if !retargeted {
		t.Fatal("善后时应当把启动项与快捷方式改指向新文件")
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatal("旧文件应当被删除")
	}
	if _, err := os.Stat(next + ReplacesSuffix); !os.IsNotExist(err) {
		t.Fatal("标记应当被清除")
	}
}

// 原地替换（用户改过文件名）也要能走完，且回滚副本在下次启动被清掉。
func TestEndToEndInPlaceLifecycle(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "AgentDeck.exe")
	if err := os.WriteFile(current, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), "AgentDeck-v0.15.0-win-x64.exe")
	if err := os.WriteFile(pkg, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if target := ResolveTarget(current, pkg); target != current {
		t.Fatalf("改过名字应原地替换，得到 %q", target)
	}
	if !Install(current, current, pkg) {
		t.Fatal("Install 失败")
	}
	if b, err := os.ReadFile(current); err != nil || string(b) != "new binary" {
		t.Fatalf("原地替换内容 = %q %v", b, err)
	}
	Cleanup(current, nil)
	if _, err := os.Stat(current + BackupSuffix); !os.IsNotExist(err) {
		t.Fatal("回滚副本应当被清掉")
	}
}
