package projects

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// 声明项目：加、查、列、删，以及「未产生会话的目录也能进列表」这个
// Group 给不了的能力。
func TestDeclaredStoreRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	p, err := s.Add(`\\?\D:\Work\NewThing`, "")
	if err != nil {
		t.Fatal(err)
	}
	if p != `D:\Work\NewThing` {
		t.Fatalf("归一化路径 = %q", p)
	}
	// 名字缺省取末级目录。
	all, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Name != "NewThing" {
		t.Fatalf("声明结果 = %+v", all)
	}

	// 同一路径再次声明只更新展示名，不重复出行。
	if _, err := s.Add(`D:\Work\newthing`, "新项目"); err != nil {
		t.Fatal(err)
	}
	all, _ = s.All()
	if len(all) != 1 || all[0].Name != "新项目" {
		t.Fatalf("重复声明应更新而非新增: %+v", all)
	}

	if ok, _ := s.Has(`D:\Work\NewThing`); !ok {
		t.Error("Has 找不到刚声明的项目")
	}
	if err := s.Remove(`D:\Work\NewThing`); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Has(`D:\Work\NewThing`); ok {
		t.Error("删除后仍报已声明")
	}
	// 删未声明过的不算错。
	if err := s.Remove(`D:\ nowhere`); err != nil {
		t.Fatalf("删未声明的路径报错: %v", err)
	}
}

func TestDeclaredMigratesAgentAndPreservesItOnRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "Project")
	if _, err := db.Exec(`INSERT INTO declared_projects(lkey,path,name,added) VALUES(lower(?),?,'旧名','')`, p, p); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	all, err := s.All()
	if err != nil || len(all) != 1 || all[0].Agent != "" {
		t.Fatalf("migration: %+v %v", all, err)
	}
	if _, err := s.AddWithAgent(p, "平台项目", "codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(p, "改名"); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(p, "再次改名"); err != nil {
		t.Fatal(err)
	}
	all, err = s.All()
	if err != nil || all[0].Agent != "codex" || all[0].Name != "再次改名" {
		t.Fatalf("平台丢失: %+v %v", all, err)
	}
	if err := s.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(p, "不能复活"); err == nil {
		t.Fatal("重命名复活已移除的项目")
	}
}

// 相对路径拒绝：项目必须是绝对路径，否则不知道该在哪个目录开新会话。
func TestDeclaredRejectsRelativePath(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, bad := range []string{"", "NewThing", `.\NewThing`} {
		if _, err := s.Add(bad, ""); err == nil {
			t.Errorf("接受了非绝对路径 %q", bad)
		}
	}
}
