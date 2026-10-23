package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	_, byKey, _ := s.a.snapshot()
	if sess, ok := byKey[key]; ok && sessionResume(sess) == "" {
		return "", nil
	}
	preview, err := s.Prepare(key, "")
	if err != nil {
		return "", err
	}
	if preview.NeedsLocation && preview.Command == "" {
		return "", fmt.Errorf("项目位置不可用，请选择新项目位置")
	}
	return preview.Command, nil
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
	dir, err := s.projectDirectory(project, "")
	if err != nil {
		return err
	}
	agent, err = s.resolveAgent(projects.Normalize(project), agent)
	if err != nil {
		return err
	}
	return s.launchInTerminal(dir, newSessionScript(dir, cliCommand(agent)))
}

// NewSessionAt 使用已明确选择的目录，不对目标再次应用位置关联。
func (s *ResumeService) NewSessionAt(project, target, agent string) error {
	dir, err := existingDir(target)
	if err != nil {
		return err
	}
	agent, err = s.resolveAgent(projects.Normalize(project), agent)
	if err != nil {
		return err
	}
	return s.launchInTerminal(dir, newSessionScript(dir, cliCommand(agent)))
}

func (s *ResumeService) PreviewCommandAt(project, target, agent string) (string, error) {
	dir, err := existingDir(target)
	if err != nil {
		return "", err
	}
	agent, err = s.resolveAgent(projects.Normalize(project), agent)
	if err != nil {
		return "", err
	}
	return newSessionScript(dir, cliCommand(agent)), nil
}

// resolveAgent 定下这次要启动谁：显式指定 > 项目保存的默认智能体 > 本机会话
// 统计回退。两种入口（新建会话 / 预览命令）共用，避免两条路各算一遍。
func (s *ResumeService) resolveAgent(dir, agent string) (string, error) {
	if agent == "" {
		declared, err := s.a.declared.All()
		if err != nil {
			return "", err
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
	if cliCommand(agent) == "" {
		return "", fmt.Errorf("暂不支持使用此智能体新建会话：%s", agent)
	}
	return agent, nil
}

// newSessionScript 是在 dir 里启动 cli 的脚本。LiteralPath 保留方括号等字符，
// 切目录失败时禁止在错误目录启动 CLI。
func newSessionScript(dir, cli string) string {
	return fmt.Sprintf("Set-Location -LiteralPath %s -ErrorAction Stop; %s", quotePowerShellPath(dir), cli)
}

// CliAvailability 是一个可启动平台在本机的情况：命令名与是否能在 PATH 上找到。
type CliAvailability struct {
	Agent   string `json:"agent"`
	Command string `json:"command"`
	Found   bool   `json:"found"`
}

// Available 报告各平台的启动命令是否已安装。前端用它标注「未检测到命令」，
// 免得用户点了新建会话才发现终端里没有这个 CLI。
func (s *ResumeService) Available() []CliAvailability {
	out := make([]CliAvailability, 0, len(cliAgents))
	for _, agent := range cliAgents {
		cmd := cliCommand(agent)
		if cmd == "" {
			continue
		}
		_, err := exec.LookPath(cmd)
		out = append(out, CliAvailability{Agent: agent, Command: cmd, Found: err == nil})
	}
	return out
}

// PreviewCommand 返回新会话会交给终端的命令原文，不启动任何进程。
// 终端启动失败时前端可以用它做「复制命令」兜底。
//
// 校验与 NewSession 完全一致（含目录必须已存在）：预览说能开、真开却失败，
// 是比没有预览更坏的结果，所以这条路径不做任何放宽。
func (s *ResumeService) PreviewCommand(project, agent string) (string, error) {
	dir, err := s.projectDirectory(project, "")
	if err != nil {
		return "", err
	}
	agent, err = s.resolveAgent(projects.Normalize(project), agent)
	if err != nil {
		return "", err
	}
	return newSessionScript(dir, cliCommand(agent)), nil
}

// DirectoryState 报告一个路径当前的状态，供前端在输入时即时反馈：
// "dir"（已是目录）、"missing"（不存在；新建项目时会创建）、"file"（同名
// 文件挡着，两种用法都不行）。路径本身不合法（空、相对、含控制字符）报错。
//
// 只回答「现在是什么」，不替前端判断该不该拦：目录不存在对新建项目是正常的，
// 对新建会话才是错误，取舍留给调用方。
func (s *ResumeService) DirectoryState(project string) (string, error) {
	dir, err := projects.ValidatePath(project)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", nil
		}
		return "", fmt.Errorf("无法访问该路径：%s", dir)
	}
	if !fi.IsDir() {
		return "file", nil
	}
	return "dir", nil
}

// existingDir 校验并确认路径是一个已存在的目录：新建会话与预览共用，
// 保证两条路对「这个目录能不能开」的判断不会分叉。
func existingDir(project string) (string, error) {
	dir, err := projects.ValidatePath(project)
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("项目目录不存在: %s", dir)
	}
	return dir, nil
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

// cliAgents 是可启动平台的有序清单（前端选择列表的同一顺序）。cursor 用
// 规范名，cursor-agent 作为历史别名继续接受。dsh 只接「新建会话」：它的
// 续会话命令形态没有可靠依据，恢复仍走「仅可查看历史」（见 sessions.resumeCommand）。
var cliAgents = []string{"claude", "codex", "zcode", "opencode", "qoder", "qoder-cn", "grok", "droid", "cursor", "hermes", "dsh"}

// cliCommand 将会话的平台标识映射为固定命令，禁止把任意输入当命令执行。
func cliCommand(agent string) string {
	switch agent {
	case "qoder":
		return "qodercli"
	case "qoder-cn":
		return "qoderclicn"
	case "cursor", "cursor-agent":
		return "cursor-agent"
	case "claude", "codex", "zcode", "opencode", "grok", "droid", "hermes", "dsh":
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
	return s.OpenAt(key, "")
}

// ResumeOpenResult 区分终端启动成功与附带名称同步警告，避免用户重试时重复启动。
type ResumeOpenResult struct {
	Warning string `json:"warning"`
}

type resumeStartedError struct{ error }

func (s *ResumeService) OpenAtResult(key, override string) (ResumeOpenResult, error) {
	err := s.OpenAt(key, override)
	var started *resumeStartedError
	if errors.As(err, &started) {
		return ResumeOpenResult{Warning: started.Error()}, nil
	}
	return ResumeOpenResult{}, err
}

// OpenAt 支持仅本次指定目录，执行前重新校验，不回退至其他位置。
func (s *ResumeService) OpenAt(key, override string) error {
	_, byKey, ms := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return fmt.Errorf("会话不存在: %s", key)
	}
	if sess.WSL != "" {
		return fmt.Errorf("WSL 会话请在 WSL 终端中手动执行恢复命令")
	}
	preview, err := s.Prepare(key, override)
	if err != nil {
		return err
	}
	if preview.NeedsLocation {
		return fmt.Errorf("项目位置不可用，请选择新的项目目录")
	}
	cmd := preview.Command
	// shell 明确定义在设置里，解析失败要在任何附带动作之前报出来：不能先
	// 改了 Codex 原生名再因为终端起不来而整件事半途而废。
	shellExe, err := s.a.shellExe()
	if err != nil {
		return err
	}
	// Codex 没有把名字带进恢复命令的入口(claude 那种 --name),只能在开终端
	// 之前用它自己的 app-server 写一次原生名。失败不阻断恢复——终端照开,
	// 但错误必须如实报出去:改了名却仍是旧名,比直接报错更难排查。
	var nameErr error
	renamed := false
	if title := ms[s.a.markKey(key)].Title; needsCodexName(sess, title) && !codexNameCurrent(sess.ID, title) {
		nameErr = s.a.codexName(sess.ID, title)
		renamed = nameErr == nil
	}
	if _, err := existingDir(preview.Directory); err != nil {
		return err
	}
	name, args := terminalCommand(preview.Directory, cmd, shellExe)
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
		return &resumeStartedError{fmt.Errorf("已在终端中打开会话，但同步 Codex 原生名称失败（命令行工具中可能仍显示原名称）：%w", nameErr)}
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
	shellExe, err := s.a.shellExe()
	if err != nil {
		return err
	}
	if _, err := existingDir(dir); err != nil {
		return err
	}
	name, args := terminalCommand(dir, script, shellExe)
	run := s.a.cfg.Runner
	if run == nil {
		run = terminalLauncher()
	}
	if err := run(name, args...); err != nil {
		return fmt.Errorf("打开终端失败：%w", err)
	}
	return nil
}
