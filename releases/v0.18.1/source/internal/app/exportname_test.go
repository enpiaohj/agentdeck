package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestExportFileName(t *testing.T) {
	start := time.Date(2026, 10, 8, 23, 30, 0, 0, time.Local)
	last := start.Add(48 * time.Hour)
	for _, c := range []struct {
		name  string
		sess  sessions.Session
		title string
		want  string
	}{
		{"开始日期，不是最后活跃", sessions.Session{ID: "a1b2c3d4-1111", Start: start, Last: last}, "修复登录问题", "2026-10-08-修复登录问题-a1b2c3d4.md"},
		{"没有开始日期用最后活跃", sessions.Session{ID: "a1b2c3d4-1111", Last: last}, "x", "2026-10-10-x-a1b2c3d4.md"},
		{"都没有", sessions.Session{ID: "abc"}, "x", "未知日期-x-abc.md"},
		{"非法字符与空格", sessions.Session{ID: "a1b2c3d4", Start: start}, `Fix: the "a/b" <bug>?  now`, "2026-10-08-Fix_the_a_b_bug_now-a1b2c3d4.md"},
		{"空标题", sessions.Session{ID: "a1b2c3d4", Start: start}, " \t ", "2026-10-08-无标题-a1b2c3d4.md"},
		{"首尾标点", sessions.Session{ID: "a1b2c3d4", Start: start}, "...标题...", "2026-10-08-标题-a1b2c3d4.md"},
	} {
		if got := exportFileName(c.sess, c.title); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	long := exportTitle(strings.Repeat("长", 200))
	if utf8.RuneCountInString(long) != exportTitleMax {
		t.Errorf("标题按字符截断，得到 %d 个字符", utf8.RuneCountInString(long))
	}
}

func exportBody(id string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, "---\n"+sessions.ExportIDLine(id)+"\n---\n\nbody\n")
		return err
	}
}

// 同一会话重复导出得到同一个文件；会话改名后旧文件改名，不另存一份。
func TestWriteExportFileIdempotentAndRename(t *testing.T) {
	dir := t.TempDir()
	id := "a1b2c3d4-0000-1111-2222-333344445555"
	p1, err := writeExportFile(dir, "2026-10-08-旧标题-a1b2c3d4.md", id, exportBody(id))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := writeExportFile(dir, "2026-10-08-旧标题-a1b2c3d4.md", id, exportBody(id))
	if err != nil || p2 != p1 {
		t.Fatalf("重复导出应覆盖同一文件: %q %q %v", p1, p2, err)
	}
	p3, err := writeExportFile(dir, "2026-10-08-新标题-a1b2c3d4.md", id, exportBody(id))
	if err != nil || filepath.Base(p3) != "2026-10-08-新标题-a1b2c3d4.md" {
		t.Fatalf("改名后路径: %q %v", p3, err)
	}
	if _, err := os.Stat(p1); err == nil {
		t.Error("旧文件应当被改名而不是留着")
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatalf("目录里应只有 1 个文件，得到 %d", len(ents))
	}
}

// 目标名被别的文件占用时不覆盖，改用 _2。
func TestWriteExportFileNeverOverwritesForeignFile(t *testing.T) {
	dir := t.TempDir()
	name := "2026-10-08-标题-a1b2c3d4.md"
	foreign := filepath.Join(dir, name)
	if err := os.WriteFile(foreign, []byte("用户自己的文件"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := "a1b2c3d4-0000"
	p, err := writeExportFile(dir, name, id, exportBody(id))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "2026-10-08-标题-a1b2c3d4_2.md" {
		t.Fatalf("路径 %q", p)
	}
	if b, _ := os.ReadFile(foreign); string(b) != "用户自己的文件" {
		t.Fatal("别人的文件被改了")
	}
	// 同短 ID、不同会话的导出也不互相覆盖
	other := "a1b2c3d4-9999"
	p2, err := writeExportFile(dir, name, other, exportBody(other))
	if err != nil || p2 == p || p2 == foreign {
		t.Fatalf("不同会话应各有文件: %q %q %v", p, p2, err)
	}
}

// 写到一半失败：不留临时文件，也不毁掉旧导出。
func TestWriteExportFileFailureKeepsOld(t *testing.T) {
	dir := t.TempDir()
	id := "a1b2c3d4-0000"
	name := "2026-10-08-标题-a1b2c3d4.md"
	p, err := writeExportFile(dir, name, id, exportBody(id))
	if err != nil {
		t.Fatal(err)
	}
	_, err = writeExportFile(dir, name, id, func(w io.Writer) error { return io.ErrClosedPipe })
	if err == nil {
		t.Fatal("应当报错")
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "body") {
		t.Fatal("旧导出被毁")
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatalf("残留了临时文件: %d 个文件", len(ents))
	}
}
