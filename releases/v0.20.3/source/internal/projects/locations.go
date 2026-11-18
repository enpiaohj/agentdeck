package projects

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// MergeBackup 在单个事务中合并项目及关联；同键采用备份，其他当前记录保留。
// 目标目录可能暂不可用，恢复不检查磁盘，留待用户执行时重新选择。
func (s *Store) MergeBackup(path string) error {
	if err := ValidateBackup(path); err != nil {
		return err
	}
	source, err := Open(path)
	if err != nil {
		return err
	}
	defer source.Close()
	declared, err := source.All()
	if err != nil {
		return err
	}
	locations, err := source.Locations()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range declared {
		p, err := ValidatePath(item.Path)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO declared_projects(lkey,path,name,added,agent) VALUES(?,?,?,?,?) ON CONFLICT(lkey) DO UPDATE SET path=excluded.path,name=excluded.name,added=excluded.added,agent=excluded.agent`, strings.ToLower(p), p, item.Name, item.Added.UTC().Format(time.RFC3339Nano), item.Agent); err != nil {
			return err
		}
	}
	for _, item := range locations {
		original, err := ValidatePath(item.Original)
		if err != nil {
			return err
		}
		target, err := ValidatePath(item.Target)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO project_locations(lkey,original,target) VALUES(?,?,?) ON CONFLICT(lkey) DO UPDATE SET original=excluded.original,target=excluded.target`, strings.ToLower(original), original, target); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Location 是原项目目录与当前实际目录的一条关联。
type Location struct {
	Original string `json:"original"`
	Target   string `json:"target"`
}

// Location 按规范化完整路径精确关联，不匹配子目录，也不递归追踪目标。
//
// 路径本身不合法（空、相对、含控制字符）时返回原值：关联写入时两侧都经过
// ValidatePath，表里不可能存在这类键，因此「查不到关联」与「路径不规范」对
// 调用方是同一件事——沿用原路径，由后续的目录检查给出结论，而不是让一条
// 本该能恢复的会话因为查关联而失败。
func (s *Store) Location(path string) (string, error) {
	p, err := ValidatePath(path)
	if err != nil {
		return path, nil
	}
	var target string
	err = s.db.QueryRow(`SELECT target FROM project_locations WHERE lkey = ?`, strings.ToLower(p)).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	return target, err
}

func (s *Store) SetLocation(original, target string) error {
	p, err := ValidatePath(original)
	if err != nil {
		return err
	}
	dest, err := ValidatePath(target)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(dest); err != nil || !fi.IsDir() {
		return fmt.Errorf("新项目位置不可用，请选择已存在的目录：%s", dest)
	}
	if strings.EqualFold(p, dest) {
		return s.RemoveLocation(p)
	}
	_, err = s.db.Exec(`INSERT INTO project_locations(lkey, original, target) VALUES (?, ?, ?) ON CONFLICT(lkey) DO UPDATE SET original=excluded.original, target=excluded.target`, strings.ToLower(p), p, dest)
	return err
}

func (s *Store) RemoveLocation(original string) error {
	p, err := ValidatePath(original)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM project_locations WHERE lkey = ?`, strings.ToLower(p))
	return err
}

func (s *Store) Locations() ([]Location, error) {
	rows, err := s.db.Query(`SELECT original, target FROM project_locations ORDER BY original`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		var item Location
		if err := rows.Scan(&item.Original, &item.Target); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
