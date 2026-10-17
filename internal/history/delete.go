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
	hash, transcript, savedAt string
}

type deletionRecord struct {
	present        bool
	metadata, hash string
	versions       []deletionVersion
}

func readDeletionRecord(tx *sql.Tx, key string) (deletionRecord, error) {
	var record deletionRecord
	err := tx.QueryRow(`SELECT metadata,hash FROM history_sessions WHERE key=?`, key).Scan(&record.metadata, &record.hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return record, err
	}
	record.present = err == nil
	rows, err := tx.Query(`SELECT hash,transcript,saved_at FROM history_versions WHERE key=? ORDER BY hash`, key)
	if err != nil {
		return record, err
	}
	defer rows.Close()
	for rows.Next() {
		var version deletionVersion
		if err := rows.Scan(&version.hash, &version.transcript, &version.savedAt); err != nil {
			return record, err
		}
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
		if current.present || len(current.versions) > 0 {
			if current.present != record.present || current.metadata != record.metadata || current.hash != record.hash || !slices.Equal(current.versions, record.versions) {
				return fmt.Errorf("恢复冲突，会话已有不同内容: %s", key)
			}
			continue
		}
		for _, version := range record.versions {
			if _, err := tx.Exec(`INSERT INTO history_versions(key,hash,transcript,saved_at) VALUES(?,?,?,?)`, key, version.hash, []byte(version.transcript), version.savedAt); err != nil {
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
