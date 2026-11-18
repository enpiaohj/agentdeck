package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func writeClaudeSettings(t *testing.T, home, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeRetentionReadsSettingAndFindsSessionsNearCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	a, _, _, _ := newTestApp(t)
	dir := t.TempDir()
	mk := func(id string, ageDays int) sessions.Session {
		s := claudeFixture(t, dir, id, "x")
		mtime := time.Now().Add(-time.Duration(ageDays) * 24 * time.Hour)
		if err := os.Chtimes(s.Path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		s.Title = id
		return s
	}
	a.setList([]sessions.Session{mk("fresh", 2), mk("soon", 26), mk("overdue", 33), mk("edge", 22)})
	svc := a.NewSettingsService()

	// 没有设置文件：用默认的 30 天
	got := svc.ClaudeRetention()
	if got.Days != 30 || got.Explicit || got.Error != "" || got.Checked != 4 {
		t.Fatalf("默认: %+v", got)
	}
	if got.Soon != 2 || got.Unsaved != 2 || len(got.Risks) != 2 || got.Risks[0].Title != "overdue" || got.Risks[0].DaysLeft > 0 || got.Risks[1].Title != "soon" || got.Risks[1].DaysLeft != 4 {
		t.Fatalf("快到期的会话: %+v", got)
	}
	if got.Snippet != `"cleanupPeriodDays": 365` {
		t.Fatalf("建议片段: %q", got.Snippet)
	}

	// 调大之后没有会话快到期
	writeClaudeSettings(t, home, `{"cleanupPeriodDays": 365, "model": "x"}`)
	if got := svc.ClaudeRetention(); got.Days != 365 || !got.Explicit || got.Soon != 0 || len(got.Risks) != 0 {
		t.Fatalf("365 天: %+v", got)
	}
	// 永久库里已有副本的，标出来
	writeClaudeSettings(t, home, `{"cleanupPeriodDays": 30}`)
	if _, err := a.NewSessionService().TranscriptWindow("claude:soon", -1, false); err != nil {
		t.Fatal(err)
	}
	got = svc.ClaudeRetention()
	if got.Unsaved != 1 {
		t.Fatalf("读过的会话已进永久库，不该再算作未保存: %+v", got)
	}
	// 0 表示 Claude 不保存会话：不统计到期
	writeClaudeSettings(t, home, `{"cleanupPeriodDays": 0}`)
	if got := svc.ClaudeRetention(); got.Days != 0 || !got.Explicit || got.Soon != 0 {
		t.Fatalf("0 天: %+v", got)
	}
	// 坏文件：报错但仍按默认值估算
	writeClaudeSettings(t, home, `{不是 json`)
	if got := svc.ClaudeRetention(); got.Error == "" || got.Days != 30 {
		t.Fatalf("坏文件: %+v", got)
	}
}

func TestClaudeRetentionIsReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	a, _, _, _ := newTestApp(t)
	writeClaudeSettings(t, home, `{"cleanupPeriodDays": 14}`)
	before, _ := os.ReadFile(filepath.Join(home, "settings.json"))
	a.NewSettingsService().ClaudeRetention()
	after, _ := os.ReadFile(filepath.Join(home, "settings.json"))
	if string(before) != string(after) {
		t.Fatal("只读检测不该改 settings.json")
	}
	if entries, _ := os.ReadDir(home); len(entries) != 1 {
		t.Fatalf("不该在 Claude 目录里创建任何文件: %v", entries)
	}
}
