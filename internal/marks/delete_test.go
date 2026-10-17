package marks

import (
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
