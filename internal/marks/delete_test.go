package marks

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDeleteKeysRemovesOnlySelectedMarks(t *testing.T) {
	s := open(t)
	if err := s.SetNote("codex:selected", "remove"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote("codex:other", "keep"); err != nil {
		t.Fatal(err)
	}
	other, _ := s.Get("codex:other")
	if err := s.DeleteKeys([]string{"codex:selected", "codex:selected", "codex:missing"}); err != nil {
		t.Fatal(err)
	}
	all, err := s.All()
	if err != nil || len(all) != 1 || !reflect.DeepEqual(other, all["codex:other"]) {
		t.Fatalf("marks=%+v err=%v", all, err)
	}
}

func TestBackupKeysToCopiesOnlySelectedMarksAndUndoRestoresThem(t *testing.T) {
	s := open(t)
	if err := s.SetNote("codex:selected", "original"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFavorite("codex:selected", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTitle("codex:selected", "本地标题"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote("codex:other", "keep"); err != nil {
		t.Fatal(err)
	}
	want, _ := s.Get("codex:selected")
	backup := filepath.Join(t.TempDir(), "backup.db")
	// 只备份被删的会话：另一条标注与不存在的 key 都不该进备份库。
	if err := s.BackupKeysTo(backup, []string{"codex:selected", "codex:missing"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackup(backup); err != nil {
		t.Fatalf("按行备份校验失败: %v", err)
	}
	if err := s.DeleteKeys([]string{"codex:selected"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreKeysFrom(backup, []string{"codex:selected"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("codex:selected")
	if err != nil || !reflect.DeepEqual(want, got) {
		t.Fatalf("按行备份撤销后不一致: restored=%+v want=%+v err=%v", got, want, err)
	}
	if err := s.BackupKeysTo(backup, []string{"codex:other"}); err == nil {
		t.Fatal("已存在的备份文件应被拒绝")
	}
}

func TestRestoreKeysFromRestoresSelectedMarksAndKeepsCurrentData(t *testing.T) {
	s := open(t)
	if err := s.SetNote("codex:selected", "original"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTags("codex:selected", []string{"tag"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFavorite("codex:selected", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArchived("codex:selected", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTitle("codex:selected", "本地新标题"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote("codex:other", "old"); err != nil {
		t.Fatal(err)
	}
	want, _ := s.Get("codex:selected")
	backup := filepath.Join(t.TempDir(), "backup#%.db")
	if err := s.BackupTo(backup); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKeys([]string{"codex:selected"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote("codex:other", "current"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote("codex:new", "new data"); err != nil {
		t.Fatal(err)
	}
	other, _ := s.Get("codex:other")
	if err := s.RestoreKeysFrom(backup, []string{"codex:selected", "codex:selected", "codex:missing"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("codex:selected")
	if err != nil || !reflect.DeepEqual(want, got) {
		t.Fatalf("restored=%+v want=%+v err=%v", got, want, err)
	}
	if err := s.RestoreKeysFrom(backup, []string{"codex:selected"}); err != nil {
		t.Fatalf("identical undo retry: %v", err)
	}
	all, err := s.All()
	if err != nil || len(all) != 3 || !reflect.DeepEqual(other, all["codex:other"]) || all["codex:new"].Note != "new data" {
		t.Fatalf("current marks changed: %+v %v", all, err)
	}
	if err := ValidateBackup(backup); err != nil {
		t.Fatal(err)
	}
}

func TestVersionOneBackupWithTitleKeepsRename(t *testing.T) {
	s := open(t)
	if err := s.SetTitle("claude:one", "备份标题"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "marks.db")
	if err := s.BackupTo(path); err != nil {
		t.Fatal(err)
	}
	backup, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	backup.Close()
	if err := ValidateBackup(path); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTitle("claude:one", "后续标题"); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeBackup(path); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get("claude:one")
	if err != nil || m.Title != "备份标题" {
		t.Fatalf("合并后标题 = %+v, %v", m, err)
	}
	if err := s.DeleteKeys([]string{"claude:one"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreKeysFrom(path, []string{"claude:one"}); err != nil {
		t.Fatal(err)
	}
	m, err = s.Get("claude:one")
	if err != nil || m.Title != "备份标题" {
		t.Fatalf("撤销删除后标题 = %+v, %v", m, err)
	}
}

func TestMergeBackupRestoresTitle(t *testing.T) {
	s := open(t)
	if err := s.SetTitle("claude:one", "备份时的标题"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "marks.db")
	if err := s.BackupTo(path); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTitle("claude:one", "后来改的标题"); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeBackup(path); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get("claude:one")
	if err != nil || m.Title != "备份时的标题" {
		t.Fatalf("恢复后标题 = %+v, %v", m, err)
	}
}

func TestMergeOldBackupClearsTitleAndPreservesOtherMarks(t *testing.T) {
	s := open(t)
	if err := s.SetTitle("claude:old", "现有标题"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE marks (key TEXT PRIMARY KEY, favorite INTEGER NOT NULL DEFAULT 0, archived INTEGER NOT NULL DEFAULT 0, tags TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', updated TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO marks(key,note) VALUES ('claude:old','备份备注')`,
		`PRAGMA user_version = 1`,
	} {
		if _, err := old.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()
	if err := ValidateBackup(path); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeBackup(path); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get("claude:old")
	if err != nil || m.Note != "备份备注" || m.Title != "" {
		t.Fatalf("旧备份合并 = %+v, %v", m, err)
	}
}

func TestRestoreKeysFromMarkConflictIsAtomic(t *testing.T) {
	s := open(t)
	for _, key := range []string{"codex:selected", "codex:conflict"} {
		if err := s.SetNote(key, "original"); err != nil {
			t.Fatal(err)
		}
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err := s.BackupTo(backup); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKeys([]string{"codex:selected", "codex:conflict"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote("codex:conflict", "current"); err != nil {
		t.Fatal(err)
	}
	want, _ := s.Get("codex:conflict")
	if err := s.RestoreKeysFrom(backup, []string{"codex:selected", "codex:conflict"}); err == nil {
		t.Fatal("expected conflict")
	}
	all, err := s.All()
	if err != nil || len(all) != 1 || !reflect.DeepEqual(want, all["codex:conflict"]) {
		t.Fatalf("partial restoration: %+v %v", all, err)
	}
}

func TestMarkDeletionRejectsInvalidKeysAndInvalidBackups(t *testing.T) {
	s := open(t)
	if err := s.SetNote("codex:selected", "original"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKeys([]string{"codex:selected", "../bad"}); err == nil {
		t.Fatal("accepted invalid key")
	}
	all, err := s.All()
	if err != nil || len(all) != 1 {
		t.Fatal("invalid request changed marks")
	}
	if err := s.RestoreKeysFrom(filepath.Join(t.TempDir(), "missing.db"), []string{"../bad"}); err == nil {
		t.Fatal("restore accepted invalid key")
	}
	for _, bad := range []string{"missing.db", "corrupt.db"} {
		path := filepath.Join(t.TempDir(), bad)
		if bad == "corrupt.db" {
			if err := os.WriteFile(path, []byte("not sqlite"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.RestoreKeysFrom(path, []string{"codex:absent"}); err == nil {
			t.Fatal("accepted invalid backup")
		}
		if bad == "missing.db" {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("created missing backup")
			}
		}
	}
}

func TestMarkDeleteKeysRollsBackAllRowsOnFailure(t *testing.T) {
	s := open(t)
	for _, key := range []string{"codex:selected", "codex:blocked"} {
		if err := s.SetNote(key, "original"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`CREATE TRIGGER block_delete BEFORE DELETE ON marks WHEN OLD.key='codex:blocked' BEGIN SELECT RAISE(ABORT,'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKeys([]string{"codex:selected", "codex:blocked"}); err == nil {
		t.Fatal("expected failure")
	}
	all, err := s.All()
	if err != nil || len(all) != 2 {
		t.Fatalf("partial delete: %+v %v", all, err)
	}
}
