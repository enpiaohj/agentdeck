package proc

import (
	"os/exec"
	"syscall"
)

// AgentDeck 改动:本文件为桌面壳新增(上游 magpie 无)。桌面壳的「在终端
// 打开」恢复动作必须让用户看到终端窗口,而 Command 的 hide 会给无控制台
// 父进程的子进程加 CREATE_NO_WINDOW——回退的 powershell.exe 将在后台无窗
// 启动,功能失效。Terminal 以 CREATE_NEW_CONSOLE 启动:console 程序
// (powershell.exe)得到自己的新窗口;GUI 程序(wt.exe)本就不受控制台
// 标志影响,窗口照常出现。其余组装逻辑与 Command 一致(batch 转义)。

const createNewConsole = 0x00000010 // CREATE_NEW_CONSOLE

// Terminal is exec.Command with a new console of its own on Windows, so
// the user sees and interacts with the program: the desktop shell's
// "open in terminal" resume action, where the window is the point.
func Terminal(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	batch(cmd)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNewConsole
	return cmd
}
