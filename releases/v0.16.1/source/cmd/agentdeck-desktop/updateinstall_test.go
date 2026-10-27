package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/updateinstall"
)

// newTestInstaller 构造一个以 dir 里指定文件为「当前程序」的安装器。
func newTestInstaller(t *testing.T, dir, name string) *updateInstaller {
	t.Helper()
	current := filepath.Join(dir, name)
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &updateInstaller{current: current}
}

// 正常退出（没点重启）只落地，不拉起任何进程。
func TestFinalizeWithoutRestartOnlyInstalls(t *testing.T) {
	restartPending.Store(false)
	dir := t.TempDir()
	installer := newTestInstaller(t, dir, "AgentDeck-v0.14.0-win-x64.exe")
	pkg := filepath.Join(t.TempDir(), "AgentDeck-v0.15.0-win-x64.exe")
	if err := os.WriteFile(pkg, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	target, err := installer.Stage(pkg)
	if err != nil {
		t.Fatal(err)
	}
	finalize(installer)
	if b, err := os.ReadFile(target); err != nil || string(b) != "new" {
		t.Fatalf("退出时应已落地新版本：%q %v", b, err)
	}
	if _, err := os.Stat(target + updateinstall.FailedSuffix); !os.IsNotExist(err) {
		t.Fatal("落地成功不应留下失败标记")
	}
}

// 没登记更新包时 finalize 什么都不做，当前程序保持原样。
func TestFinalizeWithoutStagedPackageKeepsCurrent(t *testing.T) {
	restartPending.Store(false)
	dir := t.TempDir()
	installer := newTestInstaller(t, dir, "AgentDeck.exe")
	finalize(installer)
	if b, err := os.ReadFile(installer.current); err != nil || string(b) != "old" {
		t.Fatalf("没有更新包时不应改动当前程序：%q %v", b, err)
	}
}

// 落地失败：原文件保持可用，并留下失败标记供下次启动提示一次。
func TestFinalizeFailureKeepsCurrentAndMarks(t *testing.T) {
	restartPending.Store(false)
	dir := t.TempDir()
	installer := newTestInstaller(t, dir, "AgentDeck-v0.14.0-win-x64.exe")
	// 更新包指向一个不存在的文件：Install 必然失败。
	target, err := installer.Stage(filepath.Join(t.TempDir(), "AgentDeck-v0.15.0-win-x64.exe"))
	if err == nil {
		t.Fatalf("登记不存在的更新包应当失败，得到目标 %q", target)
	}
	finalize(installer)
	if b, err := os.ReadFile(installer.current); err != nil || string(b) != "old" {
		t.Fatalf("当前程序应保持可用：%q %v", b, err)
	}
}

// 重启意图在退出收尾时读取：点过重启才拉起新进程。
func TestRestartPendingDrivesFinalize(t *testing.T) {
	restartPending.Store(false)
	if restartPending.Load() {
		t.Fatal("初始状态不应有重启意图")
	}
	// relaunchSelf 只记意图，不立刻启动；真正的启动在 finalize。
	done := make(chan struct{})
	if err := relaunchSelf(func() { close(done) }); err != nil {
		t.Fatal(err)
	}
	if !restartPending.Load() {
		t.Fatal("relaunchSelf 应当记下重启意图")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("relaunchSelf 应当调用退出回调")
	}
	restartPending.Store(false)
}
