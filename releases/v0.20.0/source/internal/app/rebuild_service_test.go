package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/rebuild"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const rebuildTestID = "a1b2c3d4-2222-3333-4444-5555555555ee"

// cleanedClaudeSession 造一个“已被 Claude Code 清理”的会话：先有原文件并被读过一次（进了永久库），
// 再删掉原文件，刷新后它就只剩永久库副本。claudeHome 是隔离的 Claude 数据目录。
func cleanedClaudeSession(t *testing.T) (a *App, key, work, claudeHome string, ran *[]string) {
	t.Helper()
	claudeHome = t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeHome)
	a, _, _, ran = newTestApp(t)
	work = t.TempDir()
	src := filepath.Join(t.TempDir(), rebuildTestID+".jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"帮我修一下登录的 bug"},"uuid":"u1","cwd":` + jsonString(work) + `}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"好的，我先看看登录模块。"}]},"uuid":"u2"}`,
		`{"type":"user","message":{"role":"user","content":"改好后跑一下测试"},"uuid":"u3"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"测试全部通过。"}]},"uuid":"u4"}`,
	}
	if err := os.WriteFile(src, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sess := sessions.Session{Agent: "claude", ID: rebuildTestID, Cwd: work, Title: "修登录", Path: src,
		Start: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), Last: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)}
	a.setList([]sessions.Session{sess})
	key = "claude:" + rebuildTestID
	svc := a.NewSessionService()
	if _, err := svc.TranscriptWindow(key, -1, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	svc.Refresh()
	_, byKey, _ := a.snapshot()
	if !byKey[key].Retained {
		t.Fatalf("前置条件：会话应已只剩永久库副本: %+v", byKey[key])
	}
	return a, key, work, claudeHome, ran
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestRebuildRestoresCleanedSession(t *testing.T) {
	a, key, work, claudeHome, _ := cleanedClaudeSession(t)
	r := a.NewResumeService()
	pre, err := r.PreviewRebuild(key, "")
	if err != nil {
		t.Fatal(err)
	}
	name, _ := rebuild.ProjectDirName(work)
	want := filepath.Join(claudeHome, "projects", name, rebuildTestID+".jsonl")
	if pre.File != want || pre.Token == "" || pre.NeedsDirectory || pre.Turns != 2 || pre.Kept != 4 {
		t.Fatalf("预览: %+v（期望文件 %s）", pre, want)
	}
	if _, err := os.Stat(want); err == nil {
		t.Fatal("预览不该写任何文件")
	}
	if _, err := os.Stat(filepath.Join(claudeHome, "projects")); err == nil {
		t.Fatal("预览不该创建项目文件夹")
	}

	res, err := r.Rebuild(pre.Token)
	if err != nil || res.File != want || res.Bytes != pre.Bytes {
		t.Fatalf("重建: %+v %v", res, err)
	}
	data, err := os.ReadFile(want)
	if err != nil || !strings.Contains(string(data), "测试全部通过。") || !strings.Contains(string(data), `"sessionId":"`+rebuildTestID+`"`) {
		t.Fatalf("写入的内容: %v\n%s", err, data)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(want), ".agentdeck-rebuild-*")); len(left) != 0 {
		t.Fatalf("不该留下临时文件: %v", left)
	}
	_, byKey, _ := a.snapshot()
	if got := byKey[key]; got.Retained {
		t.Fatalf("重建后会话应回到 Claude Code 的原生会话: %+v", got)
	}
	// 之后继续会话走的是普通的恢复命令
	if _, err := r.Prepare(key, ""); err != nil {
		t.Fatalf("重建后应能恢复: %v", err)
	}
	// 同一个 token 不能再用，已存在的文件也不能被预览覆盖
	if _, err := r.Rebuild(pre.Token); err == nil {
		t.Fatal("token 只能用一次")
	}
	if _, err := r.PreviewRebuild(key, ""); err == nil {
		t.Fatal("会话已不再是“被清理”的状态，不该再给预览")
	}
}

func TestRebuildNeverOverwritesExistingFile(t *testing.T) {
	a, key, work, _, _ := cleanedClaudeSession(t)
	r := a.NewResumeService()
	pre, err := r.PreviewRebuild(key, "")
	if err != nil {
		t.Fatal(err)
	}
	// 预览之后、确认之前，有别的东西占了这个文件名
	if err := os.MkdirAll(filepath.Dir(pre.File), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pre.File, []byte("别人的内容"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Rebuild(pre.Token); err == nil || !strings.Contains(err.Error(), "不会覆盖") {
		t.Fatalf("应拒绝覆盖: %v", err)
	}
	if b, _ := os.ReadFile(pre.File); string(b) != "别人的内容" {
		t.Fatalf("已有文件被改动: %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(pre.File), ".agentdeck-rebuild-*")); len(left) != 0 {
		t.Fatalf("失败后不该留下临时文件: %v", left)
	}
	_ = work
}

func TestRebuildRefusesWhatItShouldNot(t *testing.T) {
	a, key, _, _, _ := cleanedClaudeSession(t)
	r := a.NewResumeService()
	if _, err := r.PreviewRebuild("claude:not-there", ""); err == nil {
		t.Error("不存在的会话")
	}
	if _, err := r.Rebuild("没有这个token"); err == nil {
		t.Error("无效 token")
	}
	// 过期的 token
	pre, err := r.PreviewRebuild(key, "")
	if err != nil {
		t.Fatal(err)
	}
	a.dataMu.Lock()
	a.rebuildPlans[pre.Token].expires = time.Now().Add(-time.Second)
	a.dataMu.Unlock()
	if _, err := r.Rebuild(pre.Token); err == nil {
		t.Error("过期的 token")
	}
	// 显式指定的目录必须存在
	if _, err := r.PreviewRebuild(key, filepath.Join(t.TempDir(), "不存在")); err == nil {
		t.Error("不存在的目录")
	}
	// 其他智能体、仍在原位的会话都不是目标
	a.setList([]sessions.Session{{Agent: "codex", ID: "c1", Retained: true}, {Agent: "claude", ID: rebuildTestID, Cwd: t.TempDir()}})
	if _, err := r.PreviewRebuild("codex:c1", ""); err == nil {
		t.Error("只支持 Claude Code")
	}
	if _, err := r.PreviewRebuild("claude:"+rebuildTestID, ""); err == nil || !strings.Contains(err.Error(), "还在") {
		t.Errorf("还在原位的会话不需要重建: %v", err)
	}
}

func TestRebuildAsksForDirectoryWhenOriginalIsGone(t *testing.T) {
	a, key, work, claudeHome, _ := cleanedClaudeSession(t)
	if err := os.RemoveAll(work); err != nil {
		t.Fatal(err)
	}
	r := a.NewResumeService()
	pre, err := r.PreviewRebuild(key, "")
	if err != nil || !pre.NeedsDirectory || pre.Token != "" || pre.File != "" {
		t.Fatalf("原目录不存在时应要求选目录: %+v %v", pre, err)
	}
	other := t.TempDir()
	pre, err = r.PreviewRebuild(key, other)
	if err != nil || pre.NeedsDirectory || pre.Directory != filepath.Clean(other) {
		t.Fatalf("换目录: %+v %v", pre, err)
	}
	name, _ := rebuild.ProjectDirName(filepath.Clean(other))
	if pre.File != filepath.Join(claudeHome, "projects", name, rebuildTestID+".jsonl") {
		t.Fatalf("应写进所选目录对应的项目文件夹: %s", pre.File)
	}
	if _, err := r.Rebuild(pre.Token); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(pre.File)
	if !strings.Contains(string(data), jsonString(filepath.Clean(other))) {
		t.Fatal("文件里的 cwd 应是所选目录")
	}
}

func TestWriteNewFileNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "x.jsonl")
	if err := writeNewFile(target, []byte("第一份")); err != nil {
		t.Fatal(err)
	}
	if err := writeNewFile(target, []byte("第二份")); err == nil {
		t.Fatal("已存在时必须失败")
	}
	if b, _ := os.ReadFile(target); string(b) != "第一份" {
		t.Fatalf("内容被改: %q", b)
	}
	if left, _ := os.ReadDir(filepath.Dir(target)); len(left) != 1 {
		t.Fatalf("不该留下临时文件: %v", left)
	}
}

func TestContinueWithHistoryDoesNotWriteUpstream(t *testing.T) {
	a, key, work, claudeHome, ran := cleanedClaudeSession(t)
	r := a.NewResumeService()
	res, err := r.ContinueWithHistory(key, "")
	if err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(res.File)
	if err != nil || !strings.Contains(string(md), "测试全部通过。") || !strings.Contains(string(md), "## 助手") {
		t.Fatalf("交接文档: %v\n%s", err, md)
	}
	if strings.HasPrefix(res.File, claudeHome) || !strings.HasPrefix(res.File, a.handoffDir()) {
		t.Fatalf("交接文档应在 AgentDeck 自己的目录里，而不是上游: %s", res.File)
	}
	if _, err := os.Stat(filepath.Join(claudeHome, "projects")); err == nil {
		t.Fatal("带历史新开会话不该写 Claude 的目录")
	}
	if len(*ran) != 1 {
		t.Fatalf("终端启动次数 = %d", len(*ran))
	}
	script := decodeLaunched(t, (*ran)[0])
	for _, want := range []string{work, "claude", "--add-dir", res.File, "文字记录"} {
		if !strings.Contains(script, want) {
			t.Errorf("命令里缺少 %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "--resume") || strings.Contains(script, " resume ") {
		t.Fatalf("这是新开会话，不该带 resume: %s", script)
	}
	if _, err := r.ContinueWithHistory("claude:not-there", ""); err == nil {
		t.Error("不存在的会话")
	}
}
