package marks

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// open 在测试自己的临时目录里开一个标注库：path 显式传入，不碰
// os.TempDir（陷阱 #8 的隔离靠 t.TempDir 天然成立）。
func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "marks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// 各 setter 落库后重开库仍在：标注是用户数据，随进程生死而失就不叫持久。
func TestSettersPersistAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marks.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetFavorite("claude:abc123", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArchived("claude:abc123", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTags("claude:abc123", []string{"重构", "配额"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote("claude:abc123", "3 号窗口的定价逻辑在这"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTitle("claude:abc123", "定价问题排查"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m, err := s.Get("claude:abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Favorite || !m.Archived || m.Note != "3 号窗口的定价逻辑在这" || m.Title != "定价问题排查" {
		t.Fatalf("标注没回来: %+v", m)
	}
	if len(m.Tags) != 2 || m.Tags[0] != "重构" || m.Tags[1] != "配额" {
		t.Fatalf("tags = %v", m.Tags)
	}
	if m.Updated.IsZero() {
		t.Error("updated 应有写入时间")
	}
}

// 没标过的会话返回零值而非错误，列表合成不必逐个判存在。
func TestGetAbsentIsZeroMark(t *testing.T) {
	s := open(t)
	m, err := s.Get("codex:nope")
	if err != nil {
		t.Fatal(err)
	}
	if m.Key != "codex:nope" || m.Favorite || m.Archived || m.Tags != nil || m.Note != "" {
		t.Fatalf("absent = %+v", m)
	}
}

// All 与逐个 Get 一致；TagsInUse 去重给全。
func TestAllAndTagsInUse(t *testing.T) {
	s := open(t)
	for _, c := range []struct {
		key  string
		fav  bool
		tags []string
	}{
		{"claude:a1", true, []string{"配额", "重构"}},
		{"codex:b2", false, []string{"重构"}},
		{"zcode:c3", false, []string{"远程"}},
	} {
		if err := s.SetFavorite(c.key, c.fav); err != nil {
			t.Fatal(err)
		}
		if err := s.SetTags(c.key, c.tags); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("All = %d 条, want 3", len(all))
	}
	if !all["claude:a1"].Favorite || all["codex:b2"].Favorite {
		t.Fatalf("favorite 合成错: %+v", all)
	}
	tags, err := s.TagsInUse()
	if err != nil {
		t.Fatal(err)
	}
	// 去重后 3 个，原顺序
	if len(tags) != 3 || tags[0] != "配额" || tags[1] != "重构" || tags[2] != "远程" {
		t.Fatalf("TagsInUse = %v", tags)
	}
}

// key 是上游来的不可信输入：不合白名单的一律拒绝。
func TestKeyValidation(t *testing.T) {
	s := open(t)
	for _, bad := range []string{"", "noColon", "Claude:abc", "claude:", ":abc",
		"../evil:x", "claude:a b", "claude:a:b", "claude:中文", "a b:x"} {
		if err := s.SetFavorite(bad, true); err == nil {
			t.Errorf("key %q 被接受了", bad)
		}
	}
	// 合法形态
	for _, ok := range []string{"claude:abc123", "claude-desktop:x_1-y", "qoder-cn:0190aaaa-1111-7222-8333-444455556666"} {
		if err := s.SetFavorite(ok, true); err != nil {
			t.Errorf("key %q 被拒绝: %v", ok, err)
		}
	}
}

// 标签规范化：去空去重去首尾空白；超限报错而非静默截断。
func TestSetTagsLimits(t *testing.T) {
	s := open(t)
	if err := s.SetTags("claude:a1", []string{" 配额 ", "", "配额", "重构"}); err != nil {
		t.Fatal(err)
	}
	m, _ := s.Get("claude:a1")
	if len(m.Tags) != 2 || m.Tags[0] != "配额" || m.Tags[1] != "重构" {
		t.Fatalf("tags = %v", m.Tags)
	}
	many := make([]string, maxTags+1)
	for i := range many {
		many[i] = "标签" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	if err := s.SetTags("claude:a1", many); err == nil {
		t.Errorf("%d 个标签被接受了", len(many))
	}
	long := make([]rune, maxTagLen+1)
	for i := range long {
		long[i] = '长'
	}
	if err := s.SetTags("claude:a1", []string{string(long)}); err == nil {
		t.Error("超长标签被接受了")
	}
}

// service 层来自 Wails 的多个 goroutine，写必须经 mu 串行不错乱。
func TestConcurrentWrites(t *testing.T) {
	s := open(t)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "claude:key" + string(rune('0'+i))
			if err := s.SetFavorite(key, true); err != nil {
				t.Errorf("SetFavorite: %v", err)
			}
			if err := s.SetNote(key, "n"); err != nil {
				t.Errorf("SetNote: %v", err)
			}
		}(i)
	}
	wg.Wait()
	all, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 10 {
		t.Fatalf("并发写后 %d 条, want 10", len(all))
	}
	for k, m := range all {
		if !m.Favorite || m.Note != "n" {
			t.Fatalf("%s 交错写坏了: %+v", k, m)
		}
	}
}

func TestTitleValidationAndReset(t *testing.T) {
	s := open(t)
	key := "claude:title"
	for _, bad := range []string{strings.Repeat("字", maxTitleRun+1), "两行\n标题", "含\x00控制符", "分隔 标题"} {
		if err := s.SetTitle(key, bad); err == nil {
			t.Errorf("非法标题 %q 被接受", bad)
		}
	}
	if err := s.SetTitle(key, "  新标题  "); err != nil {
		t.Fatal(err)
	}
	m, _ := s.Get(key)
	if m.Title != "新标题" {
		t.Fatalf("标题 = %q", m.Title)
	}
	if err := s.SetTitle(key, "  "); err != nil {
		t.Fatal(err)
	}
	m, _ = s.Get(key)
	if m.Title != "" {
		t.Fatalf("还原后标题 = %q", m.Title)
	}
}

func TestMigrateVersionOneWithTitleColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marks.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetTitle("claude:old", "已保存标题"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m, err := s.Get("claude:old")
	if err != nil || m.Title != "已保存标题" {
		t.Fatalf("重新打开后标注 = %+v, %v", m, err)
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("重新升级版本 = %d, %v", version, err)
	}
}

func TestMigrateOldMarksAndRejectFutureSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marks.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE marks (key TEXT PRIMARY KEY, favorite INTEGER NOT NULL DEFAULT 0, archived INTEGER NOT NULL DEFAULT 0, tags TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', updated TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO marks(key,note) VALUES ('claude:old','旧备注')`,
		`PRAGMA user_version = 1`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Get("claude:old")
	if err != nil || m.Note != "旧备注" || m.Title != "" {
		t.Fatalf("迁移后标注 = %+v, %v", m, err)
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("迁移版本 = %d, %v", version, err)
	}
	s.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("未来版本被降级打开")
	}
}

// 备注超限报错。
func TestNoteLimit(t *testing.T) {
	s := open(t)
	long := make([]rune, maxNoteRun+1)
	for i := range long {
		long[i] = '字'
	}
	if err := s.SetNote("claude:a1", string(long)); err == nil {
		t.Error("超长备注被接受了")
	}
	if err := s.SetNote("claude:a1", "正常备注"); err != nil {
		t.Fatalf("正常备注被拒: %v", err)
	}
}
