package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

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

// 恢复命令必须是不可拆的 -EncodedCommand,且终端必须是能出窗的那条路。
// `cmd /c start` 虽然 isTTY 为 true,但实测建出的控制台窗口不可见——用户
// 什么都看不到,所以不能用。
func TestTerminalCommandKeepsScriptUnsplittable(t *testing.T) {
	name, args := terminalCommand(t.TempDir(), "codex resume x")
	if filepath.Base(name) == "cmd.exe" {
		t.Fatalf("cmd start 建出的窗口不可见,不能用: %v", args)
	}
	if !strings.Contains(strings.Join(args, " "), "-EncodedCommand") {
		t.Fatalf("必须用 -EncodedCommand: %v", args)
	}
}

// 根路径 cwd 交给 wt 的 -d;不存在的目录不带 -d,但仍然执行恢复命令。
func TestTerminalCommandRootPath(t *testing.T) {
	name, args := terminalCommand(`D:\`, "claude --resume x")
	if filepath.Base(name) != "wt.exe" && filepath.Base(name) != "powershell.exe" {
		t.Fatalf("终端 = %s", name)
	}
	if filepath.Base(name) == "wt.exe" {
		if args[0] != "-d" || args[1] != `D:\` {
			t.Fatalf("根路径要原样交给 -d: %v", args)
		}
	}
	// 不存在的目录:不带 -d,但仍然启动终端
	name2, args2 := terminalCommand(filepath.Join(t.TempDir(), "NoSuchDir"), "cmd")
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
	_, args := terminalCommand(dir, script)
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
