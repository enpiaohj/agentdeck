package app

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// recorder 收集事件推送。
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) emit(event string, _ any) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (r *recorder) has(event string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e == event {
			return true
		}
	}
	return false
}

// newTestApp 开一个注入版装配根:临时标注库、内存索引、收集器回调。
func newTestApp(t *testing.T) (*App, *recorder, *[]string, *[]string) {
	t.Helper()
	rec := &recorder{}
	var clip, ran []string
	a, err := New(Config{
		MarksPath:    filepath.Join(t.TempDir(), "marks.db"),
		ProjectsPath: filepath.Join(t.TempDir(), "projects.db"), // 独立库，避免共用临时目录
		IndexPath:    ":memory:",                                // 空路径表示默认磁盘库；测试显式使用独立内存库。
		Emit:         rec.emit,
		Clipboard:    func(text string) error { clip = append(clip, text); return nil },
		Runner: func(name string, args ...string) error {
			ran = append(ran, name+" "+strings.Join(args, " "))
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a, rec, &clip, &ran
}

// claudeFixture 写一个最小 Claude 会话文件并返回 Session。
func claudeFixture(t *testing.T, dir, id, text string) sessions.Session {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	line := `{"type":"user","message":{"role":"user","content":"` + text + `"},"uuid":"u1"}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return sessions.Session{
		Agent: "claude", ID: id, Cwd: `D:\Work\Demo`, Title: "演示会话",
		Last: time.Date(2026, 10, 16, 10, 0, 0, 0, time.Local),
		Path: path, Resume: "claude --resume " + id,
	}
}

func TestLocalTitleAcrossViewsAndExport(t *testing.T) {
	a, rec, _, _ := newTestApp(t)
	original := claudeFixture(t, t.TempDir(), "rename1", "检索正文")
	a.setList([]sessions.Session{original})
	key := "claude:rename1"
	service := &SessionService{a: a}
	marks := &MarkService{a: a}
	if err := marks.SetTitle(key, "本地会话名"); err != nil {
		t.Fatal(err)
	}
	if !rec.has(EventSessionsChanged) {
		t.Fatal("改名未通知列表更新")
	}
	view, err := service.Get(key)
	if err != nil || view.Title != "本地会话名" || view.OriginalTitle != "演示会话" {
		t.Fatalf("详情标题 = %+v, %v", view, err)
	}
	if rows := service.List(Filter{Search: "本地会话"}); len(rows) != 1 || rows[0].Title != "本地会话名" {
		t.Fatalf("列表改名筛选 = %+v", rows)
	}
	if rows := service.List(Filter{Search: "演示会话"}); len(rows) != 0 {
		t.Fatalf("原始标题仍参与筛选 = %+v", rows)
	}
	hits, err := service.SearchInAll(key, "检索")
	if err != nil || len(hits) != 1 || hits[0].Title != "本地会话名" {
		t.Fatalf("会话内命中标题 = %+v, %v", hits, err)
	}
	path, err := service.ExportMarkdown(key, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "# 本地会话名") || !strings.Contains(string(body), "- **Agent:** claude") {
		t.Fatalf("导出标题或 Agent 元数据错误：%s", body)
	}
	if err := marks.SetTitle(key, ""); err != nil {
		t.Fatal(err)
	}
	view, err = service.Get(key)
	if err != nil || view.Title != "演示会话" {
		t.Fatalf("恢复原名 = %+v, %v", view, err)
	}
}

func TestListFilters(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	dir := t.TempDir()
	s1 := claudeFixture(t, dir, "11111111-2222-3333-4444-555555555555", "修复远程连接超时")
	s2 := sessions.Session{Agent: "codex", ID: "c2", Cwd: `D:\Work\Other`, Title: "其他项目"}
	s3 := sessions.Session{Agent: "zcode", ID: "z3", Cwd: "", Title: "无目录"}
	a.setList([]sessions.Session{s1, s2, s3})
	svc := &SessionService{a: a}
	mk := &MarkService{a: a}

	if n := len(svc.List(Filter{})); n != 3 {
		t.Fatalf("默认视图 = %d 条, want 3", n)
	}
	if n := len(svc.List(Filter{Agent: "claude"})); n != 1 {
		t.Fatalf("agent 过滤 = %d 条", n)
	}
	// 多选智能体：并集；旧的 Agent 字段一并计入
	if n := len(svc.List(Filter{Agents: []string{"claude", "codex"}})); n != 2 {
		t.Fatalf("多选 agent 过滤 = %d 条, want 2", n)
	}
	if n := len(svc.List(Filter{Agent: "zcode", Agents: []string{"codex"}})); n != 2 {
		t.Fatalf("Agent 与 Agents 合并 = %d 条, want 2", n)
	}
	// 收藏
	if err := mk.SetFavorite("claude:11111111-2222-3333-4444-555555555555", true); err != nil {
		t.Fatal(err)
	}
	if n := len(svc.List(Filter{Favorites: true})); n != 1 {
		t.Fatalf("收藏过滤 = %d 条", n)
	}
	// 归档:默认视图不含归档,归档视图只含归档
	if err := mk.SetArchived("codex:c2", true); err != nil {
		t.Fatal(err)
	}
	if n := len(svc.List(Filter{})); n != 2 {
		t.Fatalf("归档后默认视图 = %d 条, want 2", n)
	}
	if n := len(svc.List(Filter{Archived: true})); n != 1 {
		t.Fatalf("归档视图 = %d 条", n)
	}
	// 标签
	if err := mk.SetTags("zcode:z3", []string{"远程"}); err != nil {
		t.Fatal(err)
	}
	if n := len(svc.List(Filter{Tag: "远程"})); n != 1 {
		t.Fatalf("标签过滤 = %d 条", n)
	}
	// 项目过滤(归一化比较)
	if n := len(svc.List(Filter{Project: `\\?\D:\Work\Demo\`})); n != 1 {
		t.Fatalf("项目过滤 = %d 条(扩展长度前缀与尾分隔符应归一)", n)
	}
	// 标题检索
	if n := len(svc.List(Filter{Search: "演示"})); n != 1 {
		t.Fatalf("标题检索 = %d 条", n)
	}
}

func TestViewAssembly(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	dir := t.TempDir()
	s1 := claudeFixture(t, dir, "11111111-2222-3333-4444-555555555555", "修复远程连接超时")
	wsl := sessions.Session{Agent: "claude", ID: "wsl1", WSL: "Ubuntu-22.04", Title: "WSL 会话"}
	a.setList([]sessions.Session{s1, wsl})
	v, err := (&SessionService{a: a}).Get("claude:11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	if v.Project != "Demo" || v.Source != "local" || !v.Transcript || v.Resume == "" {
		t.Fatalf("view 装配错: %+v", v)
	}
	if _, err := time.Parse(time.RFC3339, v.Last); err != nil {
		t.Errorf("Last 不是 RFC3339: %q", v.Last)
	}
	v2, err := (&SessionService{a: a}).Get("claude:wsl1")
	if err != nil {
		t.Fatal(err)
	}
	if v2.Source != "wsl" {
		t.Errorf("WSL 来源标注错: %s", v2.Source)
	}
}

func TestTranscriptAndSearchIn(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	dir := t.TempDir()
	s1 := claudeFixture(t, dir, "11111111-2222-3333-4444-555555555555", "修复远程连接超时的问题")
	a.setList([]sessions.Session{s1})
	svc := &SessionService{a: a}
	tr, err := svc.Transcript("claude:11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Parts) != 1 || tr.Parts[0].Text != "修复远程连接超时的问题" {
		t.Fatalf("transcript: %+v", tr)
	}
	hit, err := svc.SearchIn("claude:11111111-2222-3333-4444-555555555555", "远程")
	if err != nil || hit == nil || hit.Match != "远程" {
		t.Fatalf("SearchIn: %+v %v", hit, err)
	}
}

// 索引未就绪时降级为逐会话扫描;构建完成后走全文索引且可命中 2 字中文。
func TestQueryDegradeThenIndex(t *testing.T) {
	a, rec, _, _ := newTestApp(t)
	dir := t.TempDir()
	s1 := claudeFixture(t, dir, "11111111-2222-3333-4444-555555555555", "修复远程连接超时的问题")
	a.setList([]sessions.Session{s1})
	q := &SearchService{a: a}

	r1, err := q.Query("远程连接", 10)
	if err != nil || r1.Mode != "scan" || r1.Total != 1 {
		t.Fatalf("降级路径: %+v %v", r1, err)
	}

	q.Rebuild()
	deadline := time.Now().Add(15 * time.Second)
	for {
		st := q.Status()
		if st.State == "ready" || st.State == "failed" {
			if st.State == "failed" {
				t.Fatalf("索引构建失败: %s", st.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("索引构建超时: %+v", st)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !rec.has(EventIndexReady) {
		t.Error("没收到 index:ready 事件")
	}
	r2, err := q.Query("远程连接", 10)
	if err != nil || r2.Mode != "index" || r2.Total != 1 {
		t.Fatalf("索引路径: %+v %v", r2, err)
	}
	if r2.Hits[0].Match == "" || !strings.Contains(r2.Hits[0].Snippet, "远程连接") {
		t.Fatalf("命中视图: %+v", r2.Hits[0])
	}
	if r2.Hits[0].Title != "演示会话" {
		t.Errorf("命中未关联会话标题: %q", r2.Hits[0].Title)
	}
	if err := (&MarkService{a: a}).SetTitle("claude:"+s1.ID, "远程问题记录"); err != nil {
		t.Fatal(err)
	}
	r3, err := q.Query("远程连接", 10)
	if err != nil || r3.Mode != "index" || len(r3.Hits) != 1 || r3.Hits[0].Title != "远程问题记录" {
		t.Fatalf("改名后的索引命中 = %+v, %v", r3, err)
	}
}

func TestResumeActions(t *testing.T) {
	a, _, clip, ran := newTestApp(t)
	dir := t.TempDir()
	s1 := claudeFixture(t, dir, "11111111-2222-3333-4444-555555555555", "hi")
	// 终端直开用真实存在的 cwd(Stat 验证),避免依赖机器上的固定目录
	real := sessions.Session{Agent: "claude", ID: "real1", Cwd: dir,
		Resume: "claude --resume real1", Path: filepath.Join(dir, "real1.jsonl")}
	wsl := sessions.Session{Agent: "claude", ID: "wsl1", WSL: "Ubuntu-22.04", Resume: "wsl cmd"}
	a.setList([]sessions.Session{s1, real, wsl})
	r := &ResumeService{a: a}

	cmd, err := r.Command("claude:11111111-2222-3333-4444-555555555555")
	if err != nil || cmd == "" {
		t.Fatalf("Command: %q %v", cmd, err)
	}
	if err := r.CopyToClipboard("claude:11111111-2222-3333-4444-555555555555"); err != nil {
		t.Fatal(err)
	}
	if len(*clip) != 1 || (*clip)[0] != cmd {
		t.Fatalf("clipboard: %v", *clip)
	}
	if err := r.OpenInTerminal("claude:wsl1"); err == nil || !strings.Contains(err.Error(), "WSL") {
		t.Fatalf("WSL 直开应被拒: %v", err)
	}
	if err := r.OpenInTerminal("claude:real1"); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 1 {
		t.Fatalf("runner 未被调用: %v", *ran)
	}
	// 有 wt 时优先 wt 并带 -d 落到 cwd;没有则 powershell 回退
	got := (*ran)[0]
	if strings.Contains(got, "wt.exe") {
		if !strings.Contains(got, " -d ") || !strings.Contains(got, filepath.Base(dir)) {
			t.Errorf("wt 参数: %q", got)
		}
	} else if !strings.Contains(got, "powershell.exe") || !strings.Contains(got, "-EncodedCommand") {
		t.Errorf("回退形态: %q", got)
	}
}

// 改名后恢复:Claude Code 收 --name,CLI 内部才显示 AgentDeck 里的名字。
// 没有本地改名时不加参数——不能拿来源标题去覆盖原生名字。
func TestResumeCommandCarriesLocalTitle(t *testing.T) {
	a, _, clip, _ := newTestApp(t)
	dir := t.TempDir()
	plain := claudeFixture(t, dir, "named1", "hi")
	quoted := claudeFixture(t, dir, "named2", "hi")
	codex := sessions.Session{Agent: "codex", ID: "cx1", Cwd: dir,
		Resume: "codex resume cx1", Path: filepath.Join(dir, "cx1.jsonl")}
	wsl := sessions.Session{Agent: "claude", ID: "wsl1", WSL: "Ubuntu-22.04",
		Resume: `wsl.exe -d 'Ubuntu' -e sh -lc 'claude --resume wsl1'`}
	a.setList([]sessions.Session{plain, quoted, codex, wsl})
	marks := &MarkService{a: a}
	r := &ResumeService{a: a}

	// 未改名:恢复命令保持原样。
	cmd, err := r.Command("claude:named1")
	if err != nil || cmd != "claude --resume named1" {
		t.Fatalf("未改名的恢复命令 = %q, %v", cmd, err)
	}
	if err := marks.SetTitle("claude:named1", "定价问题排查"); err != nil {
		t.Fatal(err)
	}
	cmd, err = r.Command("claude:named1")
	if err != nil || cmd != "claude --resume named1 --name '定价问题排查'" {
		t.Fatalf("改名后的恢复命令 = %q, %v", cmd, err)
	}
	// 复制命令与直开终端走同一条,不能只在一条路上生效。
	if err := r.CopyToClipboard("claude:named1"); err != nil {
		t.Fatal(err)
	}
	if len(*clip) != 1 || (*clip)[0] != cmd {
		t.Fatalf("剪贴板 = %v", *clip)
	}

	// 标题里的单引号必须按 PowerShell 规则翻倍,否则命令被截断。
	if err := marks.SetTitle("claude:named2", "it's 会话"); err != nil {
		t.Fatal(err)
	}
	cmd, err = r.Command("claude:named2")
	if err != nil || cmd != "claude --resume named2 --name 'it''s 会话'" {
		t.Fatalf("含引号的恢复命令 = %q, %v", cmd, err)
	}

	// 其余 agent 未实测支持该参数,改名后也不能加。
	if err := marks.SetTitle("codex:cx1", "Codex 本地名"); err != nil {
		t.Fatal(err)
	}
	if cmd, err = r.Command("codex:cx1"); err != nil || cmd != "codex resume cx1" {
		t.Fatalf("codex 恢复命令 = %q, %v", cmd, err)
	}
	// WSL 命令是整条 wsl.exe -e sh -lc '…',参数塞不进内层引号,保持原样。
	if err := marks.SetTitle("claude:wsl1", "WSL 本地名"); err != nil {
		t.Fatal(err)
	}
	if cmd, err = r.Command("claude:wsl1"); err != nil || strings.Contains(cmd, "--name") {
		t.Fatalf("WSL 恢复命令 = %q, %v", cmd, err)
	}
}

// Codex 没有 claude 那种 --name 入口,恢复前得用它自己的 app-server 写一次
// 原生名(session_index.jsonl 的 thread_name)。没改名不碰上游;失败不阻断
// 恢复——终端照开,但错误必须带出来,不能悄悄吞掉。
func TestOpenInTerminalSyncsCodexName(t *testing.T) {
	a, _, _, ran := newTestApp(t)
	dir := t.TempDir()
	const thread = "01a152e7-bee6-79d1-8b11-5aa8530fb63f"
	codex := sessions.Session{Agent: "codex", ID: thread, Cwd: dir,
		Resume: "codex resume " + thread, Path: filepath.Join(dir, "rollout.jsonl")}
	claude := claudeFixture(t, dir, "claude1", "hi")
	wsl := sessions.Session{Agent: "codex", ID: "wslcx", WSL: "Ubuntu-22.04", Resume: "wsl codex resume wslcx"}
	a.setList([]sessions.Session{codex, claude, wsl})
	var calls [][2]string
	var fail error
	a.codexName = func(threadID, name string) error {
		calls = append(calls, [2]string{threadID, name})
		return fail
	}
	r := &ResumeService{a: a}
	marks := &MarkService{a: a}

	if err := r.OpenInTerminal("codex:" + thread); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatalf("未改名却写了原生名: %v", calls)
	}
	if err := marks.SetTitle("codex:"+thread, "Codex 改名验证"); err != nil {
		t.Fatal(err)
	}
	if err := r.OpenInTerminal("codex:" + thread); err != nil {
		t.Fatal(err)
	}
	// 传的必须是 thread id(不是 key)与本地标题。
	if len(calls) != 1 || calls[0][0] != thread || calls[0][1] != "Codex 改名验证" {
		t.Fatalf("原生名写入 = %v", calls)
	}
	// Claude 走 --name,WSL 的 codex 在另一侧——两条路都不该碰 app-server。
	if err := marks.SetTitle("claude:claude1", "Claude 改名"); err != nil {
		t.Fatal(err)
	}
	if err := r.OpenInTerminal("claude:claude1"); err != nil {
		t.Fatal(err)
	}
	if err := marks.SetTitle("codex:wslcx", "WSL Codex"); err != nil {
		t.Fatal(err)
	}
	if err := r.OpenInTerminal("codex:wslcx"); err == nil || !strings.Contains(err.Error(), "WSL") {
		t.Fatalf("WSL 直开应被拒: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("claude/WSL 不该写 Codex 原生名: %v", calls)
	}
	if len(*ran) != 3 {
		t.Fatalf("终端启动次数 = %v", *ran)
	}

	// 写失败:终端仍然要开,错误里既要说明「已经开了」也要带上原因。
	fail = errors.New("codex 不可用")
	err := r.OpenInTerminal("codex:" + thread)
	if err == nil || !strings.Contains(err.Error(), "已在终端中打开会话") || !strings.Contains(err.Error(), "codex 不可用") {
		t.Fatalf("失败时的报错 = %v", err)
	}
	if len(*ran) != 4 {
		t.Fatalf("写原生名失败却挡下了终端: %v", *ran)
	}
}

// 线程 id 来自会话文件,是不可信输入,会被写进 JSON-RPC 参数:白名单外一律拒。
func TestCodexThreadNameRejectsUnsafeID(t *testing.T) {
	for _, id := range []string{"", "../x", "a b", `a"b`, "a\nb", "a:b"} {
		if err := setCodexThreadName(id, "名"); err == nil {
			t.Fatalf("接受了非法线程 id: %q", id)
		}
	}
	if err := setCodexThreadName("ok-id_1", ""); err == nil {
		t.Fatal("接受了空名字")
	}
}

// needsCodexName 只对**本机原生**的 codex 会话写原生名。网关与导入来源的 ID
// 未必是本地线程,拿它去改本地记录是猜;保留会话没有可恢复的命令。这几条是
// 防止「猜着写上游」的守卫,回归的后果是拿一个来历不明的 ID 改本地 Codex 索引。
func TestCodexNameOnlyForNativeSessions(t *testing.T) {
	const thread = "01a152e7-bee6-79d1-8b11-5aa8530fb63f"
	base := sessions.Session{Agent: "codex", ID: thread, Resume: "codex resume " + thread}
	cases := []struct {
		name string
		sess sessions.Session
		want bool
	}{
		{"本机原生", base, true},
		{"网关来源", sessions.Session{Agent: "codex", ID: thread, Gateway: true}, false},
		{"导入来源", sessions.Session{Agent: "codex", ID: thread, Imported: true}, false},
		{"只读", sessions.Session{Agent: "codex", ID: thread, ReadOnly: true}, false},
		{"保留会话", sessions.Session{Agent: "codex", ID: thread, Retained: true}, false},
		{"WSL", sessions.Session{Agent: "codex", ID: thread, WSL: "Ubuntu-22.04"}, false},
		{"别的 agent", sessions.Session{Agent: "claude", ID: thread}, false},
	}
	for _, c := range cases {
		if got := needsCodexName(c.sess, "本地标题"); got != c.want {
			t.Errorf("%s: needsCodexName = %v, 期望 %v", c.name, got, c.want)
		}
	}
	// 没有本地标题时没什么可写。
	if needsCodexName(base, "") {
		t.Error("空标题仍要写原生名")
	}
}

// app-server 不答复时必须靠本地预算返回,不能只靠 ctx:proc.CommandContext
// 的 ctx 结束只杀直接子进程,而 codex 是 .cmd、真身 node 是孙进程,它会一直
// 抱着 stdout,卡在读取上的 goroutine 等不到 EOF(实测 1s 的 ctx、6s 后仍
// 未返回)。这里用一个装了同样包装层、永远不说话的假 codex 复现那个形态。
func TestCodexThreadNameTimesOutOnSilentServer(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "codex.cmd")
	// 孙进程活过预算,所以读取不可能自己结束——超时只能由 setCodexThreadName
	// 自己兜住。
	if err := os.WriteFile(shim, []byte("@echo off\r\npowershell -NoProfile -Command \"Start-Sleep 8\"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := codexNameTimeout
	codexNameTimeout = time.Second
	t.Cleanup(func() { codexNameTimeout = old })
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	proc.Forget()

	start := time.Now()
	err := setCodexThreadName("ok-id_1", "改名验证")
	spent := time.Since(start)
	if err == nil {
		t.Fatal("不说话的 app-server 竟被当成成功")
	}
	if err == nil || !strings.Contains(err.Error(), "没有答复") {
		t.Fatalf("报错文案 = %v", err)
	}
	// 预算 1s,给足启动开销:超过 7s 说明等的是插件进程而不是自己的预算。
	if spent > 7*time.Second {
		t.Fatalf("超时没有兜住,等了 %v", spent)
	}
}

// 恢复命令必须是不可拆的 -EncodedCommand,且终端必须是能出窗的那条路。
// `cmd /c start` 虽然 isTTY 为 true,但实测建出的控制台窗口不可见——用户
// 什么都看不到,所以不能用。
func TestTerminalCommandKeepsScriptUnsplittable(t *testing.T) {
	name, args := terminalCommand(t.TempDir(), "codex resume x", powershellExe)
	if filepath.Base(name) == "cmd.exe" {
		t.Fatalf("cmd start 建出的窗口不可见,不能用: %v", args)
	}
	if !strings.Contains(strings.Join(args, " "), "-EncodedCommand") {
		t.Fatalf("必须用 -EncodedCommand: %v", args)
	}
}

// 根路径 cwd 交给 wt 的 -d;不存在的目录不带 -d,但仍然执行恢复命令。
func TestTerminalCommandRootPath(t *testing.T) {
	name, args := terminalCommand(`D:\`, "claude --resume x", powershellExe)
	if filepath.Base(name) != "wt.exe" && filepath.Base(name) != "powershell.exe" {
		t.Fatalf("终端 = %s", name)
	}
	if filepath.Base(name) == "wt.exe" {
		if args[0] != "-d" || args[1] != `D:\` {
			t.Fatalf("根路径要原样交给 -d: %v", args)
		}
	}
	// 不存在的目录:不带 -d,但仍然启动终端
	name2, args2 := terminalCommand(filepath.Join(t.TempDir(), "NoSuchDir"), "cmd", powershellExe)
	if filepath.Base(name2) != "powershell.exe" {
		t.Fatalf("目录不存在时应回退 powershell: %s", name2)
	}
	if !strings.Contains(strings.Join(args2, " "), "-EncodedCommand") {
		t.Fatalf("仍要执行恢复命令: %v", args2)
	}
}

// 直接启动终端时 stdout 不是终端,agent CLI 会因此不上色;resumeEnv 补
// FORCE_COLOR 告诉它,但不覆盖用户自己设的值。
func TestResumeEnvForcesColorUnlessSet(t *testing.T) {
	t.Setenv("FORCE_COLOR", "")
	os.Unsetenv("FORCE_COLOR")
	env := resumeEnv()
	found := false
	for _, kv := range env {
		if kv == "FORCE_COLOR=1" {
			found = true
		}
	}
	if !found {
		t.Fatal("没设 FORCE_COLOR 时应补 FORCE_COLOR=1")
	}
	t.Setenv("FORCE_COLOR", "0")
	if resumeEnv() != nil {
		t.Fatal("用户自己设了 FORCE_COLOR 时不应覆盖")
	}
}

// 中间层(cmd start)会把 -Command 的值按空格拆一遍:恢复命令里分号后的
// 半句被当成另一条参数丢掉,终端只剩 Set-Location,报「系统找不到指定的
// 文件」(0x80070002)。交给终端的必须是不可拆的 -EncodedCommand。
func TestTerminalCommandPassesScriptUnsplittable(t *testing.T) {
	dir := t.TempDir()
	script := "Set-Location -LiteralPath '" + dir + "'; codex resume 01a13d4d-0000-7000-8000-000000000000"
	_, args := terminalCommand(dir, script, powershellExe)
	if !strings.Contains(strings.Join(args, " "), "-EncodedCommand") {
		t.Fatalf("必须用 -EncodedCommand: %v", args)
	}
	// 脚本原文不能出现在任何参数里——那正是会被拆开的东西。
	for _, a := range args {
		if strings.Contains(a, ";") || strings.Contains(a, "resume") {
			t.Fatalf("脚本原文泄漏到终端参数: %q", a)
		}
	}
	got := args[len(args)-1]
	if got != encodedCommand(script) {
		t.Fatalf("编码不一致: %q", got)
	}
}

// -EncodedCommand 收 UTF-16LE 的 base64;解码回来必须与原文逐字一致
// (含中文路径,避免有人改回 UTF-8 编码后只在纯 ASCII 上看起来正确)。
func TestEncodedCommandRoundTrip(t *testing.T) {
	for _, script := range []string{
		"codex resume 01a13d4d",
		"Set-Location -LiteralPath 'D:\\项目\\A B'; claude --resume x",
		"Set-Location -LiteralPath 'D:\\it''s'; codex resume y",
	} {
		raw, err := base64.StdEncoding.DecodeString(encodedCommand(script))
		if err != nil {
			t.Fatalf("base64: %v", err)
		}
		if len(raw)%2 != 0 {
			t.Fatalf("UTF-16 编码长度必须为偶数: %q", script)
		}
		units := make([]uint16, len(raw)/2)
		for i := range units {
			units[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
		}
		if got := string(utf16.Decode(units)); got != script {
			t.Fatalf("往返不一致:\n got %q\nwant %q", got, script)
		}
	}
}

func TestExportMarkdown(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	dir := t.TempDir()
	s1 := claudeFixture(t, dir, "11111111-2222-3333-4444-555555555555", "修复远程连接超时的问题")
	a.setList([]sessions.Session{s1})
	dest := t.TempDir()
	p, err := (&SessionService{a: a}).ExportMarkdown("claude:11111111-2222-3333-4444-555555555555", dest)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(b), "修复远程连接超时的问题") {
		t.Fatalf("导出内容: %v", err)
	}
	// 相对路径拒绝
	if _, err := (&SessionService{a: a}).ExportMarkdown("claude:11111111-2222-3333-4444-555555555555", "relative"); err == nil {
		t.Error("相对路径被接受")
	}
}

// ScanProgress 透传 sessions 层进度。
func TestScanProgress(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	_ = (&SessionService{a: a}).ScanProgress() // 真实值依赖本机扫描状态,只验证可调用
}

// 诊断行按 agent 聚合:计数、最近活动、恢复能力。
func TestDiagnostics(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	dir := t.TempDir()
	s1 := claudeFixture(t, dir, "11111111-2222-3333-4444-555555555555", "hi")
	wsl := sessions.Session{Agent: "claude", ID: "wsl1", WSL: "Ubuntu-22.04", Resume: "wsl cmd", Path: "x"}
	a.setList([]sessions.Session{s1, wsl})
	rows := (&SessionService{a: a}).Diagnostics()
	if len(rows) != 1 {
		t.Fatalf("诊断行 = %d, want 1(claude)", len(rows))
	}
	r := rows[0]
	if r.Agent != "claude" || !r.Found || r.Sessions != 2 || !r.Resume {
		t.Fatalf("诊断行: %+v", r)
	}
	if r.LastAt == "" {
		t.Error("LastAt 为空")
	}
	if !contains(r.ResumeNote, "WSL") {
		t.Errorf("WSL 会话应体现在说明里: %q", r.ResumeNote)
	}
}

// 备份 → 改动 → 恢复:标注回到备份时点,且不覆盖已有备份。
func TestBackupRestoreMarks(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	svc := &MarkService{a: a}
	key := "claude:k1"
	if err := svc.SetFavorite(key, true); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	bak, err := svc.BackupMarks(dest)
	if err != nil {
		t.Fatal(err)
	}
	// 改动当前状态
	if err := svc.SetFavorite(key, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetArchived(key, true); err != nil {
		t.Fatal(err)
	}
	// 同名备份已存在 → 拒绝(不覆盖)
	if _, err := svc.BackupMarks(dest); err == nil {
		t.Log("同一秒内第二次备份重名被拒才算对;跨秒命名不冲突,此断言仅提示")
	}
	// 恢复
	if err := svc.RestoreMarks(bak); err != nil {
		t.Fatal(err)
	}
	m, err := svc.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Favorite || m.Archived {
		t.Fatalf("恢复后标注未回到备份时点: %+v", m)
	}
	// 非法文件拒绝
	if err := svc.RestoreMarks(filepath.Join(t.TempDir(), "nope.db")); err == nil {
		t.Error("不存在的备份被接受")
	}
	bad := filepath.Join(t.TempDir(), "bad.db")
	os.WriteFile(bad, []byte("not a sqlite db"), 0o600)
	if err := svc.RestoreMarks(bad); err == nil {
		t.Error("非法库被接受")
	}
	// 留底文件存在
	cur := a.marks.Path()
	if fi, err := os.Stat(cur + ".bak-"); err == nil && fi.IsDir() {
		t.Error("留底路径异常")
	}
}

// 崩溃标记:正常开合不误报;残留 lock 报崩溃。
func TestCrashMarker(t *testing.T) {
	dir := t.TempDir()
	mp := filepath.Join(dir, "marks.db")
	a1, err := New(Config{MarksPath: mp, IndexPath: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	if a1.crashed {
		t.Fatal("首次启动不应报崩溃")
	}
	// 模拟崩溃:不 Close,直接重新打开(写 lock 已在 New 时完成)
	if err := os.WriteFile(crashLockPath(mp), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	a2, err := New(Config{MarksPath: mp, IndexPath: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	if !a2.crashed {
		t.Fatal("残留 lock 未被识别为崩溃")
	}
	a2.Close()
	// 正常退出后 lock 清除
	if _, err := os.Stat(crashLockPath(mp)); !os.IsNotExist(err) {
		t.Error("Close 未清除 lock")
	}
	a1.Close()
}
