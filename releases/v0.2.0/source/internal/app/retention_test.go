package app

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestReadConversationSurvivesSourceRemovalRefreshAndRestart(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	sess := claudeFixture(t, t.TempDir(), "retained", "持久保存的会话，不随上游清理")
	a.setList([]sessions.Session{sess})
	svc := a.NewSessionService()
	if _, err := svc.Transcript("claude:retained"); err != nil {
		t.Fatal(err)
	}
	if err := a.NewMarkService().SetNote("claude:retained", "保留备注"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sess.Path); err != nil {
		t.Fatal(err)
	}
	svc.Refresh()
	rows := svc.List(Filter{})
	if len(rows) != 1 || rows[0].ID != "retained" || rows[0].Note != "保留备注" {
		t.Fatalf("读取过的会话被上游清理带走: %+v", rows)
	}
	cfg := a.cfg
	a.Close()
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	svc = reopened.NewSessionService()
	tr, err := svc.Transcript("claude:retained")
	if err != nil || len(tr.Parts) != 1 || !strings.Contains(tr.Parts[0].Text, "持久保存") {
		t.Fatalf("重启后正文丢失: %+v %v", tr, err)
	}
	if cmd, err := reopened.NewResumeService().Command("claude:retained"); err != nil || cmd != "" {
		t.Fatalf("本地保留副本误显示可恢复: %q %v", cmd, err)
	}
	reopened.buildIndex()
	if got, err := reopened.NewSearchService().Query("持久保存", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("持久正文不可检索: %+v %v", got, err)
	}
	out, err := svc.ExportMarkdown("claude:retained", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(filepath.Clean(out))
	if err != nil || !strings.Contains(string(md), "持久保存") {
		t.Fatalf("本地副本不可导出: %v", err)
	}
}

func TestRetentionKeepsFullTextBeyondPreviewCutsAndIndexReset(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	text := strings.Repeat("保留", 9000) + "末尾内容"
	sess := claudeFixture(t, t.TempDir(), "full", text)
	a.setList([]sessions.Session{sess})
	a.buildIndex() // startup/rebuild captures sessions even before opening one
	preview, err := a.NewSessionService().Transcript("claude:full")
	if err != nil || preview.Parts[0].Cut == 0 {
		t.Fatalf("预览应保留显示上限: %+v %v", preview, err)
	}
	_, saved, found, err := a.history.Get("claude:full")
	if err != nil || !found || saved.Cut || saved.Parts[0].Text != text {
		t.Fatalf("永久库不应保存截断预览: found=%v err=%v", found, err)
	}
	if err := os.WriteFile(sess.Path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewSessionService().Transcript("claude:full"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sess.Path); err != nil {
		t.Fatal(err)
	}
	a.NewSessionService().Refresh()
	if got, err := a.NewSearchService().Query("末尾内容", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("快速搜索不应裁切正文: %+v %v", got, err)
	}
	a.buildIndex() // reset must not erase the independent permanent library
	stats := a.NewSessionService().LibraryStatus()
	if stats.Sessions != 1 || stats.Versions != 1 || stats.Error != "" {
		t.Fatalf("索引重建/空读取损坏永久库: %+v", stats)
	}
	if got, err := a.NewSearchService().Query("保留", 20); err != nil || len(got.Hits) == 0 {
		t.Fatalf("保留副本重建检索失败: %+v %v", got, err)
	}
	if got, err := a.NewSearchService().Query("末尾内容", 20); err != nil || len(got.Hits) != 1 || got.Mode != "index" {
		t.Fatalf("完整索引应收录预览之外的永久正文: %+v %v", got, err)
	}
}

func TestCaptureErrorIsNotClearedByAnotherSession(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	bad := claudeFixture(t, t.TempDir(), "unsaved", "不能静默丢失")
	bad.Cost = math.NaN() // JSON metadata cannot be saved; source still parses.
	good := claudeFixture(t, t.TempDir(), "saved", "正常保存")
	a.setList([]sessions.Session{bad, good})
	tr, err := a.NewSessionService().Transcript("claude:unsaved")
	if err != nil || tr.StorageWarning == "" {
		t.Fatalf("缺少本次保存失败警告: %+v %v", tr, err)
	}
	if _, err := a.NewSessionService().Transcript("claude:saved"); err != nil {
		t.Fatal(err)
	}
	if stats := a.NewSessionService().LibraryStatus(); stats.Error == "" {
		t.Fatal("其他会话成功保存不应清除未解决的失败")
	}
	bad.Cost = 0
	a.setList([]sessions.Session{bad, good})
	if _, err := a.NewSessionService().Transcript("claude:unsaved"); err != nil {
		t.Fatal(err)
	}
	if stats := a.NewSessionService().LibraryStatus(); stats.Error != "" {
		t.Fatalf("同一会话成功重试后应解除失败: %+v", stats)
	}
}

func TestCloseWaitsForCaptureCommit(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	sess := claudeFixture(t, t.TempDir(), "closing", "正常退出也必须落盘")
	a.setList([]sessions.Session{sess})
	read, release := make(chan struct{}), make(chan struct{})
	a.readNative = func(s sessions.Session) (sessions.Transcript, error) {
		tr, err := sessions.TranscriptForRetention(s)
		close(read)
		<-release
		return tr, err
	}
	captured := make(chan error, 1)
	go func() { _, _, _, err := a.readConversation(sess); captured <- err }()
	<-read
	closed := make(chan struct{})
	go func() { a.Close(); close(closed) }()
	select {
	case <-closed:
		close(release)
		t.Fatal("读取尚未提交就关闭了永久库")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-captured; err != nil {
		t.Fatal(err)
	}
	<-closed
	reopened, err := New(a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	_, tr, found, err := reopened.history.Get("claude:closing")
	if err != nil || !found || len(tr.Parts) != 1 {
		t.Fatalf("退出丢失在途读取: found=%v err=%v", found, err)
	}
}
