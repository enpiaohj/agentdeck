// Package history owns AgentDeck's permanent conversation library. It is not
// an index/cache: there is deliberately no automatic expiry or pruning. Explicit
// user-requested deletion is handled separately with recovery backups.
package history

import (
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
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
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
		`CREATE TABLE IF NOT EXISTS history_versions (key TEXT NOT NULL, hash TEXT NOT NULL, transcript BLOB NOT NULL, saved_at TEXT NOT NULL, PRIMARY KEY(key, hash))`,
		`CREATE TABLE IF NOT EXISTS history_sessions (key TEXT PRIMARY KEY, metadata BLOB NOT NULL, hash TEXT NOT NULL)`,
	} {
		if _, err = db.Exec(stmt); err != nil {
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
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO history_versions(key,hash,transcript,saved_at) VALUES(?,?,?,?) ON CONFLICT(key,hash) DO NOTHING`, key, hash, body, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO history_sessions(key,metadata,hash) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET metadata=excluded.metadata,hash=excluded.hash`, key, metadata, hash); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Get(key string) (sessions.Session, sessions.Transcript, bool, error) {
	var metadata, body []byte
	err := s.db.QueryRow(`SELECT s.metadata,v.transcript FROM history_sessions s JOIN history_versions v ON v.key=s.key AND v.hash=s.hash WHERE s.key=?`, key).Scan(&metadata, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return sessions.Session{}, sessions.Transcript{}, false, nil
	}
	if err != nil {
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

func (s *Store) Stats() (Stats, error) {
	var out Stats
	err := s.db.QueryRow(`SELECT (SELECT count(*) FROM history_sessions), count(*), coalesce(sum(length(transcript)),0) FROM history_versions`).Scan(&out.Sessions, &out.Versions, &out.Bytes)
	return out, err
}
