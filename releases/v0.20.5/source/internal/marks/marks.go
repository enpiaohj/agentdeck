// Package marks 是 AgentDeck 自己的用户标注层：收藏、标签、归档、备注。
//
// 这些是用户在 AgentDeck 里加的数据，不属于任何上游 Agent 的存储，
// 因此单独落一份自己的库（%APPDATA%\AgentDeck\marks.db，随 Roaming
// 漫游），按会话 key（agent:id）对应到列表上。归档在这里只是标记——
// 列表过滤隐藏，源文件一个字节都不动，可随时撤销。
//
// 经由 internal/sqlitedb 打开数据库而非直接 import modernc.org/sqlite：
// modernc 的 libc 在 getenv 遇到无 "=" 的环境变量时会直接 os.Exit(1)。
// sqlitedb 的 init 先把这类条目剔除，是整个程序唯一允许接触 libc 的
// 地方（见 sqlitedb 包注释与 #2327）。
package marks

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/enpiaohj/agentdeck/internal/sqlitedb"
)

// Mark 是一个会话在 AgentDeck 里的全部用户标注。key 形如 agent:id，
// 与 sessions.Get 的键同一写法。
type Mark struct {
	Key      string    `json:"key"`
	Favorite bool      `json:"favorite"`
	Archived bool      `json:"archived"`
	Tags     []string  `json:"tags"`
	Note     string    `json:"note"`
	Title    string    `json:"title"`
	Updated  time.Time `json:"updated"`
}

// Store 是 marks.db 的句柄。service 层的调用来自 Wails 的多个 goroutine，
// 写操作经 mu 串行；读走 database/sql 连接池。
type Store struct {
	db   *sql.DB
	path string
	mu   sync.Mutex
}

// schema 是新库的表结构。旧库通过 migrate 增列升级。
var schema = [...]string{
	`CREATE TABLE IF NOT EXISTS marks (
		key      TEXT PRIMARY KEY,
		favorite INTEGER NOT NULL DEFAULT 0,
		archived INTEGER NOT NULL DEFAULT 0,
		tags     TEXT    NOT NULL DEFAULT '[]',
		note     TEXT    NOT NULL DEFAULT '',
		title    TEXT    NOT NULL DEFAULT '',
		updated  TEXT    NOT NULL DEFAULT ''
	)`,
	`PRAGMA user_version = 2`,
}

// keyRe 是会话 key 的白名单：agent 段小写字母数字连字符，id 段是各
// Agent 实际用的 id 字符集（UUID、sess_… 等）。key 来自上游会话文件，
// 属不可信输入——入库前校验，参数化查询，绝不进路径或 shell。
var keyRe = regexp.MustCompile(`^[a-z0-9-]+:[0-9A-Za-z_-]+$`)

// 限制：标签个数与单个标签长度、备注长度，防一条会话的备注写满一行。
const (
	maxTags     = 32
	maxTagLen   = 64
	maxNoteRun  = 10000
	maxTitleRun = 200
)

// DefaultPath 是标注库的默认位置：%APPDATA%\AgentDeck\marks.db。
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "AgentDeck", "marks.db"), nil
}

// Open 打开（必要时创建）位于 path 的标注库。
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

// migrate 在单个事务里升级旧标注库，失败时保留原库版本与内容。
func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	switch version {
	case 0:
		var exists int
		if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='marks'`).Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			return fmt.Errorf("标注库未标版本且已有 marks 表，拒绝迁移")
		}
		for _, stmt := range schema {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
	case 1:
		// 旧版程序重开已升级的库时会把 user_version 写回 1；列仍在。
		if err := validateMarkColumns(tx, 1); err != nil {
			return err
		}
		var titleColumns int
		if err := tx.QueryRow(`SELECT count(*) FROM pragma_table_info('marks') WHERE name='title'`).Scan(&titleColumns); err != nil {
			return err
		}
		if titleColumns == 0 {
			if _, err := tx.Exec(`ALTER TABLE marks ADD COLUMN title TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
			return err
		}
	case 2:
		if err := validateMarkColumns(tx, 2); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持的标注库 schema 版本 %d", version)
	}
	return tx.Commit()
}

func validateMarkColumns(tx *sql.Tx, version int) error {
	rows, err := tx.Query(`PRAGMA table_info(marks)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); err != nil {
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range []string{"key", "favorite", "archived", "tags", "note", "updated"} {
		if !columns[name] {
			return fmt.Errorf("标注库缺少 %s 列", name)
		}
	}
	if version == 2 && !columns["title"] {
		return fmt.Errorf("标注库缺少 title 列")
	}
	return nil
}

// Close 关闭底层库。
func (s *Store) Close() error {
	return s.db.Close()
}

// Path 返回库文件路径(恢复流程重建句柄用)。
func (s *Store) Path() string { return s.path }

// BackupTo 用 VACUUM INTO 做在线一致性备份:连接内原子完成,不复制
// 正在写入的库文件(F09)。目标已存在时报错,不覆盖已有备份。
func (s *Store) BackupTo(dest string) error {
	if fi, err := os.Stat(dest); err == nil && !fi.IsDir() {
		return fmt.Errorf("备份目标已存在,不覆盖: %s", dest)
	}
	_, err := s.db.Exec(`VACUUM INTO ?`, dest)
	return err
}

// Validate 校验 path 是合法的 AgentDeck 标注库，恢复前检查用。
func Validate(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v != 1 && v != 2 {
		return fmt.Errorf("schema 版本 %d 不是本程序标注库(期望 1 或 2)", v)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='marks'`).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("缺少 marks 表,不是本程序标注库")
	}
	return nil
}

// Get 读一个会话的标注；没有记录时返回零值 Mark（Tags 为 nil）而非错误。
func (s *Store) Get(key string) (Mark, error) {
	m := Mark{Key: key}
	var tags string
	var updated string
	err := s.db.QueryRow(`SELECT favorite, archived, tags, note, title, updated FROM marks WHERE key = ?`, key).
		Scan(&m.Favorite, &m.Archived, &tags, &m.Note, &m.Title, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	m.Tags = decodeTags(tags)
	m.Updated = parseTime(updated)
	return m, nil
}

// All 取全部标注，供列表一次性合成。
func (s *Store) All() (map[string]Mark, error) {
	rows, err := s.db.Query(`SELECT key, favorite, archived, tags, note, title, updated FROM marks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Mark{}
	for rows.Next() {
		var m Mark
		var tags, updated string
		if err := rows.Scan(&m.Key, &m.Favorite, &m.Archived, &tags, &m.Note, &m.Title, &updated); err != nil {
			return nil, err
		}
		m.Tags = decodeTags(tags)
		m.Updated = parseTime(updated)
		out[m.Key] = m
	}
	return out, rows.Err()
}

// SetFavorite 收藏/取消收藏。
func (s *Store) SetFavorite(key string, v bool) error {
	return s.update(key, func(m *Mark) { m.Favorite = v })
}

// SetArchived 归档/撤销归档。归档只是标记，源文件不动。
func (s *Store) SetArchived(key string, v bool) error {
	return s.update(key, func(m *Mark) { m.Archived = v })
}

// SetTags 整组替换标签：去空、去重、去首尾空白，超限报错而非静默截断。
func (s *Store) SetTags(key string, tags []string) error {
	clean := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		if len([]rune(t)) > maxTagLen {
			return fmt.Errorf("标签 %q 超过 %d 字", t, maxTagLen)
		}
		seen[t] = true
		clean = append(clean, t)
	}
	if len(clean) > maxTags {
		return fmt.Errorf("标签最多 %d 个，给了 %d 个", maxTags, len(clean))
	}
	return s.update(key, func(m *Mark) { m.Tags = clean })
}

// SetNote 写备注。
func (s *Store) SetNote(key string, note string) error {
	if len([]rune(note)) > maxNoteRun {
		return fmt.Errorf("备注最多 %d 字", maxNoteRun)
	}
	return s.update(key, func(m *Mark) { m.Note = note })
}

// SetTitle 保存本地显示标题；空串清除覆盖，恢复来源标题。
func (s *Store) SetTitle(key, title string) error {
	title = strings.TrimSpace(title)
	if len([]rune(title)) > maxTitleRun {
		return fmt.Errorf("标题最多 %d 字", maxTitleRun)
	}
	for _, r := range title {
		if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 {
			return fmt.Errorf("标题不能包含换行或控制字符")
		}
	}
	return s.update(key, func(m *Mark) { m.Title = title })
}

// TagsInUse 是库里出现过的全部标签（去重、原顺序），侧栏标签清单用。
func (s *Store) TagsInUse() ([]string, error) {
	rows, err := s.db.Query(`SELECT tags FROM marks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	seen := map[string]bool{}
	for rows.Next() {
		var tags string
		if err := rows.Scan(&tags); err != nil {
			return nil, err
		}
		for _, t := range decodeTags(tags) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out, rows.Err()
}

// update 读出该 key 现有标注、套用 change、整体写回（无则插入），
// 一个事务完成。写操作经 mu 串行，避免两个 goroutine 交错读改写。
func (s *Store) update(key string, change func(*Mark)) error {
	if !keyRe.MatchString(key) {
		return fmt.Errorf("非法的会话 key: %q", key)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	m := Mark{Key: key}
	var tags, updated string
	err = tx.QueryRow(`SELECT favorite, archived, tags, note, title, updated FROM marks WHERE key = ?`, key).
		Scan(&m.Favorite, &m.Archived, &tags, &m.Note, &m.Title, &updated)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		tx.Rollback()
		return err
	default:
		m.Tags = decodeTags(tags)
		m.Updated = parseTime(updated)
	}
	change(&m)
	m.Updated = time.Now().UTC()
	if _, err := tx.Exec(`INSERT INTO marks(key, favorite, archived, tags, note, title, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			favorite = excluded.favorite, archived = excluded.archived,
			tags = excluded.tags, note = excluded.note, title = excluded.title, updated = excluded.updated`,
		key, b2i(m.Favorite), b2i(m.Archived), encodeTags(m.Tags), m.Note, m.Title,
		m.Updated.Format(time.RFC3339Nano)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

func encodeTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeTags(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var tags []string
	if json.Unmarshal([]byte(s), &tags) != nil {
		return nil
	}
	return tags
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
