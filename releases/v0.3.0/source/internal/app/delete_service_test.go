package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func deletionFixture(t *testing.T, a *App) sessions.Session {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	project := filepath.Join(home, "projects", "test-project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	sess := claudeFixture(t, project, "11111111-2222-4333-8444-555555555555", "需要手动删除的临时样本")
	sessions.Reset()
	a.setList([]sessions.Session{sess})
	if _, _, _, err := a.readConversation(sess); err != nil {
		t.Fatal(err)
	}
	if err := a.NewMarkService().SetNote("claude:"+sess.ID, "保留在恢复备份中的备注"); err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestManualDeleteBacksUpOriginalAndSavedDataAndCanUndo(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	sess := deletionFixture(t, a)
	key := "claude:" + sess.ID
	svc := a.NewSessionService()
	original, _ := os.ReadFile(sess.Path)
	preview, err := svc.PreviewDeleteSessions([]string{key}, false)
	if err != nil || preview.Sessions != 1 || preview.NativeFiles < 1 || preview.Token == "" {
		t.Fatalf("Concrete preview missing: %+v %v", preview, err)
	}
	if _, err := os.Stat(sess.Path); err != nil {
		t.Fatal("Preview must not change native data")
	}
	if _, err := svc.DeleteSessions("invalid-token"); err == nil {
		t.Fatal("Deletion without a valid preview must be rejected")
	}
	report, err := svc.DeleteSessions(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.DeletedKeys) != 1 || report.UndoToken == "" || report.BackupPath == "" {
		t.Fatalf("Missing recoverable result: %+v", report)
	}
	if _, err := os.Stat(sess.Path); !os.IsNotExist(err) {
		t.Fatal("Original source must be removed")
	}
	if _, _, found, err := a.history.Get(key); found || err != nil {
		t.Fatal("Saved history must be removed")
	}
	if err := a.NewMarkService().SetNote(key, "延迟到达的备注"); err == nil {
		t.Fatal("A delayed mark write must not recreate deleted annotations")
	}
	if _, _, _, err := a.readConversation(sess); err == nil {
		t.Fatal("An old index snapshot must not recapture deleted sessions")
	}
	if rows := svc.List(Filter{}); containsSession(rows, key) {
		t.Fatal("Refresh resurrected deleted records")
	}
	if _, err := svc.RestoreDeletedSessions(report.UndoToken); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(sess.Path)
	if err != nil || string(restored) != string(original) {
		t.Fatal("Undo must restore exact original content")
	}
	if _, _, found, err := a.history.Get(key); !found || err != nil {
		t.Fatal("Undo must restore saved versions")
	}
	mark, err := a.marks.Get(key)
	if err != nil || mark.Note != "保留在恢复备份中的备注" {
		t.Fatal("Undo must restore the selected note")
	}
	if _, err := svc.DeleteSessions(preview.Token); err == nil {
		t.Fatal("Deletion tokens must be one-use")
	}
}

func TestManualDeleteRefusesChangedSourceAndUnavailableBackup(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "backup-fails", true: "source-changes"}[changed], func(t *testing.T) {
			a, _, _, _ := newTestApp(t)
			sess := deletionFixture(t, a)
			key := "claude:" + sess.ID
			svc := a.NewSessionService()
			preview, err := svc.PreviewDeleteSessions([]string{key}, false)
			if err != nil {
				t.Fatal(err)
			}
			if changed {
				if err := os.WriteFile(sess.Path, []byte("原程序又写入了新的内容"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(filepath.Join(filepath.Dir(a.cfg.LibraryPath), "deletions"), []byte("阻止备份目录创建"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := svc.DeleteSessions(preview.Token); err == nil {
				t.Fatal("Changed sources and backup failures must prevent deletion")
			}
			if _, err := os.Stat(sess.Path); err != nil {
				t.Fatal("Rejected deletion changed original data")
			}
			if _, _, found, err := a.history.Get(key); !found || err != nil {
				t.Fatal("Rejected deletion changed saved history")
			}
		})
	}
}

func TestManualDeleteRejectsUnsupportedSourceWithoutMutation(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	sess := claudeFixture(t, t.TempDir(), "unsupported", "原始记录")
	sess.Agent, sess.WSL = "claude", "Ubuntu"
	a.setList([]sessions.Session{sess})
	if _, err := a.NewSessionService().PreviewDeleteSessions([]string{"claude:unsupported"}, false); err == nil {
		t.Fatal("WSL deletion must be explicitly unsupported")
	}
	data, err := os.ReadFile(sess.Path)
	if err != nil || !strings.Contains(string(data), "原始记录") {
		t.Fatal("Unsupported record was modified")
	}
}

func TestManualDeleteRollsBackEarlierSourcesWhenLaterDeletionFails(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	first := deletionFixture(t, a)
	second := claudeFixture(t, filepath.Dir(first.Path), "66666666-7777-4888-8999-000000000000", "另一个临时会话")
	sessions.Reset()
	a.setList([]sessions.Session{first, second})
	if _, _, _, err := a.readConversation(second); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.nativeDelete = func(plan *sessions.NativeDeletePlan) error {
		calls++
		if calls == 2 {
			return fmt.Errorf("模拟第二个来源写入失败")
		}
		return plan.Delete()
	}
	svc := a.NewSessionService()
	preview, err := svc.PreviewDeleteSessions([]string{"claude:" + first.ID, "claude:" + second.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.DeleteSessions(preview.Token); err == nil || !strings.Contains(err.Error(), "已自动恢复") {
		t.Fatalf("Expected verified rollback, got %v", err)
	}
	for _, sess := range []sessions.Session{first, second} {
		if _, err := os.Stat(sess.Path); err != nil {
			t.Fatal("Rollback lost a source", err)
		}
		if _, _, found, err := a.history.Get("claude:" + sess.ID); !found || err != nil {
			t.Fatal("Rollback lost saved history", err)
		}
	}
}

func TestManualDeletePersistsNoRecaptureAcrossRestart(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	sess := deletionFixture(t, a)
	key := "claude:" + sess.ID
	data, err := os.ReadFile(sess.Path)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := a.NewSessionService().PreviewDeleteSessions([]string{key}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.NewSessionService().DeleteSessions(preview.Token); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(sess.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	other, err := New(a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(other.Close)
	sessions.Reset()
	if containsSession(other.NewSessionService().List(Filter{}), key) {
		t.Fatal("Restart recaptured a manually deleted native session")
	}
	if _, _, _, err = other.readConversation(sess); err == nil {
		t.Fatal("Restart allowed a deleted capture")
	}
}

func TestManualDeleteCodexFamilyBatchAndUndo(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	dir := filepath.Join(home, "sessions", "2026", "10", "17")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ids := []string{"11111111-2222-4333-8444-555555555555", "66666666-7777-4888-8999-000000000000"}
	var list []sessions.Session
	var index, promptHistory string
	for i, id := range ids {
		path := filepath.Join(dir, "rollout-2026-10-17T10-00-00-"+id+".jsonl")
		meta, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]any{"id": id, "source": "cli"}})
		if err := os.WriteFile(path, append(meta, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		sess := sessions.Session{Agent: "codex", ID: id, Path: path, Role: "main", RootSession: id, Title: "临时批量样本"}
		if i == 1 {
			sess.Role, sess.ParentSession, sess.RootSession = "subagent", ids[0], ids[0]
		}
		list = append(list, sess)
		line, _ := json.Marshal(map[string]any{"id": id, "title": "临时标题"})
		index += string(line) + "\n"
		line, _ = json.Marshal(map[string]any{"session_id": id, "text": "临时输入"})
		promptHistory += string(line) + "\n"
	}
	if err := os.WriteFile(filepath.Join(home, "session_index.jsonl"), []byte(index), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "history.jsonl"), []byte(promptHistory), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(home, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT NOT NULL)`, `CREATE TABLE thread_spawn_edges (parent_thread_id TEXT NOT NULL, child_thread_id TEXT PRIMARY KEY, status TEXT NOT NULL)`} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, sess := range list {
		if _, err := db.Exec(`INSERT INTO threads VALUES(?,?)`, sess.ID, sess.Path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO threads VALUES('unselected','other.jsonl')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO thread_spawn_edges VALUES(?,?,'done')`, ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	sessions.Reset()
	a.setList(list)
	svc := a.NewSessionService()
	preview, err := svc.PreviewDeleteSessions([]string{"codex:" + ids[0]}, false)
	if err != nil || preview.Sessions != 1 || preview.Children != 1 {
		t.Fatalf("Explicit default scope: %+v %v", preview, err)
	}
	preview, err = svc.PreviewDeleteSessions([]string{"codex:" + ids[0]}, true)
	if err != nil || preview.Sessions != 2 {
		t.Fatalf("Opt-in scope: %+v %v", preview, err)
	}
	// The Codex state DB and the shared index files (session_index.jsonl,
	// history.jsonl) are rewritten in place, so the preview must count them as
	// metadata rather than as files that get deleted.
	if preview.NativeFiles != 2 || preview.Metadata != 3 {
		t.Fatalf("preview file split: %+v", preview)
	}
	report, err := svc.DeleteSessions(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("Deleted unrelated thread or failed batch: %d %v", count, err)
	}
	if _, err = svc.RestoreDeletedSessions(report.UndoToken); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("Undo incomplete: %d %v", count, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM thread_spawn_edges`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("Undo edge missing/duplicated: %d %v", count, err)
	}
	for _, sess := range list {
		if _, err := os.Stat(sess.Path); err != nil {
			t.Fatal(err)
		}
	}
}

func containsSession(rows []SessionView, key string) bool {
	for _, row := range rows {
		if row.Key == key {
			return true
		}
	}
	return false
}
