package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/chatgpthistory"
)

func deleteAllPermanently(t *testing.T, svc *SessionService, keys []string) {
	t.Helper()
	preview, err := svc.PreviewDeleteSessions(keys, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteSessionsPermanently(preview.Token); err != nil {
		t.Fatal(err)
	}
}

func transcriptCopies(a *App, key string) int {
	files, _ := chatgpthistory.TranscriptFiles(filepath.Join(a.cfg.ImportDir, "chatgpt"), strings.TrimPrefix(key, "chatgpt:"))
	return len(files)
}

// A deleted conversation is not written back by the next import, the preview lists it,
// and only the ones chosen come back.
func TestDeletedImportsAreListedAndOnlyChosenOnesComeBack(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	svc := a.NewSessionService()
	first := filepath.Join(t.TempDir(), "e1.json")
	if err := os.WriteFile(first, []byte(chatgptShard), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportChatGPT(first, "个人", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{}
	for _, r := range svc.List(Filter{Agent: "chatgpt"}) {
		keys[r.Title] = r.Key
	}
	a.NewSettingsService().SetImportDeleteEnabled(true)
	deleteAllPermanently(t, svc, []string{keys["整理发票"], keys["闲聊"]})
	if len(svc.List(Filter{Agent: "chatgpt"})) != 0 {
		t.Fatal("两个都删掉了")
	}

	// the next export (bytes differ, so it is a new batch)
	next := filepath.Join(t.TempDir(), "e2.json")
	if err := os.WriteFile(next, []byte(strings.ReplaceAll(chatgptShard, `"update_time":1760000100`, `"update_time":1780000100`)), 0o600); err != nil {
		t.Fatal(err)
	}
	pre, err := svc.PreviewChatGPT(next, "个人")
	if err != nil || len(pre.Deleted) != 2 {
		t.Fatalf("预览应列出这份导出里被删除过的会话: %+v %v", pre.Deleted, err)
	}
	titles := map[string]string{}
	for _, d := range pre.Deleted {
		titles[d.Title] = d.ID
	}
	if titles["整理发票"] == "" || titles["闲聊"] == "" {
		t.Fatalf("%+v", pre.Deleted)
	}
	rep, err := svc.ImportChatGPT(next, "个人", nil, false, nil)
	if err != nil || rep.SkippedDeleted != 2 || rep.Restored != 0 {
		t.Fatalf("默认一个都不恢复: %+v %v", rep, err)
	}
	for _, k := range keys {
		if transcriptCopies(a, k) != 0 {
			t.Fatalf("被删除的会话重新导入后不该写回磁盘: %s", k)
		}
	}

	// restore only 整理发票; an id that was never deleted is ignored
	rep, err = svc.ImportChatGPT(next, "个人", nil, false, []string{titles["整理发票"], "gpt_" + strings.Repeat("e", 64)})
	if err != nil || rep.Restored != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	now := titlesNow(svc)
	if !now["整理发票"] || now["闲聊"] {
		t.Fatalf("只有勾选的恢复: %v", now)
	}
	if transcriptCopies(a, keys["闲聊"]) != 0 {
		t.Fatal("没勾选的仍不该写回")
	}
	// the record on disk follows: after a restart only 闲聊 is still blocked
	blocked, err := loadDeletedKeys(a.cfg.LibraryPath)
	if err != nil || blocked[keys["整理发票"]] || !blocked[keys["闲聊"]] {
		t.Fatalf("删除记录应只剩没恢复的: %v %v", blocked, err)
	}
	// and the restored one can be deleted again
	deleteAllPermanently(t, svc, []string{keys["整理发票"]})
	if titlesNow(svc)["整理发票"] {
		t.Fatal("恢复后的会话可以再次删除")
	}
}

// Restoring rewrites the deletion record atomically and only for what was asked.
func TestUnblockDeletedImportsKeepsOtherKeysBlocked(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	dir := filepath.Join(deletionDir(a.cfg.LibraryPath), "delete-20260101-000000-x")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	k1, k2, k3 := "chatgpt:gpt_"+strings.Repeat("1", 64), "chatgpt:gpt_"+strings.Repeat("2", 64), "codex:other"
	body := `{"Version":1,"State":"deleted","Keys":["` + k1 + `","` + k2 + `","` + k3 + `"],"Owned":["` + k1 + `","` + k2 + `","` + k3 + `"]}`
	path := filepath.Join(dir, "operation.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	a.blockDeleted([]string{k1, k2, k3}, true)
	if err := a.unblockDeletedImports([]string{strings.TrimPrefix(k1, "chatgpt:")}); err != nil {
		t.Fatal(err)
	}
	blocked, err := loadDeletedKeys(a.cfg.LibraryPath)
	if err != nil || blocked[k1] || !blocked[k2] || !blocked[k3] {
		t.Fatalf("只解除被要求的那个: %v %v", blocked, err)
	}
	if a.deleted[k1] || !a.deleted[k2] {
		t.Fatal("内存里的屏蔽也要同步")
	}
	if err := a.unblockDeletedImports([]string{strings.TrimPrefix(k2, "chatgpt:")}); err != nil {
		t.Fatal(err)
	}
	// the codex key keeps the record alive
	if blocked, _ = loadDeletedKeys(a.cfg.LibraryPath); blocked[k2] || !blocked[k3] {
		t.Fatalf("%v", blocked)
	}
	// a record that is not in the "deleted" state is never touched
	other := filepath.Join(deletionDir(a.cfg.LibraryPath), "delete-20260102-000000-y")
	os.MkdirAll(other, 0o700)
	pending := `{"Version":1,"State":"deleting","Keys":["` + k2 + `"],"Owned":["` + k2 + `"]}`
	os.WriteFile(filepath.Join(other, "operation.json"), []byte(pending), 0o600)
	a.unblockDeletedImports([]string{strings.TrimPrefix(k2, "chatgpt:")})
	if b, _ := os.ReadFile(filepath.Join(other, "operation.json")); string(b) != pending {
		t.Fatal("进行中的删除记录不能改")
	}
}
