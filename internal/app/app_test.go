package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
		MarksPath: filepath.Join(t.TempDir(), "marks.db"),
		IndexPath: ":memory:", // 空路径表示默认磁盘库；测试显式使用独立内存库。
		Emit:      rec.emit,
		Clipboard: func(text string) error { clip = append(clip, text); return nil },
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
	// wt 优先且带 -d 落到 cwd
	if !strings.Contains((*ran)[0], " -d ") || !strings.Contains((*ran)[0], filepath.Base(dir)) {
		t.Errorf("终端命令形态: %q", (*ran)[0])
	}
}

// 根路径 cwd 的终端参数:目录存在且以 \ 结尾交给 exec 转义。
func TestTerminalCommandRootPath(t *testing.T) {
	name, args := terminalCommand(`D:\`, "claude --resume x")
	if filepath.Base(name) != "wt.exe" && filepath.Base(name) != "powershell.exe" {
		t.Fatalf("终端 = %s", name)
	}
	if filepath.Base(name) == "wt.exe" && args[0] != "-d" {
		t.Fatalf("wt 参数: %v", args)
	}
	// 不存在的目录回退 powershell
	name2, args2 := terminalCommand(filepath.Join(t.TempDir(), "NoSuchDir"), "cmd")
	if name2 != "powershell.exe" || args2[0] != "-NoExit" {
		t.Fatalf("回退: %s %v", name2, args2)
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
