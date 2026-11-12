package app

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"github.com/enpiaohj/agentdeck/internal/history"
	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/sessions"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDataBackupRoundTrip(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	s := a.NewSessionService()
	original, target := filepath.Join(t.TempDir(), "old"), t.TempDir()
	if err := a.declared.SetLocation(original, target); err != nil {
		t.Fatal(err)
	}
	if err := a.marks.SetNote("claude:test", "saved"); err != nil {
		t.Fatal(err)
	}
	p, err := s.BackupData(t.TempDir(), `{"theme":"dark"}`)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.InspectBackup(p)
	if err != nil || info.Preferences != `{"theme":"dark"}` {
		t.Fatalf("%+v %v", info, err)
	}
	if err := a.declared.RemoveLocation(original); err != nil {
		t.Fatal(err)
	}
	a.marks.SetNote("claude:test", "changed")
	report, err := s.RestoreData(p, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(report.RollbackPath); err != nil {
		t.Fatal(err)
	}
	if got, err := a.declared.Location(original); err != nil || got != target {
		t.Fatalf("restored project location = %q %v", got, err)
	}
	m, _ := a.marks.Get("claude:test")
	if m.Note != "saved" {
		t.Fatal(m)
	}
	if _, err = s.BackupData(t.TempDir(), `[]`); err == nil {
		t.Fatal("accepted non-object")
	}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	os.WriteFile(bad, []byte("bad"), 0600)
	if _, err = s.RestoreData(bad, `{}`); err == nil {
		t.Fatal("accepted corrupt archive")
	}
}

func TestDataBackupMergeVersionsAndUniqueNames(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	s := a.NewSessionService()
	sess := sessions.Session{Agent: "claude", ID: "backup"}
	save := func(text string) {
		t.Helper()
		if err := a.history.Save(sess, sessions.Transcript{Parts: []sessions.Part{{Kind: "text", Role: "user", Text: text}}}); err != nil {
			t.Fatal(err)
		}
	}
	save("old")
	save("second")
	dir := t.TempDir()
	p, err := s.BackupData(dir, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.BackupData(dir, `{}`)
	if err != nil || p == p2 {
		t.Fatalf("unique %s %s %v", p, p2, err)
	}
	b, _, _, _ := newTestApp(t)
	if _, e := b.NewSessionService().RestoreData(p, `{}`); e != nil {
		t.Fatal(e)
	}
	restored, _ := b.history.Stats()
	if restored.Versions != 2 || restored.Sessions != 1 {
		t.Fatalf("all revisions not restored: %+v", restored)
	}
	save("current")
	if _, err = s.RestoreData(p, `{}`); err != nil {
		t.Fatal(err)
	}
	stats, _ := a.history.Stats()
	_, tr, _, _ := a.history.Get("claude:backup")
	if stats.Versions != 3 || tr.Parts[0].Text != "current" {
		t.Fatalf("%+v %+v", stats, tr)
	}
}
func TestDataBackupRejectsUnsafeArchives(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	s := a.NewSessionService()
	original, err := s.BackupData(t.TempDir(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"traversal", "duplicate", "hash", "version", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			zr, e := zip.OpenReader(original)
			if e != nil {
				t.Fatal(e)
			}
			defer zr.Close()
			dest := filepath.Join(t.TempDir(), "invalid.zip")
			f, _ := os.Create(dest)
			zw := zip.NewWriter(f)
			for _, entry := range zr.File {
				r, _ := entry.Open()
				data, _ := io.ReadAll(r)
				r.Close()
				if kind == "hash" && entry.Name == "preferences.json" {
					data = []byte(`{"changed":true}`)
				}
				if kind == "version" && entry.Name == "manifest.json" {
					data = bytes.Replace(data, []byte(`"formatVersion":1`), []byte(`"formatVersion":2`), 1)
				}
				w, _ := zw.Create(entry.Name)
				w.Write(data)
			}
			switch kind {
			case "traversal":
				w, _ := zw.Create("../escape")
				w.Write([]byte("x"))
			case "duplicate":
				zw.Create("history.db")
			case "symlink":
				h := &zip.FileHeader{Name: "imports/link"}
				h.SetMode(os.ModeSymlink | 0777)
				w, _ := zw.CreateHeader(h)
				w.Write([]byte("target"))
			}
			zw.Close()
			f.Close()
			if _, e = s.RestoreData(dest, `{}`); e == nil {
				t.Fatal("accepted invalid archive")
			}
		})
	}
}

func TestDataBackupRestoresImportsAndRejectsConflicts(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	s := a.NewSessionService()
	source := filepath.Join(t.TempDir(), "conversations.json")
	os.WriteFile(source, []byte(`[{"uuid":"backup-import","name":"备份导入","chat_messages":[{"sender":"human","text":"完整导入内容"}]}]`), 0600)
	if _, err := s.ImportClaudeHistory(source, "账号"); err != nil {
		t.Fatal(err)
	}
	p, err := s.BackupData(t.TempDir(), `{"theme":"light"}`)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _, _ := newTestApp(t)
	bs := b.NewSessionService()
	if _, err = bs.RestoreData(p, `{}`); err != nil {
		t.Fatal(err)
	}
	rows := bs.List(Filter{Agent: "claude-desktop"})
	if len(rows) != 1 {
		t.Fatalf("imports: %+v", rows)
	}
	var imported string
	filepath.WalkDir(b.cfg.ImportDir, func(path string, d os.DirEntry, e error) error {
		if e == nil && !d.IsDir() && filepath.Ext(path) == ".jsonl" {
			imported = path
		}
		return e
	})
	if imported == "" {
		t.Fatal("missing imported transcript")
	}
	os.WriteFile(imported, []byte("conflict"), 0600)
	if _, err = bs.RestoreData(p, `{}`); err == nil {
		t.Fatal("overwrote conflicting import")
	}
	data, _ := os.ReadFile(imported)
	if string(data) != "conflict" {
		t.Fatal("changed conflict file")
	}
}

func TestDataBackupRejectsUnsafeDestinations(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	s := a.NewSessionService()
	for _, dest := range []string{"relative-backup", a.cfg.ImportDir, filepath.Join(a.cfg.ImportDir, "backups")} {
		if _, err := s.BackupData(dest, `{}`); err == nil {
			t.Fatalf("accepted destination %s", dest)
		}
	}
	if _, err := os.Stat(filepath.Join(a.cfg.ImportDir, "backups")); !os.IsNotExist(err) {
		t.Fatalf("created unsafe destination: %v", err)
	}
}
func TestDataBackupValidationDoesNotCreateDatabase(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.db")
	if _, err := history.ValidateBackup(missing); err == nil {
		t.Fatal("accepted missing history")
	}
	if err := marks.ValidateBackup(missing); err == nil {
		t.Fatal("accepted missing marks")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("validation created database: %v", err)
	}
}
func TestDataBackupPartialMergeRefreshesCache(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	sess := sessions.Session{Agent: "claude", ID: "partial"}
	if err := a.history.Save(sess, sessions.Transcript{Parts: []sessions.Part{{Kind: "text", Role: "user", Text: "保留部分恢复正文"}}}); err != nil {
		t.Fatal(err)
	}
	a.marks.SetNote("claude:partial", "触发写入")
	backup, err := a.NewSessionService().BackupData(t.TempDir(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _, _ := newTestApp(t)
	b.setList(nil)
	db, err := sql.Open("sqlite", b.cfg.MarksPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TRIGGER reject_recovery BEFORE INSERT ON marks BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	report, err := b.NewSessionService().RestoreData(backup, `{}`)
	if err == nil || report.RollbackPath == "" {
		t.Fatalf("expected retained backup and partial failure: %+v %v", report, err)
	}
	rows := b.NewSessionService().List(Filter{})
	if len(rows) != 1 || rows[0].ID != "partial" {
		t.Fatalf("stale cache after partial merge: %+v", rows)
	}
	if _, err = os.Stat(report.RollbackPath); err != nil {
		t.Fatal(err)
	}
}
