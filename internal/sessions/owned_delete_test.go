package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ownedFixture(t *testing.T) (dir string, files []string, resolve func() ([]string, error)) {
	t.Helper()
	dir = t.TempDir()
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("content of "+name), 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	resolve = func() ([]string, error) {
		m, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		return m, err
	}
	return dir, files, resolve
}

// The same plan as a native deletion: a verified backup first, then removal, and a restore.
func TestOwnedDeleteRoundTrip(t *testing.T) {
	_, files, resolve := ownedFixture(t)
	s := Session{Agent: "chatgpt", ID: "gpt_" + strings.Repeat("a", 64), Imported: true}
	plan, err := PrepareOwnedDelete(s, resolve)
	if err != nil || len(plan.Files) != 2 || len(plan.Remove) != 2 {
		t.Fatalf("%+v %v", plan, err)
	}
	if err := plan.Delete(); err == nil {
		t.Fatal("没有备份不能删")
	}
	backup := filepath.Join(t.TempDir(), "bk")
	if err := plan.Backup(backup); err != nil {
		t.Fatal(err)
	}
	if err := plan.Delete(); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			t.Fatalf("%s 应已删除", f)
		}
	}
	if err := plan.Restore(); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if b, err := os.ReadFile(f); err != nil || !strings.HasPrefix(string(b), "content of") {
			t.Fatalf("%s 应已恢复: %v", f, err)
		}
	}
}

// Anything that changed between preview and delete refuses the delete.
func TestOwnedDeleteRefusesChanges(t *testing.T) {
	dir, files, resolve := ownedFixture(t)
	s := Session{Agent: "chatgpt", ID: "gpt_" + strings.Repeat("b", 64), Imported: true}
	plan, err := PrepareOwnedDelete(s, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Backup(filepath.Join(t.TempDir(), "bk")); err != nil {
		t.Fatal(err)
	}
	// a third copy appeared: the set is no longer the one that was previewed
	extra := filepath.Join(dir, "c.jsonl")
	os.WriteFile(extra, []byte("new"), 0o600)
	if err := plan.Delete(); err == nil {
		t.Fatal("文件范围变了应拒绝")
	}
	os.Remove(extra)
	// one was edited after the backup
	os.WriteFile(files[0], []byte("edited"), 0o600)
	if err := plan.Delete(); err == nil {
		t.Fatal("内容变了应拒绝")
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("拒绝时不能删掉任何文件: %v", err)
		}
	}
}

func TestOwnedDeleteInputs(t *testing.T) {
	dir := t.TempDir()
	good := Session{Agent: "chatgpt", ID: "gpt_" + strings.Repeat("c", 64)}
	if _, err := PrepareOwnedDelete(good, nil); err == nil {
		t.Error("没有解析函数")
	}
	if _, err := PrepareOwnedDelete(Session{ID: "../x"}, func() ([]string, error) { return nil, nil }); err == nil {
		t.Error("非法 ID")
	}
	if _, err := PrepareOwnedDelete(good, func() ([]string, error) { return nil, nil }); err == nil {
		t.Error("没有任何副本应报错，而不是返回一个什么都不删的计划")
	}
	if _, err := PrepareOwnedDelete(good, func() ([]string, error) { return []string{dir}, nil }); err == nil {
		t.Error("目录不能当文件删")
	}
	if _, err := PrepareOwnedDelete(good, func() ([]string, error) { return []string{"relative.jsonl"}, nil }); err == nil {
		t.Error("相对路径应拒绝")
	}
}
