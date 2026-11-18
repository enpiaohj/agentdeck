package app

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/enpiaohj/agentdeck/internal/projects"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Open 在资源管理器中打开有效项目目录，保留原项目身份。
func (s *ProjectService) Open(original string) error {
	dir, err := (&ResumeService{a: s.a}).projectDirectory(original, "")
	if err != nil {
		return err
	}
	command := "explorer"
	if runtime.GOOS == "darwin" {
		command = "open"
	} else if runtime.GOOS != "windows" {
		command = "xdg-open"
	}
	if s.a.cfg.Runner != nil {
		return s.a.cfg.Runner(command, dir)
	}
	return proc.Command(command, dir).Start()
}

// Location 查询当前有效位置；保持项目筛选、置顶和会话原始 cwd 不变。
func (s *ProjectService) Location(original string) (string, error) {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	return s.a.declared.Location(original)
}

func (s *ProjectService) SetLocation(original, target string) error {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := s.a.declared.SetLocation(original, target); err != nil {
		return err
	}
	s.a.emit(EventProjectsChanged, "")
	return nil
}

func (s *ProjectService) RemoveLocation(original string) error {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := s.a.declared.RemoveLocation(original); err != nil {
		return err
	}
	s.a.emit(EventProjectsChanged, "")
	return nil
}

func (s *ProjectService) Locations() ([]projects.Location, error) {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	return s.a.declared.Locations()
}

func (s *ResumeService) projectDirectory(original, override string) (string, error) {
	if override != "" {
		return existingDir(override)
	}
	s.a.dataMu.Lock()
	target, err := s.a.declared.Location(original)
	s.a.dataMu.Unlock()
	if err != nil {
		return "", err
	}
	return existingDir(target)
}

type ResumePreview struct {
	Original      string `json:"original"`
	Directory     string `json:"directory"`
	Command       string `json:"command"`
	NeedsLocation bool   `json:"needsLocation"`
	Notice        string `json:"notice"`
}

// Prepare 与执行共用最终目录和命令构造；不保存关联，也不启动进程。
func (s *ResumeService) Prepare(key, override string) (ResumePreview, error) {
	_, byKey, ms := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return ResumePreview{}, fmt.Errorf("会话不存在，请刷新会话列表")
	}
	out := ResumePreview{Original: sess.Cwd}
	if sessionResume(sess) == "" {
		return out, fmt.Errorf("此会话仅支持查看历史记录")
	}
	if sess.WSL != "" {
		if override != "" {
			return out, fmt.Errorf("WSL 会话不支持更改项目位置，请在 WSL 终端中恢复会话")
		}
		out.Directory, out.Command = sess.Cwd, sessionResume(sess)
		return out, nil
	}
	if sess.Cwd == "" && override == "" {
		out.Command = sessions.ResumeCommandAt(sess, "")
		if out.Command == "" {
			return out, fmt.Errorf("无法构造恢复命令，请检查会话信息")
		}
		out.Command += resumeName(sess, ms[s.a.markKey(key)].Title)
		out.NeedsLocation = true
		out.Notice = "此会话未记录工作目录。可直接复制按会话 ID 恢复的命令；打开终端前请选择工作目录。"
		return out, nil
	}
	target := override
	if target == "" {
		s.a.dataMu.Lock()
		var err error
		target, err = s.a.declared.Location(sess.Cwd)
		s.a.dataMu.Unlock()
		if err != nil {
			return out, err
		}
	}
	out.Directory = target
	dir, err := existingDir(target)
	if err != nil {
		if override != "" {
			return out, err
		}
		out.NeedsLocation = true
		return out, nil
	}
	out.Directory = dir
	if !strings.EqualFold(projects.Normalize(sess.Cwd), dir) && (sess.Agent == "cursor" || sess.Agent == "cursor-agent") {
		out.Notice = "Cursor 按项目目录查找会话，暂不支持跨目录恢复。请选择原项目目录，或在其他目录新建会话。"
		if override == "" {
			out.NeedsLocation = true
			return out, nil
		}
		return out, fmt.Errorf("%s", out.Notice)
	}
	out.Command = sessions.ResumeCommandAt(sess, dir)
	if out.Command == "" {
		return out, fmt.Errorf("无法构造恢复命令，请检查会话信息")
	}
	out.Command += resumeName(sess, ms[s.a.markKey(key)].Title)
	return out, nil
}

func (s *ResumeService) CopyAt(key, override string) error {
	preview, err := s.Prepare(key, override)
	if err != nil {
		return err
	}
	if preview.NeedsLocation && preview.Command == "" {
		return fmt.Errorf("项目位置不可用，请选择新项目位置")
	}
	if s.a.cfg.Clipboard == nil {
		return fmt.Errorf("剪贴板不可用，请手动复制预览命令")
	}
	return s.a.cfg.Clipboard(preview.Command)
}
