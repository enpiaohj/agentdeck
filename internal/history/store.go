// Package history owns AgentDeck's permanent conversation library. It is not
// an index/cache: there is deliberately no automatic expiry or pruning. Explicit
// user-requested deletion is handled separately with recovery backups.
package history

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
	_ "github.com/enpiaohj/agentdeck/internal/sqlitedb"
	"github.com/klauspost/compress/zstd"
)

// 正文按 zstd 存。一次会话正文动辄几十 MB（JSON 的 Part 列表），同一条
// 会话每看一眼就是一份新的完整修订，库因此长到几 GB；实测压缩后约为
// 原文的 57%。codec 列区分新旧：0 是明文（升级前的行），1 是 zstd。
const (
	codecPlain  = 0
	codecZstd   = 1
	compressMin = 1 << 12 // 小于 4 KiB 的正文压缩收益抵不过一层间接
)

var (
	zstdOnce sync.Once
	zstdEnc  *zstd.Encoder
	zstdDec  *zstd.Decoder
	zstdErr  error
)

func zstdCodec() (*zstd.Encoder, *zstd.Decoder, error) {
	zstdOnce.Do(func() {
		// Encoder/Decoder 的 EncodeAll/DecodeAll 可并发调用。
		zstdEnc, zstdErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if zstdErr != nil {
			return
		}
		zstdDec, zstdErr = zstd.NewReader(nil)
	})
	return zstdEnc, zstdDec, zstdErr
}

// encodeBody 压缩一段正文，返回内容与应记录的 codec。压不小的照原样存，
// 不为了「统一格式」把小的变大。
func encodeBody(body []byte) ([]byte, int) {
	if len(body) < compressMin {
		return body, codecPlain
	}
	enc, _, err := zstdCodec()
	if err != nil {
		return body, codecPlain
	}
	out := enc.EncodeAll(body, nil)
	if len(out) >= len(body) {
		return body, codecPlain
	}
	return out, codecZstd
}

// decodeBody 把一行正文还原成 JSON。
func decodeBody(body []byte, codec int) ([]byte, error) {
	if codec == codecPlain {
		return body, nil
	}
	if codec != codecZstd {
		return nil, fmt.Errorf("未知的正文编码 %d", codec)
	}
	_, dec, err := zstdCodec()
	if err != nil {
		return nil, err
	}
	return dec.DecodeAll(body, nil)
}

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// rowQuerier 是 *sql.DB 与 *sql.Tx 共有的那一小面。
type rowQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// hasCodecColumn 报告一个库（主库用 "main"，ATTACH 进来的备份用 "recovery"）
// 里 history_versions 有没有 codec 列。删除备份与数据备份都是用户手边放
// 着的文件，升级后读旧备份不能失败。
func hasCodecColumn(q rowQuerier, schema string) (bool, error) {
	var n int
	if err := q.QueryRow(`SELECT count(*) FROM pragma_table_info('history_versions', ?) WHERE name = 'codec'`, schema).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

type Stats struct {
	Sessions int
	Versions int
	Bytes    int64
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`PRAGMA busy_timeout=5000`,
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=FULL`,
		`CREATE TABLE IF NOT EXISTS history_versions (key TEXT NOT NULL, hash TEXT NOT NULL, transcript BLOB NOT NULL, saved_at TEXT NOT NULL, codec INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(key, hash))`,
		`CREATE TABLE IF NOT EXISTS history_sessions (key TEXT PRIMARY KEY, metadata BLOB NOT NULL, hash TEXT NOT NULL)`,
	} {
		if _, err = db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	// 旧库没有 codec 列：补一列默认 0（明文），已有行照旧可读。
	has, err := hasCodecColumn(db, "main")
	if err != nil {
		db.Close()
		return nil, err
	}
	if !has {
		if _, err = db.Exec(`ALTER TABLE history_versions ADD COLUMN codec INTEGER NOT NULL DEFAULT 0`); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Save atomically commits both the content-addressed immutable revision and its
// list entry. Empty reads never replace saved conversations. Identical content
// creates no extra revision; earlier versions are never overwritten or deleted.
func (s *Store) Save(sess sessions.Session, tr sessions.Transcript) error {
	if sess.Agent == "" || sess.ID == "" {
		return fmt.Errorf("会话标识为空")
	}
	if len(tr.Parts) == 0 {
		return nil
	}
	metadata, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	body, err := json.Marshal(tr)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	hash, key := hex.EncodeToString(sum[:]), sess.Agent+":"+sess.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	// 正文与元数据都没变就不写：这是最常见的调用形态（同一会话被反复
	// 读取），而每一次写入都要在 WAL + synchronous=FULL 下等一次 fsync。
	var storedHash string
	var storedMeta []byte
	err = s.db.QueryRow(`SELECT hash, metadata FROM history_sessions WHERE key = ?`, key).Scan(&storedHash, &storedMeta)
	switch {
	case err == nil:
		if storedHash == hash && bytes.Equal(storedMeta, metadata) {
			return nil
		}
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	stored, codec := encodeBody(body)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO history_versions(key,hash,transcript,saved_at,codec) VALUES(?,?,?,?,?) ON CONFLICT(key,hash) DO NOTHING`, key, hash, stored, time.Now().UTC().Format(time.RFC3339Nano), codec); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO history_sessions(key,metadata,hash) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET metadata=excluded.metadata,hash=excluded.hash`, key, metadata, hash); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Get(key string) (sessions.Session, sessions.Transcript, bool, error) {
	var metadata, body []byte
	var codec int
	err := s.db.QueryRow(`SELECT s.metadata,v.transcript,v.codec FROM history_sessions s JOIN history_versions v ON v.key=s.key AND v.hash=s.hash WHERE s.key=?`, key).Scan(&metadata, &body, &codec)
	if errors.Is(err, sql.ErrNoRows) {
		return sessions.Session{}, sessions.Transcript{}, false, nil
	}
	if err != nil {
		return sessions.Session{}, sessions.Transcript{}, false, err
	}
	if body, err = decodeBody(body, codec); err != nil {
		return sessions.Session{}, sessions.Transcript{}, false, err
	}
	var sess sessions.Session
	var tr sessions.Transcript
	if err := json.Unmarshal(metadata, &sess); err != nil {
		return sess, tr, false, err
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return sess, tr, false, err
	}
	return sess, tr, true, nil
}

func (s *Store) List() ([]sessions.Session, error) {
	rows, err := s.db.Query(`SELECT metadata FROM history_sessions ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []sessions.Session{}
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var sess sessions.Session
		if err := json.Unmarshal(metadata, &sess); err != nil {
			return nil, err
		}
		list = append(list, sess)
	}
	return list, rows.Err()
}

// Compact 把升级前留下的明文正文重新压一遍，再 VACUUM 把文件缩回去。
// 每一行写回前先解压比对，逐字节一致才落库——压缩是唯一的改动，正文
// 本身不允许有半点变化。progress 可为 nil，按行报进度。
//
// 锁按批取放：整个 3.5 GB 压一遍要一两分钟，期间不该挡住新正文的保存
// （它们写入的是别的行，而且都是 codec=1）。
func (s *Store) Compact(progress func(done, total int64)) (Stats, error) {
	var total int64
	if err := s.db.QueryRow(`SELECT count(*) FROM history_versions WHERE codec = ? AND length(transcript) >= ?`, codecPlain, compressMin).Scan(&total); err != nil {
		return Stats{}, err
	}
	var done, last int64
	for {
		s.mu.Lock()
		rows, err := s.db.Query(`SELECT rowid, transcript FROM history_versions WHERE codec = ? AND length(transcript) >= ? AND rowid > ? ORDER BY rowid LIMIT 16`, codecPlain, compressMin, last)
		if err != nil {
			s.mu.Unlock()
			return Stats{}, err
		}
		type row struct {
			rowid int64
			body  []byte
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.rowid, &r.body); err != nil {
				rows.Close()
				s.mu.Unlock()
				return Stats{}, err
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			s.mu.Unlock()
			return Stats{}, err
		}
		if len(batch) == 0 {
			s.mu.Unlock()
			break
		}
		for _, r := range batch {
			done++
			last = r.rowid
			stored, codec := encodeBody(r.body)
			if codec == codecPlain {
				continue // 压不小，保持明文
			}
			back, err := decodeBody(stored, codec)
			if err != nil || !bytes.Equal(back, r.body) {
				s.mu.Unlock()
				return Stats{}, fmt.Errorf("压缩校验失败（rowid %d）", r.rowid)
			}
			if _, err := s.db.Exec(`UPDATE history_versions SET transcript = ?, codec = ? WHERE rowid = ?`, stored, codec, r.rowid); err != nil {
				s.mu.Unlock()
				return Stats{}, err
			}
		}
		s.mu.Unlock()
		if progress != nil {
			progress(done, total)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// VACUUM 把这些行腾出来的页真正还给文件系统。它需要一份等大的临时
	// 空间，失败时正文已经压好、库也仍然可用，只是文件暂时偏大——不把
	// 它当成压缩失败。
	//
	// 顺序不能反：WAL 模式下 VACUUM 是把整理后的整库写进 WAL、再由检查点
	// 搬回主库的。检查点排在前面，VACUUM 的一整份输出就留在 WAL 文件里，
	// 磁盘上等于「压缩后的主库 + 一整份 VACUUM 输出」，要等进程干净退出
	// 才随 WAL 一起消失。
	_, _ = s.db.Exec(`VACUUM`)
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return s.statsLocked()
}

func (s *Store) Stats() (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statsLocked()
}

func (s *Store) statsLocked() (Stats, error) {
	var out Stats
	err := s.db.QueryRow(`SELECT (SELECT count(*) FROM history_sessions), count(*), coalesce(sum(length(transcript)),0) FROM history_versions`).Scan(&out.Sessions, &out.Versions, &out.Bytes)
	return out, err
}
