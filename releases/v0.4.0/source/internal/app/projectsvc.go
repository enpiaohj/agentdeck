package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/enpiaohj/agentdeck/internal/projects"
)

// ProjectService 管「用户声明的项目」：还没有产生任何会话、但用户想让它
// 出现在项目列表里的目录。
//
// 为什么需要它：项目列表原本只从会话聚合（projects.Group），一个刚建、
// 还没开过会话的目录在那里是看不见的，于是「新建项目 → 在里面开新会话」
// 这条链路的第一个动作就看不见。声明清单补上这一段。
//
// 边界：声明只写 AgentDeck 自己的 projects.db，不碰任何 agent 的存储。
type ProjectService struct{ a *App }

// Create 声明一个项目并在磁盘上建出目录（「新建」的含义就是这个目录还不
// 存在）。父目录不存在时如实报错——不层层替用户建一路目录，那超出了
// 「新建这个项目」的授权范围。
func (s *ProjectService) Create(path, name string) (string, error) {
	p, err := s.a.declared.Add(path, name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		// 目录没建成：撤掉声明，别留一个指着一片空的项目。
		_ = s.a.declared.Remove(p)
		return "", fmt.Errorf("创建项目目录失败: %w", err)
	}
	s.a.changed("")
	return p, nil
}

// Rename 改展示名。未声明过的路径拒绝——聚合出来的项目名来自目录本身，
// 改它没有意义（也改不动上游）。
func (s *ProjectService) Rename(path, name string) error {
	ok, err := s.a.declared.Has(path)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("只能重命名 AgentDeck 里新建的项目")
	}
	if _, err := s.a.declared.Add(path, name); err != nil {
		return err
	}
	s.a.changed("")
	return nil
}

// Remove 取消声明。目录与里面的文件一概不删——声明只是 AgentDeck 列表里
// 的一行，删它不该动用户的数据。
func (s *ProjectService) Remove(path string) error {
	if err := s.a.declared.Remove(path); err != nil {
		return err
	}
	s.a.changed("")
	return nil
}

// Declared 列出全部声明的项目（不含从会话聚合出来的）。
func (s *ProjectService) Declared() ([]DeclaredProjectView, error) {
	all, err := s.a.declared.All()
	if err != nil {
		return nil, err
	}
	out := make([]DeclaredProjectView, 0, len(all))
	for _, d := range all {
		out = append(out, DeclaredProjectView{Path: d.Path, Name: d.Name})
	}
	return out, nil
}

// Exists 说目录是否已在磁盘上。前端用它区分「新建」与「已存在，只是没会话」。
func (s *ProjectService) Exists(path string) bool {
	p := projects.Normalize(path)
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// DefaultPathFor 给「新建项目」对话框一个初始目录：当前项目目录下的一个
// 新子目录名（跟着当前项目走最自然）；没有当前项目时用主目录下的
// Documents——不往用户主目录根上塞东西，也不猜某个固定的工作区路径。
// 这只是对话框的初始值，用户随时可改；目录不存在时由 Create 建出来。
func (s *ProjectService) DefaultPathFor(current, name string) string {
	if name == "" {
		name = "新项目"
	}
	base := projects.Normalize(current)
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, "Documents")
		} else {
			base = filepath.Dir(s.a.cfg.MarksPath)
		}
	}
	return filepath.Join(base, name)
}

// DeclaredProjectView 是声明项目的视图：只有路径与展示名，别的都还没有。
type DeclaredProjectView struct {
	Path string `json:"path"`
	Name string `json:"name"`
}
