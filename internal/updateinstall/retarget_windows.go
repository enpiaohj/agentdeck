//go:build windows

package updateinstall

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// RetargetShortcuts 把仍指向 oldPath 的快捷方式改指向 newPath，返回改动的个数。
//
// 只处理「目标完全等于旧路径」的快捷方式：用户放在别处的、或指向别的程序的，
// 一概不碰。全程尽力而为——改不了的跳过，不影响更新本身。
func RetargetShortcuts(oldPath, newPath string) int {
	return retargetShortcutsIn(shortcutRoots(), oldPath, newPath)
}

// retargetShortcutsIn 在给定目录列表里改指向。目录可注入，测试因此不碰
// 用户真实的桌面与开始菜单。
func retargetShortcutsIn(roots []string, oldPath, newPath string) int {
	links := findShortcuts(roots)
	if len(links) == 0 {
		return 0
	}
	uninit, ok := comReady()
	if !ok {
		return 0
	}
	defer uninit()
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return 0
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return 0
	}
	defer shell.Release()
	updated := 0
	for _, link := range links {
		if retargetShortcut(shell, link, oldPath, newPath) {
			updated++
		}
	}
	return updated
}

// comReady 在当前线程上准备好 COM，返回一个「用完要不要反初始化」的函数。
//
// 不能直接判断 ole.CoInitializeEx 的返回值：调用方可能已经初始化过 COM，
// 这时它会返回 S_FALSE（成功但未增加计数）或 RPC_E_CHANGED_MODE（单元已被
// 初始化成另一种模型）。两种情况都说明 COM 可用——后者在 AgentDeck 里是常态，
// WebView2 先把主线程初始化成了 MTA。把它们当失败会让快捷方式改指向静默失效，
// 而且不报错，用户只会看到「更新了但快捷方式还是旧的」。
//
// RPC_E_CHANGED_MODE 时不能再调 CoUninitialize：本次并没有增加计数。
func comReady() (func(), bool) {
	const (
		sFalse          = 0x00000001
		rpcEChangedMode = 0x80010106
	)
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_DISABLE_OLE1DDE)
	if err == nil {
		return ole.CoUninitialize, true
	}
	var oleErr *ole.OleError
	if errors.As(err, &oleErr) {
		switch uint32(oleErr.Code()) {
		case sFalse:
			return ole.CoUninitialize, true
		case rpcEChangedMode:
			// 线程已处于其他单元，COM 本身可用，直接用。
			return func() {}, true
		}
	}
	return nil, false
}

func retargetShortcut(shell *ole.IDispatch, link, oldPath, newPath string) bool {
	shortcut, err := oleutil.CallMethod(shell, "CreateShortcut", link)
	if err != nil {
		return false
	}
	dispatch := shortcut.ToIDispatch()
	if dispatch == nil {
		return false
	}
	defer dispatch.Release()
	target, err := oleutil.GetProperty(dispatch, "TargetPath")
	if err != nil {
		return false
	}
	value := target.ToString()
	target.Clear()
	if !strings.EqualFold(value, oldPath) {
		return false
	}
	// 只改目标与工作目录，参数（如 --startup）原样保留。
	if _, err = oleutil.PutProperty(dispatch, "TargetPath", newPath); err != nil {
		return false
	}
	_, _ = oleutil.PutProperty(dispatch, "WorkingDirectory", filepath.Dir(newPath))
	_, err = oleutil.CallMethod(dispatch, "Save")
	return err == nil
}

// shortcutRoots 是可能出现本程序快捷方式的位置。
//
// 这些目录里的 .lnk 通常只有几十个，逐个用 COM 读目标代价很小；按文件名
// 粗筛反而会漏——快捷方式的名字与它指向的 exe 名不必相同。
func shortcutRoots() []string {
	var roots []string
	appData := os.Getenv("APPDATA")
	if appData != "" {
		roots = append(roots, filepath.Join(appData, `Microsoft\Internet Explorer\Quick Launch`))
	}
	for _, env := range []string{"USERPROFILE", "PUBLIC"} {
		profile := os.Getenv(env)
		if profile == "" {
			continue
		}
		roots = append(roots,
			filepath.Join(profile, "Desktop"),
			filepath.Join(profile, `AppData\Roaming\Microsoft\Windows\Start Menu\Programs`),
		)
	}
	return roots
}

// findShortcuts 列出 roots 下（含子目录）的全部 .lnk。读不到的目录跳过。
func findShortcuts(roots []string) []string {
	var out []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".lnk") {
				out = append(out, path)
			}
			return nil
		})
	}
	return out
}
