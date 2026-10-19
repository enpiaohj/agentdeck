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

// backupHasTitle checks the actual column layout: older binaries may reset
// user_version to 1 after a newer binary has already added title.
func backupHasTitle(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, schema string) (bool, error) {
	rows, err := q.Query(`PRAGMA ` + schema + `.table_info(marks)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == "title" {
			return true, nil
		}
	}
	return false, rows.Err()
}

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
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || (version != 1 && version != 2) {
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
	hasTitle, err := backupHasTitle(db, "main")
	if err != nil {
		return err
	}
	if version == 2 && !hasTitle {
		return fmt.Errorf("标注库缺少 title 列")
	}
	columns := `key,favorite,archived,tags,note,updated`
	if hasTitle {
		columns = `key,favorite,archived,tags,note,title,updated`
	}
	rows, err := db.Query(`SELECT ` + columns + ` FROM marks`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, tags, note, title, updated string
		var f, a int
		if hasTitle {
			err = rows.Scan(&key, &f, &a, &tags, &note, &title, &updated)
		} else {
			err = rows.Scan(&key, &f, &a, &tags, &note, &updated)
		}
		if err != nil {
			return err
		}
		if len([]rune(title)) > maxTitleRun {
			return fmt.Errorf("标注库标题内容校验失败")
		}
		for _, r := range title {
			if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 {
				return fmt.Errorf("标注库标题内容校验失败")
			}
		}
		var ts []string
		if !keyRe.MatchString(key) || json.Unmarshal([]byte(tags), &ts) != nil || (f != 0 && f != 1) || (a != 0 && a != 1) {
			return fmt.Errorf("标注库内容校验失败")
		}
	}
	return rows.Err()
}
func (s *Store) MergeBackup(path string) error {
	if err := ValidateBackup(path); err != nil {
		return err
	}
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
	var version int
	if err := tx.QueryRow(`PRAGMA recovery.user_version`).Scan(&version); err != nil {
		return err
	}
	if version != 1 && version != 2 {
		return fmt.Errorf("不支持的标注库格式版本")
	}
	hasTitle, err := backupHasTitle(tx, "recovery")
	if err != nil {
		return err
	}
	if version == 2 && !hasTitle {
		return fmt.Errorf("标注库缺少 title 列")
	}
	title := `''`
	if hasTitle {
		title = `title`
	}
	_, err = tx.Exec(`INSERT INTO marks(key,favorite,archived,tags,note,title,updated) SELECT key,favorite,archived,tags,note,` + title + `,updated FROM recovery.marks WHERE true ON CONFLICT(key) DO UPDATE SET favorite=excluded.favorite,archived=excluded.archived,tags=excluded.tags,note=excluded.note,title=excluded.title,updated=excluded.updated`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
