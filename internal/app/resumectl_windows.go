package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"unicode/utf16"

	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/enpiaohj/agentdeck/internal/projects"
)

// terminalCommand 决定用哪个终端启动,以及恢复命令怎么交给它。
//
// 一、恢复命令不能被拆开。恢复命令是 `<Set-Location ...>; <agent> resume
// <id>` 这样的复合语句。把它当 -Command 的值交给 wt.exe 时,wt 会重新
// 解析自己的命令行、按空格拆参数,分号后的半句被当成另一条参数丢掉,
// 终端开在正确目录却报「系统找不到指定的文件」(启动 "" codex resume
// <id>"" 时,0x80070002)。所以脚本一律编码成 UTF-16LE base64 交给
// -EncodedCommand:编码后的参数不含空格、分号与引号,谁也拆不开。
//
// 二、窗口必须真的出现在屏幕上。AgentDeck 是 -H windowsgui 的 GUI 程序,
// 自身没有控制台。试过 `cmd.exe /c start "" /D <cwd> powershell.exe ...`
// (含 CREATE_NEW_CONSOLE / CREATE_NO_WINDOW / DETACHED_PROCESS 三种标志):
// 子进程的 isTTY 确实为 true,但 start 建出的控制台窗口不可见,用户什么
// 都看不到。所以仍然直接以 CREATE_NEW_CONSOLE 启动终端程序(proc.Terminal),
// 这是实测唯一稳定出窗的路径。
//
// 三、代价是这条路里 stdout 不是终端(node 的 isTTY 实测为 false),
// 而 agent CLI 判断「要不要上色」正是看这个——恢复出来的会话没有颜色。
// 因此显式给子进程加 FORCE_COLOR=1:把「这是终端」告诉 CLI,不依赖它
// 自己探测控制台。仅当调用方环境里没有设 FORCE_COLOR 时才加,用户自己
// 的决定优先。
//
// 四、有 wt.exe 时用它(-d 直接落在会话目录);没有就回退 shell 本身
// (恢复命令自带 Set-Location)。两条路的脚本都是 -EncodedCommand。
//
// 五、shell 由设置决定(见 terminalshell):默认 Windows PowerShell 5.1,
// 可选 PowerShell 7+ 或自定义可执行文件。wt 的内层命令与回退进程都用
// 解析出的同一个可执行文件。
func terminalCommand(cwd, resumeCmd, shellExe string) (string, []string) {
	script := encodedCommand(resumeCmd)
	dir := projects.Normalize(cwd)
	if dir != "" && filepath.IsAbs(dir) {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			dir = "" // 目录不在了:仍然恢复会话,只是不切换工作目录
		}
	} else {
		dir = ""
	}
	if wt, err := proc.LookPath("wt.exe"); err == nil && dir != "" {
		return wt, []string{"-d", dir, shellExe, "-NoExit", "-EncodedCommand", script}
	}
	return shellExe, []string{"-NoExit", "-EncodedCommand", script}
}

// encodedCommand 把整条恢复命令编码成 powershell -EncodedCommand 收的
// UTF-16LE base64。原因见 terminalCommand 的第一条:复合语句交给任何
// 会重新解析命令行的中间层(wt、cmd start)都会被按空格拆开。
//
// 用 -EncodedCommand 而不是给 -Command 补引号:后者要再叠一层中间层的
// 转义规则,而转义规则本身没有文档保证。
func encodedCommand(script string) string {
	units := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(units)*2)
	for _, u := range units {
		buf = append(buf, byte(u), byte(u>>8))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// resumeEnv 是终端里跑恢复命令时的环境:在父进程环境上补 FORCE_COLOR。
// 空切片表示不改,调用方沿用默认的继承环境。
func resumeEnv() []string {
	if _, set := os.LookupEnv("FORCE_COLOR"); set {
		return nil
	}
	return append(os.Environ(), "FORCE_COLOR=1")
}
