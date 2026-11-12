package marks

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func deletionKeys(keys []string) ([]string, error) {
	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !keyRe.MatchString(key) {
			return nil, fmt.Errorf("非法的会话 key: %q", key)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return out, nil
}

// DeleteKeys atomically deletes only explicitly selected annotations. The caller
// must first create a recovery backup for manual undo.
func (s *Store) DeleteKeys(keys []string) error {
	keys, err := deletionKeys(keys)
	if err != nil || len(keys) == 0 {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, key := range keys {
		if _, err := tx.Exec(`DELETE FROM marks WHERE key=?`, key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BackupKeysTo 只把给定会话的标注写进一个新的恢复备份库。
//
// 删除会话时以前整库 VACUUM INTO + 全量校验，而标注库通常只有几十 KB、
// 全量校验却要逐行解 JSON；这里只复制被删的那几行。结构与 user_version
// 与主库一致，恢复路径（RestoreKeysFrom）与校验（ValidateBackup）不用改。
//
// 目标文件必须不存在；先写临时文件再改名，中途失败不会留下半份备份。
func (s *Store) BackupKeysTo(dest string, keys []string) error {
	keys, err := deletionKeys(keys)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return errors.New("没有要备份的会话")
	}
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("恢复备份已存在：%s", dest)
	} else if !os.IsNotExist(err) {
		return err
	}
	tmp := dest + ".partial"
	_ = os.Remove(tmp)
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	backup, err := sql.Open("sqlite", tmp)
	if err != nil {
		return err
	}
	defer func() {
		backup.Close()
		_ = os.Remove(tmp)
	}()
	backup.SetMaxOpenConns(1)
	// 与 Open 同一套表结构：恢复时按 user_version 与 title 列判定新旧库。
	for _, stmt := range append(append([]string{`PRAGMA journal_mode=DELETE`}, schema[:]...), `PRAGMA user_version = 2`) {
		if _, err = backup.Exec(stmt); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := backup.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 从主库读、往备份库写；备份库刚按当前 schema 建好，title 列一定在。
	// 没有标注的会话不写行：恢复时「备份里没有」就等于「原本没有」。
	for _, key := range keys {
		record, err := readDeletionRow(s.db, key, false)
		if err != nil {
			return err
		}
		if !record.present {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO marks(key,favorite,archived,tags,note,title,updated) VALUES(?,?,?,?,?,?,?)`, key, record.favorite, record.archived, record.tags, record.note, record.title, record.updated); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = backup.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

type deletionRecord struct {
	present                    bool
	favorite, archived         int
	tags, note, title, updated string
}

func readDeletionRecord(tx *sql.Tx, key string, oldSchema bool) (deletionRecord, error) {
	return readDeletionRow(tx, key, oldSchema)
}

// readDeletionRow 与 readDeletionRecord 同义，只是接受 *sql.DB：备份要从
// 主库连接读，不能借用备份库的事务。
func readDeletionRow(q interface {
	QueryRow(string, ...any) *sql.Row
}, key string, oldSchema bool) (deletionRecord, error) {
	var record deletionRecord
	var err error
	if oldSchema {
		err = q.QueryRow(`SELECT favorite,archived,tags,note,updated FROM marks WHERE key=?`, key).
			Scan(&record.favorite, &record.archived, &record.tags, &record.note, &record.updated)
	} else {
		err = q.QueryRow(`SELECT favorite,archived,tags,note,title,updated FROM marks WHERE key=?`, key).
			Scan(&record.favorite, &record.archived, &record.tags, &record.note, &record.title, &record.updated)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return record, nil
	}
	record.present = err == nil
	return record, err
}

// RestoreKeysFrom validates and reads a caller-managed, trusted recovery backup
// read-only, then restores only selected absent annotations in one transaction.
// Existing identical rows allow undo retries; differing rows reject the whole
// batch. Keys absent from the backup remain absent; unrelated rows stay intact.
func (s *Store) RestoreKeysFrom(path string, keys []string) error {
	keys, err := deletionKeys(keys)
	if err != nil || len(keys) == 0 {
		return err
	}
	if err := ValidateBackup(path); err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	uri := "file:" + strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(absolute)) + "?mode=ro"
	backup, err := sql.Open("sqlite", uri)
	if err != nil {
		return err
	}
	defer backup.Close()
	source, err := backup.Begin()
	if err != nil {
		return err
	}
	defer source.Rollback()
	hasTitle, err := backupHasTitle(source, "main")
	if err != nil {
		return err
	}
	records := make(map[string]deletionRecord, len(keys))
	for _, key := range keys {
		record, err := readDeletionRecord(source, key, !hasTitle)
		if err != nil {
			return err
		}
		records[key] = record
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, key := range keys {
		current, err := readDeletionRecord(tx, key, false)
		if err != nil {
			return err
		}
		record := records[key]
		if current.present {
			if current != record {
				return fmt.Errorf("恢复冲突，标注已有不同内容: %s", key)
			}
			continue
		}
		if record.present {
			if _, err := tx.Exec(`INSERT INTO marks(key,favorite,archived,tags,note,title,updated) VALUES(?,?,?,?,?,?,?)`, key, record.favorite, record.archived, record.tags, record.note, record.title, record.updated); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
