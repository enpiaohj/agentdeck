package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/projects"
)

// DiagRow 是一个 Agent 的能力诊断行。设计文档 §3/§11.3:能力由运行时
// 上报,UI 呈现证据、状态及验证时间——Found/Sessions/LastAt 即证据,
// 每次打开本页都是即时验证。
type DiagRow struct {
	Agent      string `json:"agent"`
	Found      bool   `json:"found"`       // 存储根是否发现(以扫描到会话为证)
	Sessions   int    `json:"sessions"`    // 发现的会话数
	LastAt     string `json:"last_at"`     // 最近活动(RFC3339)
	Resume     bool   `json:"resume"`      // 能构造原生恢复命令
	ResumeNote string `json:"resume_note"` // 恢复能力说明
	Source     string `json:"source"`      // local | wsl | gateway
}

// Diagnostics 逐 Agent 汇报发现与恢复能力(即时验证,非缓存表)。
func (s *SessionService) Diagnostics() []DiagRow {
	list, _, _ := s.a.snapshot()
	type agg struct {
		n      int
		last   time.Time
		wsl    bool
		local  bool
		resume bool
		path   string
	}
	by := map[string]*agg{}
	for _, sess := range list {
		a := by[sess.Agent]
		if a == nil {
			a = &agg{}
			by[sess.Agent] = a
		}
		a.n++
		if sess.Last.After(a.last) {
			a.last = sess.Last
		}
		if sess.WSL != "" {
			a.wsl = true
		} else {
			a.local = true
		}
		if !a.resume && sessionResume(sess) != "" {
			a.resume = true
		}
		if a.path == "" && sess.Path != "" {
			a.path = sess.Path
		}
	}
	out := make([]DiagRow, 0, len(by))
	for agent, a := range by {
		note := "原生恢复命令已验证构造"
		if !a.resume {
			note = "未发现可用的恢复命令,仅可查看历史"
		} else if a.wsl && a.local {
			note = "本机与 WSL 会话均有;WSL 会话仅支持复制恢复命令"
		} else if a.wsl {
			note = "WSL 会话:仅支持复制恢复命令,不支持终端直开"
		}
		source := "local"
		if a.wsl && !a.local {
			source = "wsl"
		}
		out = append(out, DiagRow{
			Agent: agent, Found: true, Sessions: a.n,
			LastAt: lastString(a.last), Resume: a.resume,
			ResumeNote: note, Source: source,
		})
	}
	s.a.mu.Lock()
	importError := s.a.importError
	s.a.mu.Unlock()
	if importError != "" {
		found := false
		for i := range out {
			if out[i].Agent == "claude-desktop" {
				out[i].ResumeNote += "; 聊天导入库读取失败: " + importError
				found = true
			}
		}
		if !found {
			out = append(out, DiagRow{Agent: "claude-desktop", ResumeNote: "聊天导入库读取失败: " + importError, Source: "import"})
		}
	}
	return out
}

// ---- 备份与恢复(F09)----

// BackupMarks 把标注库在线备份到 destDir(VACUUM INTO 一致性快照),
// 返回备份文件全路径。目标已存在同名文件时报错,不覆盖已有备份。
func (s *MarkService) BackupMarks(destDir string) (string, error) {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	dir := projects.Normalize(destDir)
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("备份目录必须是绝对路径: %s", destDir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("备份目录不存在: %s", dir)
	}
	dest := filepath.Join(dir, "agentdeck-marks-backup-"+time.Now().Format("20060102-150405")+".db")
	if err := s.a.marks.BackupTo(dest); err != nil {
		return "", err
	}
	return dest, nil
}

// RestoreMarks 从备份文件恢复标注库:校验 schema → 关闭当前库 →
// 现库自动备份为 .bak → 覆盖 → 重开 → 刷新缓存并广播。失败时保留
// 现库不动(先校验后动手,半途失败不破坏当前数据,对齐 TC-11)。
func (s *MarkService) RestoreMarks(src string) error {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := marks.Validate(src); err != nil {
		return fmt.Errorf("不是有效的标注库备份: %v", err)
	}
	a := s.a
	cur := a.marks.Path()
	// 现库留底:同目录 .bak-<时间戳>,不删除任何用户数据
	bak := cur + ".bak-" + time.Now().Format("20060102-150405")
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := a.marks.Close(); err != nil {
		return fmt.Errorf("关闭当前标注库失败: %v", err)
	}
	if err := os.Rename(cur, bak); err != nil {
		// 重开原库,保持可用
		a.marks, _ = marks.Open(cur)
		return fmt.Errorf("留底当前库失败: %v", err)
	}
	if err := os.WriteFile(cur, data, 0o600); err != nil {
		// 写回失败:留底还在,恢复它
		os.Rename(bak, cur)
		a.marks, _ = marks.Open(cur)
		return fmt.Errorf("写入恢复数据失败(当前库已还原): %v", err)
	}
	fresh, err := marks.Open(cur)
	if err != nil {
		return fmt.Errorf("重开标注库失败: %v", err)
	}
	a.mu.Lock()
	a.marks = fresh
	a.marksAll = map[string]marks.Mark{}
	a.mu.Unlock()
	a.emit(EventSessionsChanged, "")
	return nil
}

// ---- 崩溃恢复 ----

// lastCrashedLock 是运行标记文件:New 时写入、正常 Close 时删除。
// 启动时若已存在,说明上次进程未正常退出(崩溃/被杀),前端据此提示
// 「已自动恢复」。数据本身全是派生或 SQLite 持久化,无需重放动作。
// 多实例同时运行时该检测会误报——v1.0 假设单实例,后续用互斥锁收紧。
func crashLockPath(marksPath string) string {
	return filepath.Join(filepath.Dir(marksPath), "session.lock")
}

// checkCrash 在装配前检查残留 lock,返回「上次是否异常退出」并写新 lock。
func checkCrash(marksPath string) bool {
	lock := crashLockPath(marksPath)
	crashed := false
	if _, err := os.Stat(lock); err == nil {
		crashed = true
	}
	_ = os.WriteFile(lock, []byte(time.Now().Format(time.RFC3339)), 0o600)
	return crashed
}

// clearCrash 正常退出时清除标记。
func clearCrash(marksPath string) {
	_ = os.Remove(crashLockPath(marksPath))
}

// LastCrashed 上次进程是否未正常退出(前端启动横幅)。
func (s *SessionService) LastCrashed() bool {
	return s.a.crashed
}
