package history

import (
	"errors"
	"fmt"
)

// PruneStats 描述一次清理（或其预览）涉及的范围。
type PruneStats struct {
	Sessions int   // 有旧版本被清掉的会话数
	Versions int   // 被清掉的版本数
	Bytes    int64 // 这些版本占用的正文字节数（压缩后的存储大小）
}

// 候选集：每个会话按保存时间从新到旧排序，超出 keep 的才是候选；
// 每个会话当前指向的那一版（history_sessions.hash）无论多旧都不是候选。
const pruneCandidates = `
WITH ranked AS (
	SELECT v.rowid AS rid, v.key AS key, v.hash AS hash, length(v.transcript) AS len,
		row_number() OVER (PARTITION BY v.key ORDER BY v.saved_at DESC, v.rowid DESC) AS rn
	FROM history_versions v
), candidates AS (
	SELECT rid, key, len FROM ranked r
	WHERE rn > ? AND NOT EXISTS (SELECT 1 FROM history_sessions s WHERE s.key = r.key AND s.hash = r.hash)
)`

// PrunePreview 计算保留每个会话最新 keep 个版本时会清掉什么，不改动任何数据。
func (s *Store) PrunePreview(keep int) (PruneStats, error) {
	if keep < 1 {
		return PruneStats{}, errors.New("每个会话至少保留 1 个版本")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out PruneStats
	err := s.db.QueryRow(pruneCandidates+`SELECT count(DISTINCT key), count(*), coalesce(sum(len), 0) FROM candidates`, keep).
		Scan(&out.Sessions, &out.Versions, &out.Bytes)
	return out, err
}

// Prune 清掉每个会话超出最新 keep 个的旧版本，并回收文件空间。
//
// 这是永久库里唯一会删除版本的路径，只能由用户显式触发：
//   - 每个会话当前指向的版本永远保留，会话列表与正文读取不受影响；
//   - 候选集的计算与删除在同一个事务里，预览之后新写入的版本只会更新鲜，
//     不会被误删；
//   - VACUUM 失败不算清理失败（行已删净，只是文件暂时偏大）。
func (s *Store) Prune(keep int) (PruneStats, error) {
	if keep < 1 {
		return PruneStats{}, errors.New("每个会话至少保留 1 个版本")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return PruneStats{}, err
	}
	defer tx.Rollback()
	var out PruneStats
	if err = tx.QueryRow(pruneCandidates+`SELECT count(DISTINCT key), count(*), coalesce(sum(len), 0) FROM candidates`, keep).
		Scan(&out.Sessions, &out.Versions, &out.Bytes); err != nil {
		return PruneStats{}, err
	}
	if out.Versions == 0 {
		return out, nil
	}
	res, err := tx.Exec(`DELETE FROM history_versions WHERE rowid IN (`+pruneCandidates+`SELECT rid FROM candidates)`, keep)
	if err != nil {
		return PruneStats{}, err
	}
	if n, nerr := res.RowsAffected(); nerr != nil || int(n) != out.Versions {
		return PruneStats{}, fmt.Errorf("清理的版本数与预期不符（预期 %d，实际 %d），已回滚", out.Versions, n)
	}
	if err = tx.Commit(); err != nil {
		return PruneStats{}, err
	}
	// 顺序同 Compact：先 VACUUM 再检查点，否则整理后的整库留在 WAL 里。
	_, _ = s.db.Exec(`VACUUM`)
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return out, nil
}
