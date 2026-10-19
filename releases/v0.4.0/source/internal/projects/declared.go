package projects

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/enpiaohj/agentdeck/internal/sqlitedb"
)

// 本文件是「用户声明的项目」：还没产生任何会话、但用户想让它出现在
// AgentDeck 项目列表里的目录。Group 从会话聚合，只认已有会话的目录；
// 一个刚建、还没开过会话的目录在那里是看不见的。
//
// 之所以单独一份而不是并进 Group：Group 是纯函数（会话 → 项目），塞进
// 持久化状态会让它既不是纯函数也不再可测试，而且语义混淆——「聚合出来
// 的项目」与「用户声明的项目」生命周期不同。

// errNotAbsolute 是 Add 对非法路径的拒绝：项目必须是绝对路径，否则
// 无法确定在哪个目录开新会话。
var errNotAbsolute = errors.New("项目路径必须是绝对路径")

// Declared 是用户声明的一个项目：路径 + 展示名。
type Declared struct {
	Path  string    // 归一化路径，保留原大小写（展示用）
	Name  string    // 展示名；缺省用末级目录
	Added time.Time // 声明时间
}

// Store 是声明项目清单的存储：AgentDeck 自己的一份小库（用户数据，
// Roaming）。并发安全，一个进程持有一个实例。
//
// 表里 path 存**原样大小写**的归一化路径（展示与 Set-Location 要用），
// lkey 存它的小写形式作唯一键——SQLite 默认 BINARY collation 区分大小写，
// 而 Windows 路径不区分，两个字段分开存才能两者都要。
//
// 与 marks 一样经 internal/sqlitedb 打开，不直接 import
// modernc.org/sqlite（见 marks 包注释与 #2327）。
type Store struct {
	db *sql.DB
}

// schema 是声明项目的表结构。只加列不改列。
const schema = `CREATE TABLE IF NOT EXISTS declared_projects (
	lkey  TEXT PRIMARY KEY,
	path  TEXT NOT NULL,
	name  TEXT NOT NULL DEFAULT '',
	added TEXT NOT NULL DEFAULT ''
)`

// Open 打开（必要时创建）声明项目清单库。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close 关闭底层库。
func (s *Store) Close() error { return s.db.Close() }

// Add 声明一个项目。同一路径（不分大小写）只保留一行，重复声明只更新
// 展示名。返回归一化后的原样路径（调用方拿它去建目录/开会话）。
func (s *Store) Add(path, name string) (string, error) {
	p := Normalize(path)
	if p == "" || !filepath.IsAbs(p) {
		return "", errNotAbsolute
	}
	if name == "" {
		name = DisplayName(p)
	}
	if _, err := s.db.Exec(`INSERT INTO declared_projects(lkey, path, name, added)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(lkey) DO UPDATE SET path = excluded.path, name = excluded.name`,
		strings.ToLower(p), p, name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return "", err
	}
	return p, nil
}

// Remove 取消声明。未声明过的路径不算错（幂等）。
func (s *Store) Remove(path string) error {
	p := Normalize(path)
	if p == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM declared_projects WHERE lkey = ?`, strings.ToLower(p))
	return err
}

// All 取全部声明的项目，按路径排序（稳定，前端不必再排）。
func (s *Store) All() ([]Declared, error) {
	rows, err := s.db.Query(`SELECT path, name, added FROM declared_projects ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Declared{}
	for rows.Next() {
		var d Declared
		var added string
		if err := rows.Scan(&d.Path, &d.Name, &added); err != nil {
			return nil, err
		}
		d.Added, _ = time.Parse(time.RFC3339Nano, added)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Has 说 path 是否已声明。
func (s *Store) Has(path string) (bool, error) {
	p := Normalize(path)
	if p == "" {
		return false, nil
	}
	rows, err := s.db.Query(`SELECT 1 FROM declared_projects WHERE lkey = ?`, strings.ToLower(p))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

// SortDeclared 按展示名排序，与 Group 的结果并排展示时稳定。
func SortDeclared(in []Declared) {
	sort.SliceStable(in, func(i, j int) bool {
		return strings.ToLower(in[i].Name) < strings.ToLower(in[j].Name)
	})
}
