package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// 新建项目：还没有任何会话的目录也要能进项目列表，且不与会话聚合的重复。
func TestDeclaredProjectsAppearBesideAggregatedOnes(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	root := t.TempDir()
	knownPath := filepath.Join(root, "KnownProject")
	freshPath := filepath.Join(root, "Fresh")
	a.setList([]sessions.Session{
		{Agent: "codex", ID: "c1", Cwd: knownPath, Last: time.Unix(10, 0)},
		{Agent: "claude", ID: "k1", Cwd: knownPath, Last: time.Unix(20, 0)},
	})
	ps := a.NewProjectService()
	created, err := ps.Create(freshPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if created != freshPath {
		t.Fatalf("创建返回路径 = %q", created)
	}
	// 目录真的建出来了。
	if !ps.Exists(created) {
		t.Fatal("新建项目没有建出目录")
	}
	// 已经聚出会话的那个路径再声明一次，只更新名字、不多出一行；聚合出来的
	// 统计数据（会话数）要填进这一行，不能显示成 0 个会话。
	// 注意：行里的 Path 取用户最后一次书写的大小写（这里故意用小写）。
	if _, err := ps.Create(strings.ToLower(knownPath), "本仓库"); err != nil {
		t.Fatal(err)
	}
	rows := a.NewSessionService().Projects()
	if len(rows) != 2 {
		t.Fatalf("项目行数 = %d, want 2: %+v", len(rows), rows)
	}
	byPath := map[string]ProjectView{}
	for _, r := range rows {
		byPath[r.Path] = r
	}
	fresh, ok := byPath[freshPath]
	if !ok || !fresh.Declared || fresh.Count != 0 || fresh.Name != "Fresh" {
		t.Fatalf("声明的项目行不对: %+v", fresh)
	}
	known, ok := byPath[strings.ToLower(knownPath)]
	if !ok || !known.Declared || known.Count != 2 || known.Name != "本仓库" {
		t.Fatalf("重名的聚合项目行不对: %+v", known)
	}
	// 取消声明：目录与文件不动，列表里消失。
	if err := ps.Remove(freshPath); err != nil {
		t.Fatal(err)
	}
	if !ps.Exists(freshPath) {
		t.Fatal("取消声明把目录删了")
	}
	for _, r := range a.NewSessionService().Projects() {
		if r.Path == freshPath {
			t.Fatal("取消声明后仍出现在列表里")
		}
	}
}

// 目录建不出来时撤掉声明，不留一个指着一片空的项目。
func TestDeclaredProjectRollsBackWhenDirectoryFails(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	ps := a.NewProjectService()
	// 把一个已有文件当父目录：MkdirAll 必然失败（ENOTDIR）。
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create(filepath.Join(file, "子项目"), ""); err == nil {
		t.Fatal("父路径是文件竟被当成建成功了")
	}
	rows, err := ps.Declared()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("失败后仍留下声明: %+v", rows)
	}
}

// 解码 -EncodedCommand 交付的脚本，看它到底让终端跑什么。
func decodeLaunched(t *testing.T, cmdline string) string {
	t.Helper()
	i := strings.Index(cmdline, "-EncodedCommand ")
	if i < 0 {
		t.Fatalf("没有走 -EncodedCommand: %s", cmdline)
	}
	field := strings.TrimSpace(cmdline[i+len("-EncodedCommand "):])
	// runner 记的是 name+" "+join(args)，末段即编码后的脚本。
	field = field[strings.LastIndex(field, " ")+1:]
	raw, err := base64.StdEncoding.DecodeString(field)
	if err != nil {
		t.Fatalf("EncodedCommand 不是合法 base64: %v", err)
	}
	u16 := make([]uint16, 0, len(raw)/2)
	for j := 0; j+1 < len(raw); j += 2 {
		u16 = append(u16, uint16(raw[j])|uint16(raw[j+1])<<8)
	}
	return string(utf16.Decode(u16))
}

// 新建会话：命令就是 agent 自己的名字，落在项目目录，不带 resume 参数。
func TestNewSessionLaunchesAgentInProjectDir(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	a.setList([]sessions.Session{{Agent: "codex", ID: "c1", Cwd: `D:\Work\AgentDeck`, Last: time.Unix(5, 0)}})
	dir := t.TempDir()
	r := a.NewResumeService()
	if err := r.NewSession(dir, "claude"); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 1 {
		t.Fatalf("终端启动次数 = %d", len(*ran))
	}
	script := decodeLaunched(t, (*ran)[0])
	if !strings.Contains(script, "claude") {
		t.Fatalf("命令里没有目标 agent: %s", script)
	}
	if strings.Contains(script, "resume") {
		t.Fatalf("新建会话不该带 resume: %s", script)
	}
	if !strings.Contains(script, dir) {
		t.Fatalf("没有落在项目目录: %s", script)
	}
}

// 默认路径：跟着当前项目走；没有当前项目时不往用户主目录根上塞东西。
func TestDefaultPathForNewProject(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	ps := a.NewProjectService()
	got := ps.DefaultPathFor(`D:\Work\AgentDeck`, "")
	if got != filepath.Join(`D:\Work`, "新项目") {
		t.Fatalf("有当前项目时 = %q", got)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	blank := ps.DefaultPathFor("", "")
	if blank != filepath.Join(home, "Documents", "新项目") {
		t.Fatalf("无当前项目时 = %q, 期望落在 Documents 下", blank)
	}
	if strings.EqualFold(blank, filepath.Join(home, "新项目")) {
		t.Fatalf("回落到了用户主目录根: %q", blank)
	}
}

func TestProjectAgentAndFailedCreatePreserveDeclaration(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	ps := a.NewProjectService()
	dir := filepath.Join(t.TempDir(), "project[1]'s")
	if _, err := ps.CreateWithAgent(dir, "原名", "qoder"); err != nil {
		t.Fatal(err)
	}
	if err := ps.Rename(dir, "新名"); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.CreateWithAgent(dir, "不应覆盖", "claude"); err == nil {
		t.Fatal("重复新建覆盖了已有项目")
	}
	rows := a.NewSessionService().Projects()
	if len(rows) != 1 || rows[0].Name != "新名" || rows[0].Agent != "qoder" {
		t.Fatalf("项目平台或名称丢失: %+v", rows)
	}
	if err := a.NewResumeService().NewSession(dir, ""); err != nil {
		t.Fatal(err)
	}
	script := decodeLaunched(t, (*ran)[0])
	if want := "Set-Location -LiteralPath " + quotePowerShellPath(dir) + " -ErrorAction Stop; qodercli"; script != want {
		t.Fatalf("script = %q, want %q", script, want)
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.declared.AddWithAgent(blocked, "保留", "codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create(blocked, "覆盖"); err == nil {
		t.Fatal("文件被当作目录")
	}
	all, err := ps.Declared()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.Path == blocked && (p.Name != "保留" || p.Agent != "codex") {
			t.Fatalf("失败创建改写声明: %+v", p)
		}
	}
	for _, bad := range []string{"claude-desktop", "claude; bad"} {
		if _, err := ps.CreateWithAgent(filepath.Join(t.TempDir(), "unused"), "", bad); err == nil {
			t.Fatalf("接受平台 %q", bad)
		}
	}
	if err := a.NewResumeService().NewSession(dir+"\n", "claude"); err == nil {
		t.Fatal("接受控制字符路径")
	}
}

func TestCLICommandsAndReadOnlyFallback(t *testing.T) {
	for agent, want := range map[string]string{"qoder": "qodercli", "qoder-cn": "qoderclicn", "cursor": "cursor-agent", "cursor-agent": "cursor-agent", "claude": "claude"} {
		if got := cliCommand(agent); got != want {
			t.Fatalf("%s: %s != %s", agent, got, want)
		}
	}
	a, _, _, _ := newTestApp(t)
	a.setList([]sessions.Session{{Agent: "claude-desktop", ID: "d1", ReadOnly: true}, {Agent: "claude-desktop", ID: "d2", ReadOnly: true}, {Agent: "codex", ID: "c1"}, {Agent: "claude", ID: "r1", Retained: true}})
	if got := a.NewResumeService().defaultAgent(); got != "codex" {
		t.Fatalf("fallback = %q", got)
	}
}

// agent 缺省取本机会话最多的那个；白名单外的拒绝，不猜。
func TestNewSessionAgentFallbackAndWhitelist(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	a.setList([]sessions.Session{
		{Agent: "codex", ID: "c1", Cwd: `D:\P`, Last: time.Unix(1, 0)},
		{Agent: "codex", ID: "c2", Cwd: `D:\P`, Last: time.Unix(2, 0)},
		{Agent: "claude", ID: "k1", Cwd: `D:\P`, Last: time.Unix(3, 0)},
		{Agent: "claude-desktop", ID: "d1", Cwd: `D:\P`, Last: time.Unix(4, 0), ReadOnly: true},
	})
	r := a.NewResumeService()
	if err := r.NewSession(t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	if script := decodeLaunched(t, (*ran)[len(*ran)-1]); !strings.Contains(script, "codex") {
		t.Fatalf("缺省 agent 挑了错的: %s", script)
	}
	for _, bad := range []string{"claude-desktop", "wsl", "../x"} {
		before := len(*ran)
		if err := r.NewSession(t.TempDir(), bad); err == nil {
			t.Errorf("接受了不支持的 agent %q", bad)
		}
		if len(*ran) != before {
			t.Errorf("拒绝后仍启动了终端: %q", bad)
		}
	}
	// 目录不存在要拒绝：不替用户建一路目录。
	if err := r.NewSession(`D:\不存在的目录xyz`, "claude"); err == nil {
		t.Error("接受了不存在的项目目录")
	}
}
