package app

import (
	"fmt"

	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// ResumeService 是恢复动作服务:构造/复制恢复命令、在终端打开执行。
type ResumeService struct{ a *App }

// Command 返回恢复命令原文(空串 = 该会话不可恢复,前端标「仅可查看历史」)。
func (s *ResumeService) Command(key string) (string, error) {
	_, byKey, _ := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return "", fmt.Errorf("会话不存在: %s", key)
	}
	return sessionResume(sess), nil
}

// sessionResume 构造恢复命令:快照里已有 Resume 用快照值;为空(如
// ReadOnly 或装配缺口)时按会话现算。
func sessionResume(s sessions.Session) string {
	if s.Retained {
		return ""
	}
	if s.Resume != "" {
		return s.Resume
	}
	return sessions.ResumeCommand(s.Agent, s.ID, s.Cwd)
}

// CopyToClipboard 复制恢复命令。
func (s *ResumeService) CopyToClipboard(key string) error {
	cmd, err := s.Command(key)
	if err != nil {
		return err
	}
	if cmd == "" {
		return fmt.Errorf("该会话不可恢复,仅可查看历史")
	}
	if s.a.cfg.Clipboard == nil {
		return fmt.Errorf("剪贴板能力未装配")
	}
	return s.a.cfg.Clipboard(cmd)
}

// OpenInTerminal 在终端里打开会话原工作目录并执行恢复命令(Windows 上
// 直接启动 wt.exe / powershell.exe,恢复命令经 -EncodedCommand 交付,
// 见 terminalCommand)。WSL 会话不支持直开——只提供复制。
func (s *ResumeService) OpenInTerminal(key string) error {
	_, byKey, _ := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return fmt.Errorf("会话不存在: %s", key)
	}
	if sess.WSL != "" {
		return fmt.Errorf("WSL 会话请在 WSL 终端中手动执行恢复命令")
	}
	cmd, err := s.Command(key)
	if err != nil {
		return err
	}
	if cmd == "" {
		return fmt.Errorf("该会话不可恢复,仅可查看历史")
	}
	name, args := terminalCommand(sess.Cwd, cmd)
	run := s.a.cfg.Runner
	if run == nil {
		// 终端窗口本身就是功能,不能被 CREATE_NO_WINDOW 藏掉:用
		// proc.Terminal(CREATE_NEW_CONSOLE)。这条路里 stdout 不是终端,
		// agent CLI 会因此不上色,resumeEnv 补 FORCE_COLOR 告诉它。
		run = func(name string, args ...string) error {
			c := proc.Terminal(name, args...)
			if env := resumeEnv(); env != nil {
				c.Env = env
			}
			return c.Start()
		}
	}
	return run(name, args...)
}
