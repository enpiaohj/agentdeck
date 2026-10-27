package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeStartupValue struct {
	value   string
	err     error
	deleted bool
}

func TestActivationNotification(t *testing.T) {
	name := fmt.Sprintf(`Local\AgentDeck-Test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	opened := make(chan struct{}, 2)
	cleanup, err := listenForActivation(name, func() { opened <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		notifyActivation(name)
		select {
		case <-opened:
		case <-time.After(2 * time.Second):
			t.Fatal("existing instance was not notified")
		}
	}
	cleanup()
	notifyActivation(name) // A missing listener must be harmless.
}

func (f *fakeStartupValue) read() (string, error) { return f.value, f.err }
func (f *fakeStartupValue) write(v string) error {
	if f.err != nil {
		return f.err
	}
	f.value = v
	return nil
}
func (f *fakeStartupValue) remove() error {
	if f.err != nil {
		return f.err
	}
	f.deleted = true
	f.value = ""
	return nil
}

func TestStartupRegistration(t *testing.T) {
	f := &fakeStartupValue{}
	r := startupRegistration{value: f, executable: func() (string, error) { return `D:\应用 程序\AgentDeck.exe`, nil }}
	if on, err := r.enabled(); err != nil || on {
		t.Fatalf("default: %v %v", on, err)
	}
	if err := r.set(true); err != nil {
		t.Fatal(err)
	}
	if f.value != `"D:\应用 程序\AgentDeck.exe" --startup` {
		t.Fatalf("unsafe command %q", f.value)
	}
	if on, err := r.enabled(); err != nil || !on {
		t.Fatalf("enabled: %v %v", on, err)
	}
	if err := r.set(false); err != nil || !f.deleted {
		t.Fatalf("disable %v", err)
	}
	f.err = errors.New("access denied")
	if _, err := r.enabled(); err == nil {
		t.Fatal("read failure hidden")
	}
	if err := r.set(true); err == nil {
		t.Fatal("write failure hidden")
	}
	f.err = nil
	r.executable = func() (string, error) { return "", errors.New("missing exe") }
	if err := r.set(true); err == nil {
		t.Fatal("missing executable accepted")
	}
	if err := r.set(false); err != nil {
		t.Fatal("disable should not need executable", err)
	}
}

// 启动项停在更早的版本名（文件已不存在）时也要能修：这类残留不会因为
// 「上一个版本」的 oldPath 出现，只能靠启动时的自愈。
func TestStartupRetargetHealsStaleVersion(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "AgentDeck-v0.16.0-win-x64.exe")
	if err := os.WriteFile(current, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 停在 v0.13.0，而那个文件根本不存在——本机实测就是这个状态。
	stale := filepath.Join(dir, "AgentDeck-v0.13.0-win-x64.exe")
	f := &fakeStartupValue{value: `"` + stale + `" --startup`}
	r := startupRegistration{value: f, executable: func() (string, error) { return current, nil }}
	if !r.retarget(filepath.Join(dir, "AgentDeck-v0.15.3-win-x64.exe"), current) {
		t.Fatal("指向已不存在旧产物的启动项应当被修复")
	}
	if f.value != `"`+current+`" --startup` {
		t.Fatalf("启动项未改指向新版本：%q", f.value)
	}
}

// 仍然存在的另一份副本不碰：用户可能有意让它开机启动另一个位置。
func TestStartupRetargetLeavesExistingCopy(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "AgentDeck-v0.16.0-win-x64.exe")
	other := filepath.Join(dir, "AgentDeck-v0.15.3-win-x64.exe")
	for _, p := range []string{current, other} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeStartupValue{value: `"` + other + `" --startup`}
	r := startupRegistration{value: f, executable: func() (string, error) { return current, nil }}
	if !r.retarget(filepath.Join(dir, "AgentDeck-v0.14.0-win-x64.exe"), current) {
		t.Fatal("没有需要修的东西时应报告完成")
	}
	if !strings.Contains(f.value, other) {
		t.Fatalf("仍可用的副本被改掉了：%q", f.value)
	}
}

// 别的目录、别的程序名都不碰，避免替用户改掉他自己的自启动配置。
func TestStartupRetargetLeavesUnrelatedCommands(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "AgentDeck-v0.16.0-win-x64.exe")
	if err := os.WriteFile(current, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		`"C:\Other\SomeApp.exe" --flag`,
		`"` + filepath.Join(dir, "MyOwnName.exe") + `"`,
		`C:\Other\AgentDeck-v0.13.0-win-x64.exe`,
	} {
		f := &fakeStartupValue{value: command}
		r := startupRegistration{value: f, executable: func() (string, error) { return current, nil }}
		if !r.retarget(filepath.Join(dir, "AgentDeck-v0.15.3-win-x64.exe"), current) {
			t.Fatalf("%q：不应报告失败", command)
		}
		if f.value != command {
			t.Fatalf("%q 被改成了 %q", command, f.value)
		}
	}
}

// 用户关掉自启动（值为空）时什么都不做。
func TestStartupRetargetIgnoresEmptyValue(t *testing.T) {
	f := &fakeStartupValue{}
	r := startupRegistration{value: f, executable: func() (string, error) { return `D:\x\AgentDeck-v0.16.0-win-x64.exe`, nil }}
	if !r.retarget(`D:\x\AgentDeck-v0.15.3-win-x64.exe`, `D:\x\AgentDeck-v0.16.0-win-x64.exe`) {
		t.Fatal("空值应视为无需修改")
	}
	if f.value != "" {
		t.Fatalf("空值被写成了 %q", f.value)
	}
}
