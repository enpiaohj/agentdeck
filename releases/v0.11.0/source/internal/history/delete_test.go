package history

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func deletionStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func deletionSave(t *testing.T, s *Store, id, text string) {
	t.Helper()
	if err := s.Save(sessions.Session{Agent: "codex", ID: id, Title: text}, sessions.Transcript{Parts: []sessions.Part{{Role: "user", Kind: "text", Text: text}}}); err != nil {
		t.Fatal(err)
	}
}

func deletionRows(t *testing.T, s *Store, key string) [][]string {
	t.Helper()
	rows, err := s.db.Query(`SELECT s.key, s.metadata, s.hash, v.hash, v.transcript, v.saved_at FROM history_sessions s JOIN history_versions v ON s.key=v.key WHERE s.key=? ORDER BY v.hash`, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		r := make([]string, 6)
		if err := rows.Scan(&r[0], &r[1], &r[2], &r[3], &r[4], &r[5]); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDeleteKeysRemovesOnlySelectedHistoryAndAllVersions(t *testing.T) {
	s := deletionStore(t)
	deletionSave(t, s, "selected", "first")
	deletionSave(t, s, "selected", "second")
	deletionSave(t, s, "other", "keep")
	other := deletionRows(t, s, "codex:other")
	if err := s.DeleteKeys([]string{"codex:selected", "codex:selected", "codex:missing"}); err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats()
	if err != nil || stats.Sessions != 1 || stats.Versions != 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	if !reflect.DeepEqual(other, deletionRows(t, s, "codex:other")) {
		t.Fatal("unrelated history changed")
	}
}

func TestBackupKeysToCopiesOnlySelectedSessionsAndUndoRestoresThem(t *testing.T) {
	s := deletionStore(t)
	deletionSave(t, s, "selected", "first")
	deletionSave(t, s, "selected", "second")
	deletionSave(t, s, "other", "keep")
	want := deletionRows(t, s, "codex:selected")
	backup := filepath.Join(t.TempDir(), "backup.db")
	// 只备份被删的会话：另一条会话与不存在的 key 都不该进备份库。
	if err := s.BackupKeysTo(backup, []string{"codex:selected", "codex:missing"}); err != nil {
		t.Fatal(err)
	}
	stats, err := ValidateBackup(backup)
	if err != nil || stats.Sessions != 1 || stats.Versions != 2 {
		t.Fatalf("备份应只含被删会话的两条版本: %+v %v", stats, err)
	}
	if err := s.DeleteKeys([]string{"codex:selected"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreKeysFrom(backup, []string{"codex:selected"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, deletionRows(t, s, "codex:selected")) {
		t.Fatal("按行备份撤销后内容不一致")
	}
	// 目标已存在时拒绝覆盖：删除备份目录里不该出现半份或重复的备份。
	if err := s.BackupKeysTo(backup, []string{"codex:other"}); err == nil {
		t.Fatal("已存在的备份文件应被拒绝")
	}
}

func TestRestoreKeysFromRestoresExactVersionsWithoutRevertingOtherHistory(t *testing.T) {
	s := deletionStore(t)
	deletionSave(t, s, "selected", "first")
	deletionSave(t, s, "selected", "second")
	deletionSave(t, s, "other", "old")
	want := deletionRows(t, s, "codex:selected")
	backup := filepath.Join(t.TempDir(), "backup#%.db")
	if err := s.BackupTo(backup); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKeys([]string{"codex:selected"}); err != nil {
		t.Fatal(err)
	}
	deletionSave(t, s, "other", "current")
	deletionSave(t, s, "new", "new data")
	other := deletionRows(t, s, "codex:other")
	if err := s.RestoreKeysFrom(backup, []string{"codex:selected", "codex:selected", "codex:missing"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, deletionRows(t, s, "codex:selected")) {
		t.Fatal("restored metadata, versions, hashes or timestamps differ")
	}
	if err := s.RestoreKeysFrom(backup, []string{"codex:selected"}); err != nil {
		t.Fatalf("identical undo retry: %v", err)
	}
	if !reflect.DeepEqual(other, deletionRows(t, s, "codex:other")) || len(deletionRows(t, s, "codex:new")) != 1 {
		t.Fatal("unrelated current history changed")
	}
	stats, err := ValidateBackup(backup)
	if err != nil || stats.Versions != 3 {
		t.Fatalf("backup changed: %+v %v", stats, err)
	}
}

func TestRestoreKeysFromHistoryConflictIsAtomic(t *testing.T) {
	s := deletionStore(t)
	deletionSave(t, s, "selected", "original")
	deletionSave(t, s, "conflict", "old")
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err := s.BackupTo(backup); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKeys([]string{"codex:selected", "codex:conflict"}); err != nil {
		t.Fatal(err)
	}
	deletionSave(t, s, "conflict", "current")
	want := deletionRows(t, s, "codex:conflict")
	if err := s.RestoreKeysFrom(backup, []string{"codex:selected", "codex:conflict"}); err == nil {
		t.Fatal("expected conflict")
	}
	if len(deletionRows(t, s, "codex:selected")) != 0 || !reflect.DeepEqual(want, deletionRows(t, s, "codex:conflict")) {
		t.Fatal("conflict partially restored or overwrote history")
	}
}

func TestHistoryDeletionRejectsInvalidKeysAndInvalidBackups(t *testing.T) {
	s := deletionStore(t)
	deletionSave(t, s, "selected", "original")
	if err := s.DeleteKeys([]string{"codex:selected", "../bad"}); err == nil {
		t.Fatal("accepted invalid key")
	}
	if len(deletionRows(t, s, "codex:selected")) != 1 {
		t.Fatal("invalid request deleted history")
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

func TestHistoryDeleteKeysRollsBackAllRowsOnFailure(t *testing.T) {
	s := deletionStore(t)
	deletionSave(t, s, "selected", "first")
	deletionSave(t, s, "blocked", "second")
	if _, err := s.db.Exec(`CREATE TRIGGER block_delete BEFORE DELETE ON history_sessions WHEN OLD.key='codex:blocked' BEGIN SELECT RAISE(ABORT,'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKeys([]string{"codex:selected", "codex:blocked"}); err == nil {
		t.Fatal("expected failure")
	}
	stats, err := s.Stats()
	if err != nil || stats.Sessions != 2 || stats.Versions != 2 {
		t.Fatalf("partial delete: %+v %v", stats, err)
	}
}
