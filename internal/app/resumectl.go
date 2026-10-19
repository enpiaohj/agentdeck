package app

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/enpiaohj/agentdeck/internal/projects"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// ResumeService 是恢复动作服务:构造/复制恢复命令、在终端打开执行。
type ResumeService struct{ a *App }

// Command 返回恢复命令原文(空串 = 该会话不可恢复,前端标「仅可查看历史」)。
func (s *ResumeService) Command(key string) (string, error) {
	_, byKey, ms := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return "", fmt.Errorf("会话不存在: %s", key)
	}
	cmd := sessionResume(sess)
	if cmd == "" {
		return "", nil
	}
	return cmd + resumeName(sess, ms[s.a.markKey(key)].Title), nil
}

// resumeName 是恢复命令里把 AgentDeck 的会话名交给 agent CLI 的那一段。
//
// Claude Code 的 `--resume <id> --name <名>` 实测会把名字写进**该会话自己**
// 的记录(JSONL 里的 custom-title/customTitle,同时写 agent-name),不新建
// 会话,CLI 内部因此显示 AgentDeck 里的名字。这是 CLI 自己的写入口,不是
// AgentDeck 去改上游文件。
//
// 只对 claude 加:其余 Claude 系 CLI(qoder/grok/omp/droid/cursor…)是否收
// 这个参数没有实测,乱加会让恢复直接失败。WSL 会话的命令是整条
// wsl.exe -e sh -lc '…',参数得塞进内层引号里,这里不做,保持原样。
func resumeName(sess sessions.Session, title string) string {
	if sess.Agent != "claude" || sess.WSL != "" || title == "" {
		return ""
	}
	return " --name " + quoteShellArg(title)
}

// quoteShellArg 按恢复命令所在 shell 的规则加引号:Windows 上恢复命令交给
// PowerShell(单引号串里只有单引号要翻倍),其余平台是 sh(单引号按 POSIX 的
// 转义写法拆开)。标题已由 marks 校验过(无换行与控制符),这里只需处理引号。
func quoteShellArg(s string) string {
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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

// NewSession 在指定项目目录里开一个**新的** agent 会话：不挑历史会话，
// 直接起该 agent 的 CLI，由它自己建会话。这正是「新建会话」——AgentDeck
// 不写任何上游存储，只是把用户「在这个项目里开一个」的决定交给 CLI
// （与自己敲 `claude` 等价）。
//
// agent 为空时先取项目平台，再按可启动的本机会话统计回退；无记录时用 claude。
// 返回的错误自解释，与 OpenInTerminal 同一套措辞（前端不套前缀）。
func (s *ResumeService) NewSession(project, agent string) error {
	dir, err := projects.ValidatePath(project)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("项目目录不存在: %s", dir)
	}
	if agent == "" {
		declared, err := s.a.declared.All()
		if err != nil {
			return err
		}
		for _, p := range declared {
			if strings.EqualFold(p.Path, dir) {
				agent = p.Agent
				break
			}
		}
		if agent == "" {
			agent = s.defaultAgent()
		}
	}
	cli := cliCommand(agent)
	if cli == "" {
		return fmt.Errorf("暂不支持使用此智能体新建会话：%s", agent)
	}
	// LiteralPath 保留方括号等字符；切目录失败时禁止在错误目录启动 CLI。
	script := fmt.Sprintf("Set-Location -LiteralPath %s -ErrorAction Stop; %s", quotePowerShellPath(dir), cli)
	return s.launchInTerminal(dir, script)
}

// defaultAgent 是本机会话数最多的 agent，用作「新建会话」的缺省目标。
func (s *ResumeService) defaultAgent() string {
	list, _, _ := s.a.snapshot()
	counts := map[string]int{}
	for _, sess := range list {
		if sess.WSL != "" || sess.Gateway || sess.Imported || sess.ReadOnly || sess.Retained || !cliAllowed(sess.Agent) {
			continue
		}
		counts[sess.Agent]++
	}
	best, bestN := "", 0
	for agent, n := range counts {
		if n > bestN || (n == bestN && agent < best) {
			best, bestN = agent, n
		}
	}
	if best == "" {
		return "claude"
	}
	return best
}

// cliCommand 将会话的平台标识映射为固定命令，禁止把任意输入当命令执行。
func cliCommand(agent string) string {
	switch agent {
	case "qoder":
		return "qodercli"
	case "qoder-cn":
		return "qoderclicn"
	case "cursor", "cursor-agent":
		return "cursor-agent"
	case "claude", "codex", "zcode", "opencode", "grok", "droid", "hermes":
		return agent
	}
	return ""
}

func cliAllowed(agent string) bool { return cliCommand(agent) != "" }

// quotePowerShellPath 给 PowerShell 的 Set-Location 引路径。与恢复命令
// 不同，这里整条脚本交给 -EncodedCommand（见 terminalCommand），不会
// 被中间层拆开，所以只需 PowerShell 自己的引号规则。
func quotePowerShellPath(p string) string {
	return "'" + strings.ReplaceAll(p, "'", "''") + "'"
}

// OpenInTerminal 在终端里打开会话原工作目录并执行恢复命令(Windows 上
// 直接启动 wt.exe / powershell.exe,恢复命令经 -EncodedCommand 交付,
// 见 terminalCommand)。WSL 会话不支持直开——只提供复制。
//
// 返回的每条错误消息都自解释(前端对这条路径不再套「操作失败：」前缀),
// 因为其中一条是「终端已经开出来了,只是附带动作失败」,扣「操作失败」的
// 帽子与事实相反。
func (s *ResumeService) OpenInTerminal(key string) error {
	_, byKey, ms := s.a.snapshot()
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
	// Codex 没有把名字带进恢复命令的入口(claude 那种 --name),只能在开终端
	// 之前用它自己的 app-server 写一次原生名。失败不阻断恢复——终端照开,
	// 但错误必须如实报出去:改了名却仍是旧名,比直接报错更难排查。
	var nameErr error
	renamed := false
	if title := ms[s.a.markKey(key)].Title; needsCodexName(sess, title) {
		nameErr = s.a.codexName(sess.ID, title)
		renamed = nameErr == nil
	}
	name, args := terminalCommand(sess.Cwd, cmd)
	run := s.a.cfg.Runner
	if run == nil {
		run = terminalLauncher()
	}
	if err := run(name, args...); err != nil {
		if renamed {
			// 名字已经写进 Codex 自己的索引了,不说的话用户会以为整件事没发生。
			return fmt.Errorf("Codex 原生名称已同步，但打开终端失败：%w", err)
		}
		return fmt.Errorf("打开终端失败：%w", err)
	}
	if nameErr != nil {
		// 只能说「可能」:没收到答复不等于没写成功(codex 可能已经落盘,只是
		// 我们没读到那一行),字面断言「仍是旧名」会把不确定说成事实。
		return fmt.Errorf("已在终端中打开会话，但同步 Codex 原生名称失败（CLI 里可能仍是旧名）：%w", nameErr)
	}
	return nil
}

// terminalLauncher 是真实启动终端的那一个 runner:CREATE_NEW_CONSOLE 出窗
// (终端窗口本身就是功能,不能被 CREATE_NO_WINDOW 藏掉),并补 FORCE_COLOR
// 让 agent CLI 在这条 stdout 不是终端的路上仍然上色。
func terminalLauncher() func(name string, args ...string) error {
	return func(name string, args ...string) error {
		c := proc.Terminal(name, args...)
		if env := resumeEnv(); env != nil {
			c.Env = env
		}
		return c.Start()
	}
}

// launchInTerminal 在 dir 里执行 script。NewSession 与 OpenInTerminal 的
// 后半段共用它——终端选谁、命令怎么交付、失败怎么措辞，只有一处。
func (s *ResumeService) launchInTerminal(dir, script string) error {
	name, args := terminalCommand(dir, script)
	run := s.a.cfg.Runner
	if run == nil {
		run = terminalLauncher()
	}
	if err := run(name, args...); err != nil {
		return fmt.Errorf("打开终端失败：%w", err)
	}
	return nil
}
