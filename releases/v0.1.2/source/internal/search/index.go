package search

// 索引建立在 SQLite FTS5 上。入库的 seg 列是 Segment 的输出以空格
// 相连的形态：unicode61 对这个串收出的每个词，就是分词契约给出的
// 每个词元，索引侧不再有第二种切法。查询侧走同一个 Segment（见
// MatchOf），两侧共用契约，2 字中文查询才命中。
//
// 经由 internal/sqlitedb 打开数据库而非直接 import modernc.org/sqlite：
// modernc 的 libc 在 getenv 遇到无 "=" 的环境变量时会直接 os.Exit(1)。
// sqlitedb 的 init 先把这类条目剔除，是整个程序唯一允许接触 libc 的
// 地方（见 sqlitedb 包注释与 #2327）。

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	_ "github.com/enpiaohj/agentdeck/internal/sqlitedb"
)

// Doc 是一份被索引的正文：Key 标识它来自哪个会话的哪一部分（如
// claude/<uuid>#3），Text 是未分词的原文——原文存于 docs 表，命中
// 后切片段、做高亮都以它为准，不从分词后的串上取（分词串上切出来
// 的片段与渲染的原文对不上，是设计评审指出过的坑）。
type Doc struct {
	Key  string
	Text string
}

// Result 是一次命中：原文与相关度。Rank 是 FTS5 的 bm25 分值，越小
// 越相关。
type Result struct {
	Key  string
	Text string
	Rank float64
}

// Index 是全文索引。它是纯派生物：随时 Reset 整库重建，不携带任何
// 无法从会话原文推出的状态。
type Index struct {
	db *sql.DB
}

// schema 一次性建齐：docs 存原文（rowid 表，其 rowid 即 FTS 行的
// rowid），docs_fts 存分词后的串。
var schema = []string{
	`CREATE TABLE IF NOT EXISTS docs (key TEXT PRIMARY KEY, text TEXT NOT NULL)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS docs_fts USING fts5(seg, tokenize='unicode61')`,
}

// Open 打开（必要时创建）位于 path 的索引库；path 为空时用内存库。
func Open(path string) (*Index, error) {
	if path == "" {
		path = ":memory:"
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Index{db: db}, nil
}

// Close 关闭底层库。
func (ix *Index) Close() error {
	return ix.db.Close()
}

// Reset 清空并重建 schema：索引是纯派生物，分词函数或脱敏规则变更
// 后整库重造，而不是增量修补两套口径。
func (ix *Index) Reset() error {
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS docs_fts`,
		`DROP TABLE IF EXISTS docs`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, stmt := range schema {
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Add 把一批文档写入索引，整个批次在一个事务里，中途失败不留半批。
// 同一 Key 再次写入时替换旧正文，替换语义不含例外：新正文切不出词元
// 时同样走替换——旧行（原文与分词行）一起删掉，不留已失效的旧内容。
// 全新的 Key 切不出词元才直接跳过，它本来就无一词可中。
func (ix *Index) Add(docs []Doc) error {
	if len(docs) == 0 {
		return nil
	}
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	for _, d := range docs {
		toks := Segment(d.Text)
		var rowid int64
		err := tx.QueryRow(`SELECT rowid FROM docs WHERE key = ?`, d.Key).Scan(&rowid)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if len(toks) == 0 {
				continue
			}
			res, err := tx.Exec(`INSERT INTO docs(key, text) VALUES (?, ?)`, d.Key, d.Text)
			if err != nil {
				tx.Rollback()
				return err
			}
			rowid, err = res.LastInsertId()
			if err != nil {
				tx.Rollback()
				return err
			}
			if _, err := tx.Exec(`INSERT INTO docs_fts(rowid, seg) VALUES (?, ?)`, rowid, join(toks)); err != nil {
				tx.Rollback()
				return err
			}
		case err != nil:
			tx.Rollback()
			return err
		default:
			if _, err := tx.Exec(`DELETE FROM docs_fts WHERE rowid = ?`, rowid); err != nil {
				tx.Rollback()
				return err
			}
			if len(toks) == 0 {
				if _, err := tx.Exec(`DELETE FROM docs WHERE rowid = ?`, rowid); err != nil {
					tx.Rollback()
					return err
				}
				continue
			}
			if _, err := tx.Exec(`UPDATE docs SET text = ? WHERE rowid = ?`, d.Text, rowid); err != nil {
				tx.Rollback()
				return err
			}
			if _, err := tx.Exec(`INSERT INTO docs_fts(rowid, seg) VALUES (?, ?)`, rowid, join(toks)); err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit()
}

// MatchOf 把查询变成 FTS5 的 MATCH 表达式：空格分出的每个词各自成
// 一个短语——词内的 bigram 序列保住相邻关系，「远程连接」只中连续
// 出现的「远程连接」，不中「远程…的连接」；词与词之间 AND，与
// sessions.NeedleOf 的「每个词都要在」同一语义。全部词都切不出词元
// 时返回空串。
func MatchOf(query string) string {
	var clauses []string
	for _, w := range strings.Fields(query) {
		toks := Segment(w)
		if len(toks) == 0 {
			continue
		}
		// 词元只含字母、数字与 CJK 字符，双引号内不会出现需要转义的东西
		clauses = append(clauses, `"`+join(toks)+`"`)
	}
	return strings.Join(clauses, " AND ")
}

// Search 全文检索：query 与索引侧共用 Segment（经 MatchOf），命中按
// bm25 相关度升序（越小越相关）返回原文。limit <= 0 表示不设上限。
// 查询无任何有效词元时返回空结果而非错误；ctx 取消时随查询中断。
func (ix *Index) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	expr := MatchOf(query)
	if expr == "" {
		return nil, ctx.Err()
	}
	if limit <= 0 {
		limit = -1
	}
	rows, err := ix.db.QueryContext(ctx,
		`SELECT d.key, d.text, bm25(docs_fts)
		   FROM docs_fts JOIN docs d ON d.rowid = docs_fts.rowid
		  WHERE docs_fts MATCH ?
		  ORDER BY bm25(docs_fts)
		  LIMIT ?`, expr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.Key, &r.Text, &r.Rank); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Count 返回 query 的命中总数，不受 Search 的 limit 影响：调用方按
// limit 取前面的结果时，报数用它才不失真。查询无有效词元时为 0。
func (ix *Index) Count(ctx context.Context, query string) (int, error) {
	expr := MatchOf(query)
	if expr == "" {
		return 0, ctx.Err()
	}
	var n int
	err := ix.db.QueryRowContext(ctx,
		`SELECT count(*) FROM docs_fts WHERE docs_fts MATCH ?`, expr).Scan(&n)
	return n, err
}
