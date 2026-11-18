package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportedClaudeChatsPreviewSearchMarksAndExport(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	source := filepath.Join(t.TempDir(), "conversations.json")
	data := `[{"uuid":"cloud-chat","name":"October history","created_at":"2026-09-01T10:00:00Z","updated_at":"2026-10-17T10:00:00Z","chat_messages":[{"sender":"human","text":"查找十月份的历史记录","created_at":"2026-10-17T10:00:00Z"},{"sender":"assistant","text":"完整会话可以被检索","created_at":"2026-10-17T10:01:00Z"}]}]`
	if err := os.WriteFile(source, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := a.NewSessionService()
	if report, err := svc.ImportClaudeHistory(source, "工作账号"); err != nil || report.Conversations != 1 {
		t.Fatalf("import: %+v %v", report, err)
	}
	rows := svc.List(Filter{Agent: "claude-desktop"})
	if len(rows) != 1 || rows[0].Account != "工作账号" || rows[0].Source != "import" || rows[0].Messages != 2 || rows[0].Resume != "" {
		t.Fatalf("list: %+v", rows)
	}
	key := rows[0].Key
	if command, err := a.NewResumeService().Command(key); err != nil || command != "" {
		t.Fatalf("imported cloud chat advertised CLI resume: %q %v", command, err)
	}
	if tr, err := svc.Transcript(key); err != nil || len(tr.Parts) != 2 {
		t.Fatalf("transcript: %+v %v", tr, err)
	}
	if err := a.NewMarkService().SetFavorite(key, true); err != nil {
		t.Fatal(err)
	}
	if got := svc.List(Filter{Favorites: true}); len(got) != 1 {
		t.Fatalf("favorite: %d", len(got))
	}
	if got, err := a.NewSearchService().Query("十月份", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("scan: %+v %v", got, err)
	}
	a.buildIndex(true)
	if got, err := a.NewSearchService().Query("十月份", 20); err != nil || got.Mode != "index" || len(got.Hits) != 1 {
		t.Fatalf("index: %+v %v", got, err)
	}
	p, err := svc.ExportMarkdown(key, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(md), "查找十月份") {
		t.Fatalf("Markdown: %v", err)
	}
	svc.Refresh()
	if got := svc.List(Filter{Favorites: true}); len(got) != 1 || got[0].Key != key {
		t.Fatalf("refresh lost identity or mark: %+v", got)
	}
}
