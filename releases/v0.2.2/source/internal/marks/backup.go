package marks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ValidateBackup(path string) error {

	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("备份数据库无法读取：%w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("备份数据库必须是普通文件")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	uri := "file:" + strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(absolute)) + "?mode=ro"
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return err
	}
	defer db.Close()
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 1 {
		return fmt.Errorf("不支持的标注库格式版本")
	}
	var objects int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type IN ('view','trigger') OR (type='table' AND name!='marks')`).Scan(&objects); err != nil || objects != 0 {
		return fmt.Errorf("标注库表结构无效")
	}
	var check string
	if err = db.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil || check != "ok" {
		return fmt.Errorf("标注库完整性校验失败：%s %v", check, err)
	}
	rows, err := db.Query(`SELECT key,favorite,archived,tags,note,updated FROM marks`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, tags, note, updated string
		var f, a int
		if err = rows.Scan(&key, &f, &a, &tags, &note, &updated); err != nil {
			return err
		}
		var ts []string
		if !keyRe.MatchString(key) || json.Unmarshal([]byte(tags), &ts) != nil || (f != 0 && f != 1) || (a != 0 && a != 1) {
			return fmt.Errorf("标注库内容校验失败")
		}
	}
	return rows.Err()
}
func (s *Store) MergeBackup(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), `ATTACH DATABASE ? AS recovery`, path); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `DETACH DATABASE recovery`)
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO marks(key,favorite,archived,tags,note,updated) SELECT key,favorite,archived,tags,note,updated FROM recovery.marks WHERE true ON CONFLICT(key) DO UPDATE SET favorite=excluded.favorite,archived=excluded.archived,tags=excluded.tags,note=excluded.note,updated=excluded.updated`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
