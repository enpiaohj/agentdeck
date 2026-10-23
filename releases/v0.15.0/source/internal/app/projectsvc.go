package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

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

// Create 声明项目并创建目录；旧接口保留既有项目的默认平台。
func (s *ProjectService) Create(path, name string) (string, error) {
	return s.create(path, name, "", false)
}

// CreateWithAgent 显式设置项目默认平台；空平台表示使用会话统计回退。
func (s *ProjectService) CreateWithAgent(path, name, agent string) (string, error) {
	if agent != "" && !cliAllowed(agent) {
		return "", fmt.Errorf("暂不支持使用此智能体新建会话：%s", agent)
	}
	return s.create(path, name, agent, true)
}

func (s *ProjectService) create(path, name, agent string, setAgent bool) (string, error) {
	p, err := projects.ValidatePath(path)
	if err != nil {
		return "", err
	}
	if name != "" {
		name, err = projectName(name)
		if err != nil {
			return "", err
		}
	}
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if setAgent {
		exists, err := s.a.declared.Has(p)
		if err != nil {
			return "", err
		}
		if exists {
			return "", fmt.Errorf("该目录已添加到项目列表。请通过项目右键菜单新建会话或修改显示名称。")
		}
	}
	// 先建目录：失败时既有声明（名称、平台、时间）必须原样保留。
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", fmt.Errorf("创建项目目录失败: %w", err)
	}
	if setAgent {
		_, err = s.a.declared.AddWithAgent(p, name, agent)
	} else {
		_, err = s.a.declared.Add(p, name)
	}
	if err != nil {
		return "", err
	}
	// 新建只多出一个还没有会话的项目，会话行不变：只通知侧栏刷新。
	s.a.emit(EventProjectsChanged, "")
	return p, nil
}

func projectName(name string) (string, error) {
	if strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("项目显示名称不能包含控制字符")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("项目显示名称不能为空")
	}
	if utf8.RuneCountInString(name) > 200 {
		return "", fmt.Errorf("项目显示名称不能超过 200 个字符")
	}
	return name, nil
}

// Rename 改展示名。未声明过的路径拒绝——聚合出来的项目名来自目录本身，
// 改它没有意义（也改不动上游）。
func (s *ProjectService) Rename(path, name string) error {
	p, err := projects.ValidatePath(path)
	if err != nil {
		return err
	}
	name, err = projectName(name)
	if err != nil {
		return err
	}
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := s.a.declared.Rename(p, name); err != nil {
		return err
	}
	s.a.emit(EventSessionsChanged, "")
	return nil
}

// Remove 取消声明。目录与里面的文件一概不删——声明只是 AgentDeck 列表里
// 的一行，删它不该动用户的数据。
func (s *ProjectService) Remove(path string) error {
	if _, err := projects.ValidatePath(path); err != nil {
		return err
	}
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := s.a.declared.Remove(path); err != nil {
		return err
	}
	s.a.emit(EventSessionsChanged, "")
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
		out = append(out, DeclaredProjectView{Path: d.Path, Name: d.Name, Agent: d.Agent})
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

// DefaultPathFor 给「新建项目」对话框建议当前项目旁的同级目录；
// 没有当前项目时用主目录下的
// Documents——不往用户主目录根上塞东西，也不猜某个固定的工作区路径。
// 这只是对话框的初始值，用户随时可改；目录不存在时由 Create 建出来。
func (s *ProjectService) DefaultPathFor(current, name string) string {
	if name == "" {
		name = "新项目"
	}
	base := projects.Normalize(current)
	if base != "" {
		base = filepath.Dir(base)
	}
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
	Path  string `json:"path"`
	Name  string `json:"name"`
	Agent string `json:"agent"`
}
