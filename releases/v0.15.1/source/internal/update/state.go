package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State 是上一次检查的结果，落盘以便启动时的后台检查遵守「每天最多一次」，
// 也用于在没有网络时仍然能显示上次的结论。
type State struct {
	CheckedAt   time.Time `json:"checked_at"`
	Current     string    `json:"current"`
	Latest      string    `json:"latest"`
	Available   bool      `json:"available"`
	Notes       string    `json:"notes,omitempty"`
	PageURL     string    `json:"page_url,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
}

// StateDue 判断距上次检查是否已超过 interval。零值（从未检查）视为到期。
func StateDue(state State, interval time.Duration, now time.Time) bool {
	if state.CheckedAt.IsZero() {
		return true
	}
	return now.Sub(state.CheckedAt) >= interval
}

// LoadState 读取状态文件；不存在或内容损坏时返回零值（视为从未检查）。
// 状态文件只影响提示频率，损坏不该阻止应用启动。
func LoadState(path string) State {
	raw, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var state State
	if err = json.Unmarshal(raw, &state); err != nil {
		return State{}
	}
	return state
}

// SaveState 原子写入状态文件（同目录临时文件 + 改名）。
func SaveState(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "update-state-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(name)
	}
	if _, err = tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err = tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err = os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
