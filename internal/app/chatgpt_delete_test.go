package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/chatgpthistory"
)

// chatgptTwoExports writes two exports of the same chat: the first holds both
// conversations, the second only the project one, updated. Imported one after the
// other, the project conversation then has a copy in each of two batches (the
// first keeps the other conversation, so it stays).
func chatgptTwoExports(t *testing.T) (first, second string) {
	t.Helper()
	dir := t.TempDir()
	first = filepath.Join(dir, "e1.json")
	second = filepath.Join(dir, "e2.json")
	if err := os.WriteFile(first, []byte(chatgptShard), 0o600); err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(chatgptShard, ",\n{\"id\":\"c2\"")
	one := strings.Replace(strings.TrimPrefix(head, "[\n"), `"update_time":1760000100`, `"update_time":1770000100`, 1)
	if err := os.WriteFile(second, []byte("["+one+"]"), 0o600); err != nil {
		t.Fatal(err)
	}
	return first, second
}

func importBoth(t *testing.T, svc *SessionService) (key string, copies int) {
	t.Helper()
	first, second := chatgptTwoExports(t)
	for _, p := range []string{first, second} {
		if _, err := svc.ImportChatGPT(p, "个人", nil, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range svc.List(Filter{Agent: "chatgpt"}) {
		if r.Title == "整理发票" {
			key = r.Key
		}
	}
	if key == "" {
		t.Fatal("没有找到项目里的会话")
	}
	files, err := chatgpthistory.TranscriptFiles(filepath.Join(svc.a.cfg.ImportDir, "chatgpt"), strings.TrimPrefix(key, "chatgpt:"))
	if err != nil {
		t.Fatal(err)
	}
	return key, len(files)
}

func titlesNow(svc *SessionService) map[string]bool {
	out := map[string]bool{}
	for _, r := range svc.List(Filter{Agent: "chatgpt"}) {
		out[r.Title] = true
	}
	return out
}

func TestImportDeleteIsOffByDefault(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	svc := a.NewSessionService()
	key, copies := importBoth(t, svc)
	if copies != 2 {
		t.Fatalf("这个会话应在两个批次里各有一份: %d", copies)
	}
	settings := a.NewSettingsService()
	if settings.ImportDeleteEnabled() {
		t.Fatal("删除导入的聊天记录默认应关闭")
	}
	if _, err := svc.PreviewDeleteSessions([]string{key}, false); err == nil || !strings.Contains(err.Error(), "没有开启") {
		t.Fatalf("关闭时应拒绝并说明怎么开: %v", err)
	}
	if !titlesNow(svc)["整理发票"] {
		t.Fatal("被拒绝的删除不该改变任何东西")
	}
}

func TestDeleteImportedChatGPTTakesEveryCopyAndCanBeUndone(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	svc := a.NewSessionService()
	key, copies := importBoth(t, svc)
	if on, err := a.NewSettingsService().SetImportDeleteEnabled(true); err != nil || !on {
		t.Fatalf("开启: %v %v", on, err)
	}
	if err := a.NewMarkService().SetFavorite(key, true); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(a.cfg.ImportDir, "chatgpt")

	preview, err := svc.PreviewDeleteSessions([]string{key}, false)
	if err != nil || preview.Sessions != 1 || preview.NativeFiles != copies {
		t.Fatalf("预览应把每个批次里的副本都算上: %+v %v", preview, err)
	}
	report, err := svc.DeleteSessions(preview.Token)
	if err != nil || len(report.DeletedKeys) != 1 || report.UndoToken == "" || !strings.Contains(report.Message, "导入的会话") || !strings.Contains(report.Message, "不受影响") {
		t.Fatalf("%+v %v", report, err)
	}
	if titlesNow(svc)["整理发票"] {
		t.Fatal("删除后列表里不该再有它")
	}
	if left, _ := chatgpthistory.TranscriptFiles(root, strings.TrimPrefix(key, "chatgpt:")); len(left) != 0 {
		t.Fatalf("所有批次里的副本都该删掉: %v", left)
	}
	if !titlesNow(svc)["闲聊"] {
		t.Fatal("别的会话不能受影响")
	}
	if m, _ := a.marks.Get(key); m.Favorite {
		t.Fatal("它的收藏标注也该一并删掉")
	}

	// undo brings back the transcripts, the mark, and the row
	if _, err := svc.RestoreDeletedSessions(report.UndoToken); err != nil {
		t.Fatal(err)
	}
	if !titlesNow(svc)["整理发票"] {
		t.Fatal("撤销后应恢复")
	}
	if left, _ := chatgpthistory.TranscriptFiles(root, strings.TrimPrefix(key, "chatgpt:")); len(left) != copies {
		t.Fatalf("撤销后副本应全部回来: %d/%d", len(left), copies)
	}
	if m, _ := a.marks.Get(key); !m.Favorite {
		t.Fatal("撤销后收藏标注应回来")
	}
}

// A deleted conversation stays deleted: importing the same chat again does not bring it back.
func TestDeletedImportDoesNotComeBack(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	svc := a.NewSessionService()
	key, _ := importBoth(t, svc)
	if _, err := a.NewSettingsService().SetImportDeleteEnabled(true); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.PreviewDeleteSessions([]string{key}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteSessionsPermanently(preview.Token); err != nil {
		t.Fatal(err)
	}
	// a later export of the same chat, updated again
	again := filepath.Join(t.TempDir(), "e3.json")
	data := strings.ReplaceAll(chatgptShard, `"update_time":1760000100`, `"update_time":1780000100`)
	if err := os.WriteFile(again, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportChatGPT(again, "个人", nil, false); err != nil {
		t.Fatal(err)
	}
	if titlesNow(svc)["整理发票"] {
		t.Fatal("已删除的会话不该因为重新导入而回来")
	}
	if !titlesNow(svc)["闲聊"] {
		t.Fatal("没删的会话照常")
	}
}

// The switch is checked again when the delete runs, not only at preview.
func TestImportDeleteChecksTheSwitchAtExecution(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	svc := a.NewSessionService()
	key, copies := importBoth(t, svc)
	settings := a.NewSettingsService()
	settings.SetImportDeleteEnabled(true)
	preview, err := svc.PreviewDeleteSessions([]string{key}, false)
	if err != nil {
		t.Fatal(err)
	}
	settings.SetImportDeleteEnabled(false)
	if _, err := svc.DeleteSessions(preview.Token); err == nil {
		t.Fatal("开关在预览之后被关掉，执行必须拒绝")
	}
	if left, _ := chatgpthistory.TranscriptFiles(filepath.Join(a.cfg.ImportDir, "chatgpt"), strings.TrimPrefix(key, "chatgpt:")); len(left) != copies {
		t.Fatal("被拒绝时不能删任何文件")
	}
}

// Other imports (Claude's) stay protected whatever the switch says.
func TestImportDeleteDoesNotOpenOtherImports(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.NewSettingsService().SetImportDeleteEnabled(true)
	svc := a.NewSessionService()
	source := filepath.Join(t.TempDir(), "conversations.json")
	data := `[{"uuid":"cloud-chat","name":"Claude 聊天","created_at":"2026-09-01T10:00:00Z","updated_at":"2026-10-17T10:00:00Z","chat_messages":[{"sender":"human","text":"你好","created_at":"2026-10-17T10:00:00Z"}]}]`
	if err := os.WriteFile(source, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportClaudeHistory(source, "工作"); err != nil {
		t.Fatal(err)
	}
	rows := svc.List(Filter{Agent: "claude-desktop"})
	if len(rows) != 1 {
		t.Fatalf("%+v", rows)
	}
	if _, err := svc.PreviewDeleteSessions([]string{rows[0].Key}, false); err == nil {
		t.Fatal("Claude 的导入记录不该因为开了 ChatGPT 的开关就能删")
	}
}
