package history

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/enpiaohj/agentdeck/internal/sessions"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) BackupTo(dest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`VACUUM INTO ?`, dest)
	return err
}
func ValidateBackup(path string) (Stats, error) {

	info, err := os.Lstat(path)
	if err != nil {
		return Stats{}, fmt.Errorf("备份数据库无法读取：%w", err)
	}
	if !info.Mode().IsRegular() {
		return Stats{}, fmt.Errorf("备份数据库必须是普通文件")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Stats{}, err
	}
	uri := "file:" + strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(absolute)) + "?mode=ro"
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return Stats{}, err
	}
	defer db.Close()
	var version, objects int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 0 {
		return Stats{}, fmt.Errorf("不支持的历史库格式版本")
	}
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE (type IN ('view','trigger')) OR (type='table' AND name NOT IN ('history_sessions','history_versions'))`).Scan(&objects); err != nil || objects != 0 {
		return Stats{}, fmt.Errorf("历史库表结构无效")
	}
	var integrity string
	if err = db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return Stats{}, fmt.Errorf("历史库完整性校验失败：%s %v", integrity, err)
	}
	var out Stats
	err = db.QueryRow(`SELECT (SELECT count(*) FROM history_sessions),count(*),coalesce(sum(length(transcript)),0) FROM history_versions`).Scan(&out.Sessions, &out.Versions, &out.Bytes)
	if err != nil {
		return out, err
	}
	var bad int
	err = db.QueryRow(`SELECT count(*) FROM history_sessions s LEFT JOIN history_versions v ON s.key=v.key AND s.hash=v.hash WHERE v.key IS NULL OR NOT json_valid(s.metadata)`).Scan(&bad)
	if err != nil || bad != 0 {
		return out, fmt.Errorf("历史库结构或内容无效：%v", err)
	}
	// 备份可能是升级前写的（没有 codec 列，正文一律明文），也可能是
	// 升级后写的（正文按 zstd 存）。两种都要能校验：先按码解出明文，
	// 再用明文核对哈希与 JSON。
	hasCodec, err := hasCodecColumn(db, "main")
	if err != nil {
		return out, err
	}
	bodyQuery := `SELECT key,hash,transcript,saved_at FROM history_versions`
	if hasCodec {
		bodyQuery = `SELECT key,hash,transcript,saved_at,codec FROM history_versions`
	}
	rows, err := db.Query(bodyQuery)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, hash, saved string
		var body, plain []byte
		var codec int
		scan := []any{&key, &hash, &body, &saved}
		if hasCodec {
			scan = append(scan, &codec)
		}
		if err = rows.Scan(scan...); err != nil {
			return out, err
		}
		if plain, err = decodeBody(body, codec); err != nil {
			return out, fmt.Errorf("无效历史正文：%w", err)
		}
		var tr sessions.Transcript
		if json.Unmarshal(plain, &tr) != nil || len(tr.Parts) == 0 {
			return out, fmt.Errorf("无效历史正文")
		}
		if _, e := time.Parse(time.RFC3339Nano, saved); e != nil {
			return out, fmt.Errorf("无效历史时间")
		}
		sum := sha256.Sum256(plain)
		if hex.EncodeToString(sum[:]) != hash || !json.Valid(plain) || key == "" {
			return out, fmt.Errorf("历史版本校验失败")
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	rows.Close()
	metaRows, err := db.Query(`SELECT key,metadata FROM history_sessions`)
	if err != nil {
		return out, err
	}
	defer metaRows.Close()
	for metaRows.Next() {
		var key string
		var raw []byte
		var sess sessions.Session
		if err = metaRows.Scan(&key, &raw); err != nil {
			return out, err
		}
		if json.Unmarshal(raw, &sess) != nil || sess.Agent == "" || sess.ID == "" || key != sess.Agent+":"+sess.ID {
			return out, fmt.Errorf("无效历史会话元数据")
		}
	}
	return out, metaRows.Err()
}
func (s *Store) MergeBackup(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`ATTACH DATABASE ? AS recovery`, path); err != nil {
		return err
	}
	defer s.db.Exec(`DETACH DATABASE recovery`)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 合并的是用户手上的备份文件，可能是升级前写的：没有 codec 列时按
	// 明文（0）并入。
	backupCodec, err := hasCodecColumn(tx, "recovery")
	if err != nil {
		return err
	}
	versions := `INSERT OR IGNORE INTO history_versions(key,hash,transcript,saved_at,codec) SELECT key,hash,transcript,saved_at,0 FROM recovery.history_versions`
	if backupCodec {
		versions = `INSERT OR IGNORE INTO history_versions(key,hash,transcript,saved_at,codec) SELECT key,hash,transcript,saved_at,codec FROM recovery.history_versions`
	}
	for _, q := range []string{versions, `INSERT OR IGNORE INTO history_sessions(key,metadata,hash) SELECT key,metadata,hash FROM recovery.history_sessions`} {
		if _, err = tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}
