package browsers

import (
	"errors"

	"github.com/enpiaohj/agentdeck/internal/proc"
)

// Launch opens link in the browser at exe. A browser that is already running
// opens it in a new tab of its last-used window (of its last-used profile —
// which profile that is cannot be chosen from here).
func Launch(exe, link string) error {
	if err := ValidURL(link); err != nil {
		return err
	}
	if !absExe(exe) {
		return errors.New("浏览器的路径无效")
	}
	if !exists(exe) {
		return errors.New("找不到所选浏览器的程序文件")
	}
	cmd := proc.Command(exe, link)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap it; the browser outlives this call
	return nil
}
