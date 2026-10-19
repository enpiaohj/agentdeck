package search

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 旧版索引（docs_fts 自带一份分词原文）打开时就地升级：docs 里的原文
// 照旧，FTS 表换成 contentless 并重新灌一遍。用户不必重读 679 个会话
// 文件，检索结果也不变。
func TestOpenUpgradesContentlessIndexInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "search.db")
	writeLegacyIndex(t, path, []Doc{
		{Key: "claude:one#0", Text: "修复远程连接超时"},
		{Key: "claude:two#0", Text: "数据库索引重建"},
	})

	var steps [][2]int
	ix, err := OpenWithProgress(path, func(done, total int) {
		steps = append(steps, [2]int{done, total})
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()

	var version int
	if err := ix.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("user_version = %d, %v; want %d", version, err, schemaVersion)
	}
	// contentless：不再有那份分词原文的影子表。
	var shadow int
	if err := ix.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'docs_fts_content'`).Scan(&shadow); err != nil || shadow != 0 {
		t.Fatalf("docs_fts_content 仍存在: %d, %v", shadow, err)
	}
	hits(t, ix, "远程连接", []string{"claude:one#0"}, nil)
	hits(t, ix, "索引重建", []string{"claude:two#0"}, nil)
	if len(steps) == 0 || steps[len(steps)-1][0] != 2 || steps[len(steps)-1][1] != 2 {
		t.Fatalf("升级进度 = %v; want 最后一次 2/2", steps)
	}
	// 增量游标原样保留：升级不该让下一趟重建把全部会话重读一遍。
	var stamps int
	if err := ix.db.QueryRow(`SELECT count(*) FROM doc_sources`).Scan(&stamps); err != nil || stamps != 2 {
		t.Fatalf("doc_sources = %d, %v; want 2", stamps, err)
	}
}

// 升级后的表仍要能按 Key 替换、按会话删除——contentless_delete 给的
// 就是这个能力。
func TestContentlessIndexReplacesAndRemoves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "search.db")
	ix, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	if err := ix.Add([]Doc{
		{Key: "agent:one#0", Text: "远程连接超时"},
		{Key: "agent:one#1", Text: "数据库索引"},
		{Key: "agent:only-prefix#0", Text: "前缀相近的另一个会话"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ix.Add([]Doc{{Key: "agent:one#0", Text: "本地缓存"}}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "远程", nil, []string{"agent:one#0"})
	hits(t, ix, "本地缓存", []string{"agent:one#0"}, nil)

	if err := ix.Remove([]string{"agent:one"}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "数据库", nil, []string{"agent:one#1"})
	// 删除的范围只到 "agent:one#"，不能把 "agent:only-prefix" 一起带走。
	hits(t, ix, "前缀相近", []string{"agent:only-prefix#0"}, nil)
}

// SaveStamps 的「索引里还有这个会话吗」同样按 key 区间判断：另一个
// 名字以它开头的会话不能被算进来，否则游标会落在没有 doc 的会话上。
func TestSaveStampsIgnoresSessionsThatOnlyShareThePrefix(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{{Key: "agent:only-prefix#0", Text: "另一个会话"}}); err != nil {
		t.Fatal(err)
	}
	if err := ix.SaveStamps([]SourceStamp{
		{Key: "agent:one", Size: 10, Mtime: 20},
		{Key: "agent:only-prefix", Size: 11, Mtime: 21},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ix.Stamps()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "agent:only-prefix" {
		t.Fatalf("stamps = %+v; want 只有 agent:only-prefix", got)
	}
}

// writeLegacyIndex 造一个 schemaVersion 之前的索引库：docs_fts 是普通
// fts5 表，自带一份分词原文。
func writeLegacyIndex(t *testing.T, path string, docs []Doc) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE docs (key TEXT PRIMARY KEY, text TEXT NOT NULL)`,
		`CREATE VIRTUAL TABLE docs_fts USING fts5(seg, tokenize='unicode61')`,
		`CREATE TABLE doc_sources (key TEXT PRIMARY KEY, size INTEGER NOT NULL, mtime INTEGER NOT NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range docs {
		res, err := db.Exec(`INSERT INTO docs(key, text) VALUES (?, ?)`, d.Key, d.Text)
		if err != nil {
			t.Fatal(err)
		}
		rowid, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO docs_fts(rowid, seg) VALUES (?, ?)`, rowid, join(Segment(d.Text))); err != nil {
			t.Fatal(err)
		}
		session := strings.SplitN(d.Key, "#", 2)[0]
		if _, err := db.Exec(`INSERT INTO doc_sources(key, size, mtime) VALUES (?, 1, 1) ON CONFLICT(key) DO NOTHING`, session); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSearchStillUsesBM25AfterUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "search.db")
	writeLegacyIndex(t, path, []Doc{{Key: "claude:one#0", Text: "远程连接"}})
	ix, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	rs, err := ix.Search(context.Background(), "远程", 10)
	if err != nil || len(rs) != 1 || rs[0].Text != "远程连接" {
		t.Fatalf("升级后检索 = %+v, %v", rs, err)
	}
}

// 升级尾部要把 VACUUM 留在 WAL 里的帧一并收掉。WAL 模式下 VACUUM 是把
// 整理后的整库写进 WAL、再由检查点搬回主库的；检查点排在 VACUUM 之前
// （本机真实库里就是这么写的）等于把那一整份输出留在 WAL 文件里，磁盘
// 空间没省下，要等进程干净退出、WAL 被删掉才看着变小——本机实测就是
// 主库 1.2 GB 旁边挂着一个 1.1 GB 的 index.db-wal。
func TestUpgradeTightensTheWALItLeaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "search.db")
	text := strings.Repeat("远程连接超时与数据库索引重建的排查记录，", 30)
	docs := make([]Doc, 1000)
	for i := range docs {
		docs[i] = Doc{Key: fmt.Sprintf("claude:session-%04d#0", i), Text: text}
	}
	writeLegacyIndex(t, path, docs)

	ix, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()

	var walSize int64
	if fi, err := os.Stat(path + "-wal"); err == nil {
		walSize = fi.Size()
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if walSize > 1<<20 {
		t.Fatalf("升级后 WAL 仍有 %d 字节，VACUUM 的输出没被检查点收掉", walSize)
	}
	rs, err := ix.Search(context.Background(), "排查记录", 5)
	if err != nil || len(rs) != 5 {
		t.Fatalf("升级后检索 = %d 条, %v; want 5 条", len(rs), err)
	}
}
