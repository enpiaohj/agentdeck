package sessions

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const nativeTestID = "11111111-2222-3333-4444-555555555555"

func nativeWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func nativeCodexFixture(t *testing.T) (Session, []string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	name := "rollout-2026-10-17T10-00-00-" + nativeTestID + ".jsonl"
	paths := []string{filepath.Join(root, "sessions", "2026", "10", "17", name), filepath.Join(root, "archived_sessions", name), filepath.Join(root, "sessions", "2026", "10", "17", name+".zst"), filepath.Join(root, "sessions", "2026", "10", "17", "rollout-2026-10-17T11-00-00-"+nativeTestID+"_segment.jsonl")}
	for _, p := range paths {
		nativeWrite(t, p, "native test data")
	}
	return Session{Agent: "codex", ID: nativeTestID, Path: paths[0]}, paths
}

func TestNativeDeleteCodexCopiesBackupRestore(t *testing.T) {
	s, paths := nativeCodexFixture(t)
	other := filepath.Join(filepath.Dir(paths[0]), "rollout-2026-10-17T10-00-00-aaaaaaaa-2222-3333-4444-555555555555.jsonl")
	nativeWrite(t, other, "other")
	p, err := PrepareNativeDelete(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 4 {
		t.Fatalf("files=%v", p.Files)
	}
	if err = p.Delete(); err == nil {
		t.Fatal("delete without verified backup succeeded")
	}
	if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
		t.Fatal(err)
	}
	if err = p.Delete(); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("not deleted %s: %v", path, err)
		}
	}
	if b, err := os.ReadFile(other); err != nil || string(b) != "other" {
		t.Fatalf("other changed: %v", err)
	}
	if err = p.Restore(); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if b, err := os.ReadFile(path); err != nil || string(b) != "native test data" {
			t.Fatalf("restore %s: %v", path, err)
		}
	}
}

func TestNativeDeleteRefusesChangedOrForeignSources(t *testing.T) {
	s, paths := nativeCodexFixture(t)
	p, err := PrepareNativeDelete(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
		t.Fatal(err)
	}
	nativeWrite(t, paths[0], "changed")
	if err = p.Delete(); err == nil {
		t.Fatal("changed file deleted")
	}
	s.Path = filepath.Join(t.TempDir(), filepath.Base(paths[0]))
	nativeWrite(t, s.Path, "foreign")
	if _, err = PrepareNativeDelete(s); err == nil {
		t.Fatal("foreign source accepted")
	}
	for _, bad := range []Session{{Agent: "codex", ID: nativeTestID, WSL: "Ubuntu"}, {Agent: "codex", ID: nativeTestID, Gateway: true}, {Agent: "claude", ID: "../escape"}, {Agent: "opencode", ID: nativeTestID}, {Agent: "claude", ID: nativeTestID, Imported: true}} {
		if _, err = PrepareNativeDelete(bad); err == nil {
			t.Fatalf("unsupported session accepted: %+v", bad)
		}
	}
}

func TestNativeDeleteClaudePrivateFilesAndRestoreConflict(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	main := filepath.Join(root, "projects", "-work", nativeTestID+".jsonl")
	sub := filepath.Join(root, "projects", "-work", nativeTestID, "subagents", "agent-a.jsonl")
	unknown := filepath.Join(filepath.Dir(sub), "keep.txt")
	nativeWrite(t, main, "")
	nativeWrite(t, sub, "subagent")
	nativeWrite(t, unknown, "unknown")
	p, err := PrepareNativeDelete(Session{Agent: "claude", ID: nativeTestID, Path: main})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 2 {
		t.Fatalf("files=%v", p.Files)
	}
	if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
		t.Fatal(err)
	}
	if err = p.Delete(); err != nil {
		t.Fatal(err)
	}
	nativeWrite(t, main, "new conversation")
	if err = p.Restore(); err == nil {
		t.Fatal("restore overwrote rewritten source")
	}
	if b, _ := os.ReadFile(main); string(b) != "new conversation" {
		t.Fatal("new source damaged")
	}
	if _, err = os.Stat(unknown); err != nil {
		t.Fatal(err)
	}
}

func nativeTestDB(t *testing.T, root string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(root, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range []string{
		`CREATE TABLE threads(id TEXT PRIMARY KEY,rollout_path TEXT NOT NULL,title TEXT)`,
		`CREATE TABLE thread_dynamic_tools(thread_id TEXT REFERENCES threads(id) ON DELETE CASCADE,position INTEGER,name TEXT,PRIMARY KEY(thread_id,position))`,
		`CREATE TABLE thread_read_receipts(thread_id TEXT PRIMARY KEY,revision TEXT)`,
		`CREATE TABLE thread_spawn_edges(parent_thread_id TEXT,child_thread_id TEXT PRIMARY KEY,status TEXT)`,
		`CREATE TABLE thread_attachments(id TEXT PRIMARY KEY,thread_id TEXT REFERENCES threads(id) ON DELETE CASCADE,payload TEXT)`,
		`CREATE TABLE guardian_review_feedback(id TEXT PRIMARY KEY,thread_id TEXT REFERENCES threads(id) ON DELETE CASCADE,record BLOB)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestNativeDeleteCodexMetadataAndMergeRestore(t *testing.T) {
	s, paths := nativeCodexFixture(t)
	root := CodexDir()
	db := nativeTestDB(t, root)
	for _, id := range []string{s.ID, "other"} {
		if _, err := db.Exec(`INSERT INTO threads VALUES(?,?,?)`, id, paths[0], "title"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE threads SET title=? WHERE id=?`, "title\x00with-newline\nend", s.ID); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{`INSERT INTO thread_dynamic_tools VALUES(?,0,'tool')`, `INSERT INTO thread_read_receipts VALUES(?,'r')`, `INSERT INTO thread_spawn_edges VALUES(?,'other','running')`, `INSERT INTO thread_attachments VALUES('attach',?,'payload')`, `INSERT INTO guardian_review_feedback VALUES('feedback',?,X'0102')`} {
		if _, err := db.Exec(stmt, s.ID); err != nil {
			t.Fatal(err)
		}
	}
	index := filepath.Join(root, "session_index.jsonl")
	nativeWrite(t, index, `{"id":"`+s.ID+`","thread_name":"selected"}`+"\n"+`{"id":"other","thread_name":"keep"}`+"\n")
	p, err := PrepareNativeDelete(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
		t.Fatal(err)
	}
	if err = p.Delete(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("threads %d %v", count, err)
	}
	for _, table := range []string{"thread_dynamic_tools", "thread_read_receipts", "thread_spawn_edges", "thread_attachments", "guardian_review_feedback"} {
		if err = db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	if _, err = db.Exec(`INSERT INTO threads VALUES('new','new-path','new title')`); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(index, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("{\"id\":\"new\"}\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Restore(); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("restore threads %d %v", count, err)
	}
	var restoredTitle string
	if err = db.QueryRow(`SELECT title FROM threads WHERE id=?`, s.ID).Scan(&restoredTitle); err != nil || restoredTitle != "title\x00with-newline\nend" {
		t.Fatalf("exact text restore failed: %v", err)
	}
	for _, id := range []string{s.ID, "other", "new"} {
		idx, err := nativeReadIndex(index, id)
		if err != nil || len(idx.Lines) != 1 {
			t.Fatalf("index %s: %+v %v", id, idx, err)
		}
	}
}

func TestNativeDeleteUnknownDBSchemaFailsBeforeMutation(t *testing.T) {
	s, paths := nativeCodexFixture(t)
	db := nativeTestDB(t, CodexDir())
	if _, err := db.Exec(`CREATE TABLE surprise(thread_id TEXT REFERENCES threads(id))`); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareNativeDelete(s); err == nil {
		t.Fatal("unknown schema accepted")
	}
	if _, err := os.Stat(paths[0]); err != nil {
		t.Fatal("source mutated", err)
	}
}

func TestNativeDeleteMetadataChangesAreRefused(t *testing.T) {
	s, _ := nativeCodexFixture(t)
	db := nativeTestDB(t, CodexDir())
	if _, err := db.Exec(`INSERT INTO threads VALUES(?,'path','old')`, s.ID); err != nil {
		t.Fatal(err)
	}
	p, err := PrepareNativeDelete(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE threads SET title='new' WHERE id=?`, s.ID); err != nil {
		t.Fatal(err)
	}
	if err = p.Delete(); err == nil {
		t.Fatal("metadata changes not detected")
	}
	if err = p.Restore(); err != nil {
		t.Fatal("unmutated plan restore", err)
	}
}

func TestNativeDeleteRejectsSymlink(t *testing.T) {
	s, paths := nativeCodexFixture(t)
	other := filepath.Join(t.TempDir(), "outside")
	nativeWrite(t, other, "outside")
	if err := os.Remove(paths[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, paths[0]); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := PrepareNativeDelete(s); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestNativeDeleteRejectsJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows junction")
	}
	s, _ := nativeCodexFixture(t)
	root := CodexDir()
	link := filepath.Join(root, "sessions")
	target := filepath.Join(root, "real-sessions")
	if err := os.Rename(link, target); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("create fixture junction: %s %v", out, err)
	}
	defer os.Remove(link)
	if _, err := PrepareNativeDelete(s); err == nil {
		t.Fatal("junction accepted")
	}
}

func TestNativeDeleteMissingRetainedAndDamagedBackup(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	path := filepath.Join(root, "sessions", "rollout-2026-10-17T10-00-00-"+nativeTestID+".jsonl")
	p, err := PrepareNativeDelete(Session{Agent: "codex", ID: nativeTestID, Path: path, Retained: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
		t.Fatal(err)
	}
	if err = p.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err = PrepareNativeDelete(Session{Agent: "codex", ID: nativeTestID, Path: filepath.Join(t.TempDir(), "foreign.jsonl"), Retained: true}); err == nil {
		t.Fatal("missing foreign source accepted")
	}
	s, paths := nativeCodexFixture(t)
	p, err = PrepareNativeDelete(s)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "backup")
	if err = p.Backup(dir); err != nil {
		t.Fatal(err)
	}
	nativeWrite(t, filepath.Join(dir, "manifest.json"), "damaged")
	if err = p.Delete(); err == nil {
		t.Fatal("damaged backup accepted")
	}
	if _, err = os.Stat(paths[0]); err != nil {
		t.Fatal("source deleted", err)
	}
}

func TestNativeDeleteSharedHistoryMergeAndRestore(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		t.Run(agent, func(t *testing.T) {
			var s Session
			var root string
			if agent == "codex" {
				s, _ = nativeCodexFixture(t)
				root = CodexDir()
			} else {
				root = t.TempDir()
				t.Setenv("CLAUDE_CONFIG_DIR", root)
				path := filepath.Join(root, "projects", "-work", nativeTestID+".jsonl")
				nativeWrite(t, path, "")
				s = Session{Agent: agent, ID: nativeTestID, Path: path}
			}
			path := filepath.Join(root, "history.jsonl")
			key := "session_id"
			if agent == "claude" {
				key = "sessionId"
			}
			nativeWrite(t, path, `{"`+key+`":"`+s.ID+`","text":"private"}`+"\n"+`{"`+key+`":"other","text":"keep"}`+"\n")
			p, err := PrepareNativeDelete(s)
			if err != nil {
				t.Fatal(err)
			}
			if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
				t.Fatal(err)
			}
			if err = p.Delete(); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "private") {
				t.Fatal("selected history remains")
			}
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.WriteString("{\"" + key + "\":\"new\"}\n")
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if err = p.Restore(); err != nil {
				t.Fatal(err)
			}
			body, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, word := range []string{"private", "other", "new"} {
				if !strings.Contains(string(body), word) {
					t.Fatalf("missing %s in restored history", word)
				}
			}
		})
	}
}

func TestNativeDeleteBatchSharedMetadataAndUndo(t *testing.T) {
	s, paths := nativeCodexFixture(t)
	root := CodexDir()
	other := "22222222-2222-3333-4444-555555555555"
	otherPath := strings.Replace(paths[0], s.ID, other, 1)
	nativeWrite(t, otherPath, "other original")
	db := nativeTestDB(t, root)
	for _, id := range []string{s.ID, other, "keep"} {
		if _, err := db.Exec(`INSERT INTO threads VALUES(?,?,'title')`, id, paths[0]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO thread_spawn_edges VALUES(?,?,'done')`, s.ID, other); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct{ name, key string }{{"session_index.jsonl", "id"}, {"history.jsonl", "session_id"}} {
		body := ""
		for _, id := range []string{s.ID, other, "keep"} {
			body += `{"` + spec.key + `":"` + id + `"}` + "\n"
		}
		nativeWrite(t, filepath.Join(root, spec.name), body)
	}
	a, err := PrepareNativeDelete(s)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PrepareNativeDelete(Session{Agent: "codex", ID: other, Path: otherPath})
	if err != nil {
		t.Fatal(err)
	}
	plans := []*NativeDeletePlan{a, b}
	for _, p := range plans {
		if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
			t.Fatal(err)
		}
	}
	for i, p := range plans {
		if err = p.RebaseAfterOwnBatchChange(plans[:i]); err != nil {
			t.Fatal(err)
		}
		if err = p.Delete(); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("threads %d %v", count, err)
	}
	for i := len(plans) - 1; i >= 0; i-- {
		if err = plans[i].Restore(); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("restored threads %d %v", count, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM thread_spawn_edges`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("restored edge %d %v", count, err)
	}
}

func TestNativeDeleteBatchRebaseRejectsExternalIndexChange(t *testing.T) {
	s, paths := nativeCodexFixture(t)
	other := "22222222-2222-3333-4444-555555555555"
	otherPath := strings.Replace(paths[0], s.ID, other, 1)
	nativeWrite(t, otherPath, "other")
	index := filepath.Join(CodexDir(), "session_index.jsonl")
	nativeWrite(t, index, `{"id":"`+s.ID+`"}`+"\n"+`{"id":"`+other+`"}`+"\n")
	a, err := PrepareNativeDelete(s)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PrepareNativeDelete(Session{Agent: "codex", ID: other, Path: otherPath})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*NativeDeletePlan{a, b} {
		if err = p.Backup(filepath.Join(t.TempDir(), "backup")); err != nil {
			t.Fatal(err)
		}
	}
	if err = a.Delete(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(index, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{\"id\":\"external\"}\n")
	f.Close()
	if err = b.RebaseAfterOwnBatchChange([]*NativeDeletePlan{a}); err == nil {
		t.Fatal("external index change accepted")
	}
	if err = a.Restore(); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(index)
	if !strings.Contains(string(body), "external") {
		t.Fatal("external row lost")
	}
}
