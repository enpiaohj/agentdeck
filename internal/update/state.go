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

	// 资产信息也要落盘：恢复出来的结论必须能直接下载。只存版本号的话，
	// 「检查过之后当天再打开应用」就会拿到一个不带下载地址的结论，界面上
	// 按钮可点、点下去必然失败（曾如此）。旧版本写下的 check.json 没有这些
	// 字段，读回来是空值，此时下载会明确要求重新检查。
	ExeName   string `json:"exe_name,omitempty"`
	ExeURL    string `json:"exe_url,omitempty"`
	ExeSize   int64  `json:"exe_size,omitempty"`
	ExeDigest string `json:"exe_digest,omitempty"`
	SumsURL   string `json:"sums_url,omitempty"`
}

// Assets 把落盘的资产信息还原成可下载的资产；没有下载地址时返回 false。
func (s State) Assets() (Asset, Asset, bool) {
	if s.ExeURL == "" || s.ExeName == "" {
		return Asset{}, Asset{}, false
	}
	exe := Asset{Name: s.ExeName, URL: s.ExeURL, Size: s.ExeSize, Digest: s.ExeDigest}
	var sums Asset
	if s.SumsURL != "" {
		sums = Asset{Name: SumsAsset, URL: s.SumsURL}
	}
	return exe, sums, true
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
