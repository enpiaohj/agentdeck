package search

import (
	"database/sql"
	"net/url"
	"strconv"
	"strings"
)

// 索引库的打开与升级。索引是纯派生物，但重建的代价分两段：读上游会话
// （要解析 679 个文件）与给正文分词（本机整库约 13 秒）。升级只做后者
// ——docs 表里已有原文，docs_fts 从它重新灌一遍即可，不必重新解析会话。

// schemaVersion 记在 PRAGMA user_version 上。分词契约、FTS 表形态或
// docs 结构变更时加一：Open 据此把旧库就地升级，而不是留着两套口径。
//
//	1（无标记）docs_fts 自带一份分词原文（content 默认值）
//	2          docs_fts 是 contentless，不再另存一份分词原文
const schemaVersion = 2

// openDSN 给每个连接都带上前导 pragma。cache_size、synchronous 是
// 连接级设置，只在 sql.DB 上 Exec 一次只对当时那条连接生效，池里新开
// 的连接会退回默认值，所以走 DSN。
func openDSN(path string) (dsn string, memory bool) {
	if path == "" || path == ":memory:" {
		return ":memory:", true
	}
	p := strings.ReplaceAll(path, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		p = "/" + p
	}
	q := url.Values{}
	for _, pragma := range []string{
		"busy_timeout(5000)",
		// 索引可随时重建，WAL + NORMAL 够安全，也比 delete 日志快得多。
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"cache_size(-65536)", // 64 MiB 页缓存，默认只有 2 MiB
		"temp_store(MEMORY)",
		"mmap_size(268435456)",
	} {
		q.Add("_pragma", pragma)
	}
	return (&url.URL{Scheme: "file", Path: p, RawQuery: q.Encode()}).String(), false
}

// Open 打开（必要时创建）位于 path 的索引库；path 为空时用内存库。
func Open(path string) (*Index, error) {
	return OpenWithProgress(path, nil)
}

// OpenWithProgress 同 Open，另外在旧库升级时把进度（已灌入的段数 / 总
// 段数）报给调用方；progress 可为 nil。
func OpenWithProgress(path string, progress func(done, total int)) (*Index, error) {
	dsn, memory := openDSN(path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if memory {
		// 内存库的每条连接都是独立的一份库；连接池一旦开第二条，
		// 写入落在一条连接、查询读到另一条就是空库。
		db.SetMaxOpenConns(1)
	}
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	ix := &Index{db: db}
	if err := ix.upgrade(progress, memory); err != nil {
		db.Close()
		return nil, err
	}
	return ix, nil
}

// upgrade 把旧版索引就地升到 schemaVersion。只有 docs_fts 会被重建，
// docs（原文）与 doc_sources（增量游标）原样保留；中途失败时
// user_version 仍是旧值，下次打开重做一遍——重建从 docs 出发，幂等。
func (ix *Index) upgrade(progress func(done, total int), memory bool) error {
	var version int
	if err := ix.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version >= schemaVersion {
		return nil
	}
	if _, err := ix.db.Exec(`DROP TABLE IF EXISTS docs_fts`); err != nil {
		return err
	}
	if _, err := ix.db.Exec(`CREATE VIRTUAL TABLE docs_fts USING fts5(seg, tokenize='unicode61', content='', contentless_delete=1)`); err != nil {
		return err
	}
	var total int
	if err := ix.db.QueryRow(`SELECT count(*) FROM docs`).Scan(&total); err != nil {
		return err
	}
	done, last := 0, int64(0)
	for {
		batch, err := ix.docsAfter(last, 2000)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		tx, err := ix.db.Begin()
		if err != nil {
			return err
		}
		for _, d := range batch {
			if _, err := tx.Exec(`INSERT INTO docs_fts(rowid, seg) VALUES (?, ?)`, d.rowid, d.seg); err != nil {
				tx.Rollback()
				return err
			}
			last = d.rowid
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		done += len(batch)
		if progress != nil {
			progress(done, total)
		}
	}
	if _, err := ix.db.Exec(`PRAGMA user_version = ` + strconv.Itoa(schemaVersion)); err != nil {
		return err
	}
	if memory {
		return nil
	}
	// 丢掉的那张表还占着页；VACUUM 把文件真正缩回去，这才是升级要的
	// 磁盘收益。标记先落盘，收缩失败也不必重做一遍填表；VACUUM 失败
	// （临时空间不够等）不影响索引正确性，只是文件暂时偏大，下一次
	// 整库重建会顺带收掉——不因为它把可用的索引报成失败。
	//
	// 顺序不能反：WAL 模式下 VACUUM 是把整理后的整库写进 WAL 文件再
	// 检查点回主库的，1.2 GB 的索引对应 1.1 GB 的 WAL。先检查点再
	// VACUUM，等于把 VACUUM 的输出全留在 WAL 里等着——空间没省下，
	// 要等进程干净退出、WAL 被删掉才看着变小。检查点排在 VACUUM 之后，
	// 两处（重灌 FTS 与 VACUUM）写出的帧一起收掉。
	_, _ = ix.db.Exec(`VACUUM`)
	_, _ = ix.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return nil
}

type pendingDoc struct {
	rowid int64
	seg   string
}

// docsAfter 取 rowid > last 的一批原文并当场分词。分词在事务外做：
// 事务只用来插入，写入锁不被分词占着。
func (ix *Index) docsAfter(last int64, limit int) ([]pendingDoc, error) {
	rows, err := ix.db.Query(`SELECT rowid, text FROM docs WHERE rowid > ? ORDER BY rowid LIMIT ?`, last, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingDoc
	for rows.Next() {
		var rowid int64
		var text string
		if err := rows.Scan(&rowid, &text); err != nil {
			return nil, err
		}
		if toks := Segment(text); len(toks) > 0 {
			out = append(out, pendingDoc{rowid: rowid, seg: join(toks)})
		}
	}
	return out, rows.Err()
}
