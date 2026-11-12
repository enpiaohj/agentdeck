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
	a.setList([]sessions.Session{{Agent: "codex", ID: "c1", Cwd: `D:\Work\Notes`, Last: time.Unix(5, 0)}})
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
	got := ps.DefaultPathFor(`D:\Work\Notes`, "")
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

// 目录状态探针：新建项目靠它区分「已存在」与「将创建」，新建会话靠它拦下
// 不存在的目录。只回答现状、不替前端做取舍。
func TestDirectoryStateGuidesBothFlows(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	r := a.NewResumeService()
	dir := t.TempDir()
	if state, err := r.DirectoryState(dir); err != nil || state != "dir" {
		t.Fatalf("已存在目录 = %q, %v", state, err)
	}
	missing := filepath.Join(t.TempDir(), "还没建")
	if state, err := r.DirectoryState(missing); err != nil || state != "missing" {
		t.Fatalf("不存在的路径 = %q, %v", state, err)
	}
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, err := r.DirectoryState(file); err != nil || state != "file" {
		t.Fatalf("指向文件 = %q, %v", state, err)
	}
	if _, err := r.DirectoryState(`relative\x`); err == nil {
		t.Fatal("接受了相对路径")
	}
	// 预览与新建会话对「目录存不存在」的判断必须一致：目录不在，两条路都拒绝，
	// 且都不启动任何东西。
	if _, err := r.PreviewCommand(missing, "claude"); err == nil {
		t.Fatal("预览接受了不存在的目录")
	}
	if err := r.NewSession(missing, "claude"); err == nil {
		t.Fatal("新建会话接受了不存在的目录")
	}
	if len(*ran) != 0 {
		t.Fatalf("被拒的路径仍启动了终端: %+v", *ran)
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
	for agent, want := range map[string]string{"qoder": "qodercli", "qoder-cn": "qoderclicn", "cursor": "cursor-agent", "cursor-agent": "cursor-agent", "claude": "claude", "dsh": "dsh"} {
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

// 项目改了显示名称后，会话列表必须跟着改——否则「重命名项目」名不副实。
func TestProjectDisplayNameAppliesToSessions(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	root := t.TempDir()
	dir := filepath.Join(root, "OldName")
	loose := filepath.Join(root, "Loose")
	ps := a.NewProjectService()
	a.setList([]sessions.Session{
		{Agent: "claude", ID: "k1", Cwd: dir, Last: time.Unix(1, 0)},
		{Agent: "claude", ID: "k2", Cwd: loose, Last: time.Unix(2, 0)},
	})
	if _, err := ps.Create(dir, "新名字"); err != nil {
		t.Fatal(err)
	}
	byID := map[string]string{}
	for _, row := range a.NewSessionService().List(Filter{}) {
		byID[row.ID] = row.Project
	}
	if byID["k1"] != "新名字" {
		t.Fatalf("声明过的目录应显示设置里的名称: %+v", byID)
	}
	if byID["k2"] != "Loose" {
		t.Fatalf("未声明的目录仍用目录名: %+v", byID)
	}
	// 改回目录名后，列表跟着回到目录名。
	if err := ps.Rename(dir, "OldName"); err != nil {
		t.Fatal(err)
	}
	for _, row := range a.NewSessionService().List(Filter{}) {
		if row.ID == "k1" && row.Project != "OldName" {
			t.Fatalf("改名后会话项目名 = %q", row.Project)
		}
	}
}

// 平台可用性：命令取固定映射，只报是否找到，不启动任何进程。
func TestAvailableReportsCliCommands(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	got := map[string]string{}
	for _, row := range a.NewResumeService().Available() {
		got[row.Agent] = row.Command
	}
	for agent, want := range map[string]string{"qoder": "qodercli", "qoder-cn": "qoderclicn", "cursor": "cursor-agent", "claude": "claude", "dsh": "dsh"} {
		if got[agent] != want {
			t.Errorf("%s 命令 = %q, want %q", agent, got[agent], want)
		}
	}
	if len(got) != len(cliAgents) {
		t.Errorf("平台数 = %d, want %d", len(got), len(cliAgents))
	}
	if len(*ran) != 0 {
		t.Fatal("查询可用性启动了进程")
	}
	// 本机一定有 claude 或没有都不该 panic：Found 只反映 PATH 查找结果。
	for _, row := range a.NewResumeService().Available() {
		if row.Command == "" {
			t.Errorf("%s 没有对应命令", row.Agent)
		}
	}
}

// 预览命令不启动进程，且与真正启动时交给终端的一致。
func TestPreviewCommandMatchesLaunch(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	r := a.NewResumeService()
	dir := t.TempDir()
	preview, err := r.PreviewCommand(dir, "qoder")
	if err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 0 {
		t.Fatal("预览命令启动了进程")
	}
	if err := r.NewSession(dir, "qoder"); err != nil {
		t.Fatal(err)
	}
	if launched := decodeLaunched(t, (*ran)[0]); preview != launched {
		t.Fatalf("预览与启动不一致:\n预览 %q\n实际 %q", preview, launched)
	}
	// 不可启动的平台在预览阶段就被拒绝，不落到终端。
	if _, err := r.PreviewCommand(dir, "claude-desktop"); err == nil {
		t.Fatal("预览接受了不可启动的平台")
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

// 新建项目只改变项目清单：发 projects:changed，不发会触发会话整表重载的
// sessions:changed。
func TestCreateProjectEmitsProjectsChangedOnly(t *testing.T) {
	a, rec, _, _ := newTestApp(t)
	svc := a.NewProjectService()
	if _, err := svc.Create(filepath.Join(t.TempDir(), "新项目"), ""); err != nil {
		t.Fatal(err)
	}
	if !rec.has(EventProjectsChanged) || rec.has(EventSessionsChanged) {
		t.Fatalf("事件不符: projects=%v sessions=%v", rec.has(EventProjectsChanged), rec.has(EventSessionsChanged))
	}
}
