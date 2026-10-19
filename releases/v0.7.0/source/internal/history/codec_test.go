package history

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// bigTranscript 造一段压得动的长正文（JSON 的 Part 列表，和真实存储一致）。
func bigTranscript(parts int) sessions.Transcript {
	out := sessions.Transcript{}
	for i := 0; i < parts; i++ {
		out.Parts = append(out.Parts, sessions.Part{
			Role: "assistant", Kind: "text",
			Text: strings.Repeat("修复远程连接超时的排查记录 ", 40),
		})
	}
	return out
}

// 正文按 zstd 存，读回来逐字段一致；压缩是存储形态的调整，不是内容
// 的改写。
func TestSaveCompressesLargeTranscripts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sess := sessions.Session{Agent: "claude", ID: "big", Title: "长会话"}
	tr := bigTranscript(120)
	if err := s.Save(sess, tr); err != nil {
		t.Fatal(err)
	}
	var codec int
	var stored int
	if err := s.db.QueryRow(`SELECT codec, length(transcript) FROM history_versions`).Scan(&codec, &stored); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	if codec != codecZstd {
		t.Fatalf("codec = %d; want %d（大正文应压缩）", codec, codecZstd)
	}
	if stored >= len(raw) {
		t.Fatalf("压缩后 %d 字节不小于原文 %d 字节", stored, len(raw))
	}
	stats, err := s.Stats()
	if err != nil || stats.Bytes != int64(stored) {
		t.Fatalf("Stats = %+v, %v; want Bytes=%d", stats, err, stored)
	}
	_, got, found, err := s.Get("claude:big")
	if err != nil || !found {
		t.Fatal(err)
	}
	if len(got.Parts) != len(tr.Parts) || got.Parts[7].Text != tr.Parts[7].Text {
		t.Fatal("压缩往返后正文不一致")
	}
}

// 小正文不压：一层间接换不来几个字节。
func TestSaveKeepsSmallTranscriptsPlain(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sess := sessions.Session{Agent: "codex", ID: "small"}
	if err := s.Save(sess, sessions.Transcript{Parts: []sessions.Part{{Role: "user", Kind: "text", Text: "短"}}}); err != nil {
		t.Fatal(err)
	}
	var codec int
	if err := s.db.QueryRow(`SELECT codec FROM history_versions`).Scan(&codec); err != nil {
		t.Fatal(err)
	}
	if codec != codecPlain {
		t.Fatalf("codec = %d; want %d", codec, codecPlain)
	}
}

// 同一个会话、同样的正文与元数据再存一次，不重写行：这是最常见的调用
// 形态（每次读取都会存一遍），每写一次都要在 synchronous=FULL 下等一次
// fsync。
func TestSaveSkipsUnchangedWrites(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sess := sessions.Session{Agent: "claude", ID: "same", Title: "标题"}
	tr := bigTranscript(20)
	if err := s.Save(sess, tr); err != nil {
		t.Fatal(err)
	}
	var first string
	if err := s.db.QueryRow(`SELECT saved_at FROM history_versions`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(sess, tr); err != nil {
		t.Fatal(err)
	}
	var again string
	var versions int
	if err := s.db.QueryRow(`SELECT saved_at FROM history_versions`).Scan(&again); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM history_versions`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if again != first || versions != 1 {
		t.Fatalf("重复保存改动了行: saved_at %q → %q, versions %d", first, again, versions)
	}
}

// 正文没变、会话元数据变了（标题、消息数、时间），列表项要跟上。
func TestSaveRewritesWhenSessionMetadataChanges(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tr := bigTranscript(20)
	sess := sessions.Session{Agent: "claude", ID: "meta", Title: "旧标题", MessageCount: 3}
	if err := s.Save(sess, tr); err != nil {
		t.Fatal(err)
	}
	sess.Title, sess.MessageCount = "新标题", 9
	if err := s.Save(sess, tr); err != nil {
		t.Fatal(err)
	}
	got, _, found, err := s.Get("claude:meta")
	if err != nil || !found {
		t.Fatal(err)
	}
	if got.Title != "新标题" || got.MessageCount != 9 {
		t.Fatalf("元数据未更新: %+v", got)
	}
	var versions int
	if err := s.db.QueryRow(`SELECT count(*) FROM history_versions`).Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("正文没变不该多出版本: %d, %v", versions, err)
	}
}

// 升级前留下的明文行，Compact 压一遍并照样读得回来。
func TestCompactCompressesLegacyRows(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sess := sessions.Session{Agent: "codex", ID: "legacy"}
	tr := bigTranscript(80)
	if err := s.Save(sess, tr); err != nil {
		t.Fatal(err)
	}
	// 退回升级前的形态：明文、codec=0。
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE history_versions SET transcript = ?, codec = 0`, raw); err != nil {
		t.Fatal(err)
	}
	before, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	var steps [][2]int64
	after, err := s.Compact(func(done, total int64) { steps = append(steps, [2]int64{done, total}) })
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) == 0 || steps[len(steps)-1][0] != 1 || steps[len(steps)-1][1] != 1 {
		t.Fatalf("进度 = %v; want 最后一次 1/1", steps)
	}
	if after.Bytes >= before.Bytes {
		t.Fatalf("压缩后 %d 字节不小于压缩前 %d 字节", after.Bytes, before.Bytes)
	}
	var codec int
	if err := s.db.QueryRow(`SELECT codec FROM history_versions`).Scan(&codec); err != nil || codec != codecZstd {
		t.Fatalf("codec = %d, %v; want %d", codec, err, codecZstd)
	}
	_, got, found, err := s.Get("codex:legacy")
	if err != nil || !found || len(got.Parts) != len(tr.Parts) || got.Parts[40].Text != tr.Parts[40].Text {
		t.Fatalf("压缩后正文变了: %d 段, %v", len(got.Parts), err)
	}
	// 再压一次没有可压的行，且不该报错。
	steps = nil
	if _, err := s.Compact(func(done, total int64) { steps = append(steps, [2]int64{done, total}) }); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 0 {
		t.Fatalf("重复压缩仍有进度回调: %v", steps)
	}
}

// Compact 尾部也要先把 WAL 截短、再看 VACUUM：WAL 模式下 VACUUM 是把
// 整理后的整库写进 WAL、由检查点搬回主库的。检查点排在前面，VACUUM 的
// 一整份输出就留在 WAL 文件里，磁盘上等于「压缩后的主库 + 一整份 VACUUM
// 输出」，要等进程干净退出才随 WAL 一起消失。
func TestCompactTightensTheWALItLeaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sess := sessions.Session{Agent: "claude", ID: "long", Title: "长会话"}
	tr := bigTranscript(800)
	if err := s.Save(sess, tr); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	// 退回升级前的形态：明文、codec=0。
	if _, err := s.db.Exec(`UPDATE history_versions SET transcript = ?, codec = 0`, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Compact(nil); err != nil {
		t.Fatal(err)
	}
	// WAL 模式下 VACUUM 只把库「逻辑上」缩下去：新内容先写进 WAL，主库
	// 文件要等检查点才跟着截短。所以判断标准是文件系统上的总量对得上库
	// 自己的页数——VACUUM 的输出留在 WAL 里时，磁盘上就是两份库。
	var pageCount, pageSize, walSize int64
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path + "-wal"); err == nil {
		walSize = fi.Size()
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if onDisk := mainSize(path) + walSize; onDisk > pageCount*pageSize+(64<<10) {
		t.Fatalf("压缩后磁盘占用 %d 字节，而库只有 %d 页（%d 字节）：VACUUM 的输出没被检查点收掉",
			onDisk, pageCount, pageCount*pageSize)
	}
	_, got, found, err := s.Get("claude:long")
	if err != nil || !found || len(got.Parts) != len(tr.Parts) || got.Parts[799].Text != tr.Parts[799].Text {
		t.Fatalf("压缩后正文变了: %d 段, %v", len(got.Parts), err)
	}
}

func mainSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return fi.Size()
}

// 升级前的库没有 codec 列：Open 补一列，旧行按明文读，新行照常压。
func TestOpenAddsCodecColumnToLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	tr := bigTranscript(30)
	body, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := json.Marshal(sessions.Session{Agent: "claude", ID: "old", Title: "旧库"})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE history_versions (key TEXT NOT NULL, hash TEXT NOT NULL, transcript BLOB NOT NULL, saved_at TEXT NOT NULL, PRIMARY KEY(key, hash))`,
		`CREATE TABLE history_sessions (key TEXT PRIMARY KEY, metadata BLOB NOT NULL, hash TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO history_versions(key, hash, transcript, saved_at) VALUES ('claude:old', 'h1', ?, '2026-01-01T00:00:00Z')`, body); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO history_sessions(key, metadata, hash) VALUES ('claude:old', ?, 'h1')`, meta); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var codec int
	if err := s.db.QueryRow(`SELECT codec FROM history_versions WHERE hash = 'h1'`).Scan(&codec); err != nil || codec != codecPlain {
		t.Fatalf("旧行 codec = %d, %v; want %d", codec, err, codecPlain)
	}
	_, got, found, err := s.Get("claude:old")
	if err != nil || !found || len(got.Parts) != len(tr.Parts) {
		t.Fatalf("旧行读不回来: %d 段, %v", len(got.Parts), err)
	}
	// 补列之后新写入的行仍然是压缩的。
	if err := s.Save(sessions.Session{Agent: "claude", ID: "fresh"}, bigTranscript(30)); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT codec FROM history_versions WHERE key = 'claude:fresh'`).Scan(&codec); err != nil || codec != codecZstd {
		t.Fatalf("新行 codec = %d, %v; want %d", codec, err, codecZstd)
	}
}
