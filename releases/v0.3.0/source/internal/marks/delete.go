package marks

import (
	"database/sql"
	"errors"
	"fmt"
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

type deletionRecord struct {
	present             bool
	favorite, archived  int
	tags, note, updated string
}

func readDeletionRecord(tx *sql.Tx, key string) (deletionRecord, error) {
	var record deletionRecord
	err := tx.QueryRow(`SELECT favorite,archived,tags,note,updated FROM marks WHERE key=?`, key).
		Scan(&record.favorite, &record.archived, &record.tags, &record.note, &record.updated)
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
	records := make(map[string]deletionRecord, len(keys))
	for _, key := range keys {
		record, err := readDeletionRecord(source, key)
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
		current, err := readDeletionRecord(tx, key)
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
			if _, err := tx.Exec(`INSERT INTO marks(key,favorite,archived,tags,note,updated) VALUES(?,?,?,?,?,?)`, key, record.favorite, record.archived, record.tags, record.note, record.updated); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
