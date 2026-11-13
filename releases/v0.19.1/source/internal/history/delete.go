package history

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Match the session-key whitelist used by the user annotation store.
var deletionKeyRE = regexp.MustCompile(`^[a-z0-9-]+:[0-9A-Za-z_-]+$`)

func deletionKeys(keys []string) ([]string, error) {
	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !deletionKeyRE.MatchString(key) {
			return nil, fmt.Errorf("非法的会话 key: %q", key)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return out, nil
}

// DeleteKeys atomically deletes only explicitly selected sessions and all their
// versions. The caller must first create a recovery backup for manual undo.
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
		if _, err := tx.Exec(`DELETE FROM history_versions WHERE key=?`, key); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM history_sessions WHERE key=?`, key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type deletionVersion struct {
	hash string
	// stored 是库里的字节（可能压缩过），plain 是解出来的 JSON。比对用
	// plain：压缩形态只是存储方式，同一份正文在升级前后必须被认作同一份
	// （否则迁移压缩过的会话，撤销删除会误报冲突）。
	stored, plain, savedAt string
	codec                  int
}

type deletionRecord struct {
	present        bool
	metadata, hash string
	versions       []deletionVersion
}

func readDeletionRecord(q rowQuerier, key string, withCodec bool) (deletionRecord, error) {
	var record deletionRecord
	err := q.QueryRow(`SELECT metadata,hash FROM history_sessions WHERE key=?`, key).Scan(&record.metadata, &record.hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return record, err
	}
	record.present = err == nil
	query := `SELECT hash,transcript,saved_at FROM history_versions WHERE key=? ORDER BY hash`
	if withCodec {
		query = `SELECT hash,transcript,saved_at,codec FROM history_versions WHERE key=? ORDER BY hash`
	}
	rows, err := q.Query(query, key)
	if err != nil {
		return record, err
	}
	defer rows.Close()
	for rows.Next() {
		var version deletionVersion
		scan := []any{&version.hash, &version.stored, &version.savedAt}
		if withCodec {
			scan = append(scan, &version.codec)
		}
		if err := rows.Scan(scan...); err != nil {
			return record, err
		}
		plain, err := decodeBody([]byte(version.stored), version.codec)
		if err != nil {
			return record, err
		}
		version.plain = string(plain)
		record.versions = append(record.versions, version)
	}
	return record, rows.Err()
}

// RestoreKeysFrom validates and reads a caller-managed, trusted recovery backup
// read-only, then restores only selected absent keys in one transaction. Existing
// keys are accepted only when metadata and every version exactly match the backup,
// allowing undo retries; any differing current content rejects the whole batch.
// A key absent from the backup remains absent. Unrelated data is never changed.
func (s *Store) RestoreKeysFrom(path string, keys []string) error {
	keys, err := deletionKeys(keys)
	if err != nil || len(keys) == 0 {
		return err
	}
	if _, err := ValidateBackup(path); err != nil {
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
	records := make(map[string]deletionRecord, len(keys))
	backupCodec, err := hasCodecColumn(backup, "main")
	if err != nil {
		return err
	}
	for _, key := range keys {
		record, err := readDeletionRecord(source, key, backupCodec)
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
	liveCodec, err := hasCodecColumn(tx, "main")
	if err != nil {
		return err
	}
	for _, key := range keys {
		current, err := readDeletionRecord(tx, key, liveCodec)
		if err != nil {
			return err
		}
		record := records[key]
		if current.present || len(current.versions) > 0 {
			if current.present != record.present || current.metadata != record.metadata || current.hash != record.hash || !slices.Equal(current.versions, record.versions) {
				return fmt.Errorf("恢复冲突，会话已有不同内容: %s", key)
			}
			continue
		}
		for _, version := range record.versions {
			if _, err := tx.Exec(`INSERT INTO history_versions(key,hash,transcript,saved_at,codec) VALUES(?,?,?,?,?)`, key, version.hash, []byte(version.stored), version.savedAt, version.codec); err != nil {
				return err
			}
		}
		if record.present {
			if _, err := tx.Exec(`INSERT INTO history_sessions(key,metadata,hash) VALUES(?,?,?)`, key, []byte(record.metadata), record.hash); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
