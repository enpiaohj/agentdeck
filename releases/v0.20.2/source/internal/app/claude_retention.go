package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Claude Code 会在启动时清理超过 cleanupPeriodDays（默认 30）天没有改动的会话文件。这里只读地
// 把这个设置和“哪些会话快到期”告诉用户，由用户自己决定要不要调大——AgentDeck 不去写 Claude 的
// settings.json。

const (
	claudeDefaultRetentionDays = 30
	retentionWarnWindow        = 7 * 24 * time.Hour
	maxRetentionRisks          = 5
)

// RetentionRisk 是一个快被 Claude Code 清理的会话。
type RetentionRisk struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	DaysLeft int    `json:"days_left"` // 距清理还剩几天；<=0 表示已到期，下次启动 Claude Code 时会被清理
	Saved    bool   `json:"saved"`     // 永久库里是否已有副本（清理后仍可查看、可重建）
}

// ClaudeRetention 是 Claude Code 会话保留期的只读快照。
type ClaudeRetention struct {
	Days     int             `json:"days"`     // 生效的保留天数；0 表示 Claude 设置成了“不保存”
	Explicit bool            `json:"explicit"` // 是否在 settings.json 里明确设置
	Source   string          `json:"source"`   // 读取的 settings.json 路径
	Error    string          `json:"error"`    // 读取失败的原因（按默认值估算）
	Checked  int             `json:"checked"`  // 检查了多少个本机 Claude 会话
	Soon     int             `json:"soon"`     // 其中 7 天内到期或已到期的数量
	Unsaved  int             `json:"unsaved"`  // Soon 里永久库还没有副本的数量
	Risks    []RetentionRisk `json:"risks"`    // 最先到期的几个
	Snippet  string          `json:"snippet"`  // 建议加进 settings.json 的内容
}

// ClaudeRetention 读取 Claude Code 的会话保留期设置，并统计快到期的会话。只读：不改任何文件。
func (s *SettingsService) ClaudeRetention() ClaudeRetention {
	a := s.a
	out := ClaudeRetention{Days: claudeDefaultRetentionDays, Snippet: `"cleanupPeriodDays": 365`}
	path := filepath.Join(sessions.ClaudeDir(), "settings.json")
	out.Source = path
	days, explicit, err := readCleanupPeriod(path)
	switch {
	case err != nil:
		out.Error = err.Error()
	case explicit:
		out.Days, out.Explicit = days, true
	}
	if out.Days <= 0 {
		return out // 0 = Claude 不保存会话，没有“到期”可言；界面单独提示
	}
	saved := map[string]bool{}
	if list, err := a.history.List(); err == nil {
		for _, sess := range list {
			saved[sess.Agent+":"+sess.ID] = true
		}
	}
	list, _, _ := a.snapshot()
	now := time.Now()
	limit := time.Duration(out.Days) * 24 * time.Hour
	type aged struct {
		RetentionRisk
		left time.Duration
	}
	var soon []aged
	for _, sess := range list {
		if sess.Agent != "claude" || sess.Retained || sess.Imported || sess.Gateway || sess.WSL != "" || sess.Path == "" {
			continue
		}
		// Claude Code 看的是文件修改时间
		modified := sess.Last
		if info, err := os.Stat(sess.Path); err == nil {
			modified = info.ModTime()
		}
		if modified.IsZero() {
			continue
		}
		out.Checked++
		left := limit - now.Sub(modified)
		if left >= retentionWarnWindow {
			continue
		}
		key := sess.Agent + ":" + sess.ID
		title := sess.Title
		if title == "" {
			title = "未命名会话"
		}
		days := int(math.Ceil(left.Hours() / 24))
		soon = append(soon, aged{RetentionRisk{Key: key, Title: title, DaysLeft: days, Saved: saved[key]}, left})
		out.Soon++
		if !saved[key] {
			out.Unsaved++
		}
	}
	sort.Slice(soon, func(i, j int) bool { return soon[i].left < soon[j].left })
	for i := 0; i < len(soon) && i < maxRetentionRisks; i++ {
		out.Risks = append(out.Risks, soon[i].RetentionRisk)
	}
	return out
}

// readCleanupPeriod 读取 settings.json 里的 cleanupPeriodDays。文件不存在不算错（用默认值）。
func readCleanupPeriod(path string) (days int, explicit bool, err error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return 0, false, fmt.Errorf("%s 不是可读取的设置文件", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false, err
	}
	var settings struct {
		Cleanup *float64 `json:"cleanupPeriodDays"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return 0, false, fmt.Errorf("无法解析 %s：%w", path, err)
	}
	if settings.Cleanup == nil || math.IsNaN(*settings.Cleanup) || math.IsInf(*settings.Cleanup, 0) {
		return 0, false, nil
	}
	return int(math.Floor(*settings.Cleanup)), true, nil
}
