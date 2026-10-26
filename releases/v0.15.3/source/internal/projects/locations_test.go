package projects

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocationsExactAndBackup(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	original := filepath.Join(root, "old")
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.SetLocation(original, target); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Location(strings.ToUpper(original)); err != nil || got != Normalize(target) {
		t.Fatalf("location = %q, %v", got, err)
	}
	child := filepath.Join(original, "child")
	if got, _ := store.Location(child); got != Normalize(child) {
		t.Fatalf("must not map child: %q", got)
	}
	if err := store.SetLocation(target, root); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Location(original); got != Normalize(target) {
		t.Fatalf("must not follow chains: %q", got)
	}
	backup := filepath.Join(root, "backup.db")
	if err := store.BackupTo(backup); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackup(backup); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if got, _ := restored.Location(original); got != Normalize(target) {
		t.Fatalf("restored = %q", got)
	}
	if err := store.RemoveLocation(original); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Location(original); got != Normalize(original) {
		t.Fatalf("removed = %q", got)
	}
	if err := store.SetLocation(original, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing target accepted")
	}
	if err := store.SetLocation("relative", target); err == nil {
		t.Fatal("relative original accepted")
	}
	// 不规范的路径不可能有关联，查询要沿用原值而不是报错：会话的 cwd 由上游
	// 写入，可能为空或不是绝对路径，这条路上不该因此丢掉恢复能力。
	for _, path := range []string{"relative", "", "  "} {
		if got, err := store.Location(path); err != nil || got != path {
			t.Fatalf("non-absolute %q = %q, %v", path, got, err)
		}
	}
}

func TestLocationBackupSchemaCompatibility(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "invalid"}[malformed], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "projects.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(schema + `; ALTER TABLE declared_projects ADD COLUMN agent TEXT NOT NULL DEFAULT ''`)
			if err != nil {
				t.Fatal(err)
			}
			if malformed {
				if _, err := db.Exec(`CREATE TABLE project_locations (lkey TEXT, original TEXT, target TEXT)`); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			if err := ValidateBackup(path); (err != nil) != malformed {
				t.Fatalf("validation = %v", err)
			}
			if !malformed {
				s, err := Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				if rows, err := s.Locations(); err != nil || len(rows) != 0 {
					t.Fatalf("migrated = %v %v", rows, err)
				}
			}
		})
	}
}
