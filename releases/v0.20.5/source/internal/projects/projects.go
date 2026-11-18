// Package projects 把会话按工作目录聚合成项目视图。
//
// 归组依据是 Session.Cwd——它已是会话层解析后的真实路径（侦察结论 #4
// 的三种路径表示在 sessions 层消化，目录名的有损编码不可反解，本包
// 不做也不需要反解）。Normalize 只处理残余差异：扩展长度前缀、尾
// 分隔符、Clean，比较时大小写折叠（Windows 路径不区分大小写）。
package projects

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Project 是一个聚合出的项目：同一归一化目录下全部会话的归组。
type Project struct {
	Path  string    // 代表路径：组内首个会话的归一化 Cwd，保留原大小写
	Name  string    // 展示名：末级目录；盘符根为 "D:\"
	Drive string    // "D"；UNC 为共享名
	Last  time.Time // 组内最新的会话活动时间
	Keys  []string  // 会话 key（agent:id），按 Last 降序
}

// Normalize 是归一化比较键：剥扩展长度前缀、Clean。返回值保留原
// 大小写——展示用；归组时另以小写形式做 map 键（见 Group）。
func Normalize(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	cwd = strings.TrimPrefix(cwd, `\\?\`)
	if cwd == "" {
		return ""
	}
	return filepath.Clean(cwd)
}

// Group 按归一化 Cwd 归组。Cwd 为空的会话（上游没写 cwd 的少数记录）
// 不属于任何项目，跳过。返回值按项目最新活动降序。
func Group(list []sessions.Session) []Project {
	byKey := map[string]*Project{}   // 小写归一化路径 → 组
	paths := map[string]string{}     // 小写键 → 首个见到的原样路径
	latest := map[string]time.Time{} // 小写键 → 组内最新活动
	lasts := map[string]time.Time{}  // 会话 key → Last，排序用
	for _, s := range list {
		if !s.Last.IsZero() {
			lasts[s.Agent+":"+s.ID] = s.Last
		}
		norm := Normalize(s.Cwd)
		if norm == "" {
			continue
		}
		lower := strings.ToLower(norm)
		p, ok := byKey[lower]
		if !ok {
			p = &Project{Path: norm}
			byKey[lower] = p
			paths[lower] = norm
		}
		p.Keys = append(p.Keys, s.Agent+":"+s.ID)
		if s.Last.After(latest[lower]) {
			latest[lower] = s.Last
		}
	}
	out := make([]Project, 0, len(byKey))
	for lower, p := range byKey {
		p.Last = latest[lower]
		p.Drive = drive(paths[lower])
		p.Name = DisplayName(paths[lower])
		sort.SliceStable(p.Keys, func(i, j int) bool {
			a, aok := lasts[p.Keys[i]]
			b, bok := lasts[p.Keys[j]]
			if aok != bok {
				return bok // 读不到时间的排后面
			}
			if !a.Equal(b) {
				return a.After(b)
			}
			return p.Keys[i] < p.Keys[j]
		})
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Last.Equal(out[j].Last) {
			return out[i].Last.After(out[j].Last)
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// drive 取盘符（"D"）或 UNC 共享名；没有卷标的为空。
func drive(p string) string {
	vol := filepath.VolumeName(p)
	return strings.TrimSuffix(vol, ":")
}

// Drive 是 drive 的导出形式：取盘符（"D"）或 UNC 共享名。装配
// ProjectView.Drive 用，声明项目没有会话可聚合时也走这条。
func Drive(p string) string { return drive(p) }

// DisplayName 是项目展示名：末级目录；盘符根本身就是名（"D:\"）。
// 导出给 app 服务层装配 SessionView.Project 用。
func DisplayName(p string) string {
	base := filepath.Base(p)
	if base == "." || base == string(filepath.Separator) || base == p {
		return p
	}
	return base
}
