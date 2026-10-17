package app

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/enpiaohj/agentdeck/internal/projects"
)

// terminalCommand 决定用哪个终端启动:优先 wt.exe(带 -d 直接落在会话
// cwd),不可用回退 powershell.exe(恢复命令自带 Set-Location,无需 -d)。
//
// wt 的 -d 收根路径(如 D:\)时尾反斜杠经 Windows 命令行转义规则可能
// 吞掉结尾引号——Go 的 exec 参数转义(syscall.EscapeArg)会把尾反斜杠
// 正确双写,这里保持原样交由 exec 处理,并先用 Stat 验目录存在。
func terminalCommand(cwd, resumeCmd string) (string, []string) {
	dir := projects.Normalize(cwd)
	if dir != "" && filepath.IsAbs(dir) {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			if wt, err := exec.LookPath("wt.exe"); err == nil {
				return wt, []string{"-d", dir, "powershell.exe", "-NoExit", "-Command", resumeCmd}
			}
		}
	}
	return "powershell.exe", []string{"-NoExit", "-Command", resumeCmd}
}
