//go:build windows

package updateinstall

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// errCOMUnavailable 表示当前环境下 COM 用不了（初始化失败或 WScript.Shell 取不到）。
var errCOMUnavailable = errors.New("COM 不可用")

// RetargetShortcuts 把仍指向 oldPath 的快捷方式改指向 newPath，返回改动的个数。
//
// 只处理「目标完全等于旧路径」的快捷方式：用户放在别处的、或指向别的程序的，
// 一概不碰。全程尽力而为——改不了的跳过，不影响更新本身。
func RetargetShortcuts(oldPath, newPath string) int {
	return retargetShortcutsIn(shortcutRoots(), oldPath, newPath)
}

// ShortcutsPointingTo 报告是否还有快捷方式指向 targetPath。
//
// 善后删除旧文件前用它复核：改指向是尽力而为的，没改到却照删，用户就会留下
// 一个点不开的死链接。COM 整个用不了时按「可能还有」算——那时改指向本来也没成，
// 正好一起拦住删除。
func ShortcutsPointingTo(targetPath string) bool {
	return anyShortcutPointsTo(shortcutRoots(), targetPath)
}

// AlignShortcutNames 把指向 current 的版本派生快捷方式改成与 current 同名。
//
// 单独于改指向存在，因为「目标已是最新、名字还停在旧版本」是个会自己出现的
// 状态：跨版本手工替换文件、或某次改名失败后重试，都会留下它，而那时 oldPath
// 早就不是「上一版」了，retarget 永远匹配不到。启动时按当前 exe 名对齐一次，
// 这类残留就能自愈，不必等下一次版本变化。
func AlignShortcutNames(current string) int {
	return alignShortcutNamesIn(shortcutRoots(), current)
}

// alignShortcutNamesIn 在给定目录列表里对齐名字。目录可注入，测试因此不碰
// 用户真实的桌面与开始菜单。
func alignShortcutNamesIn(roots []string, current string) int {
	links := findShortcuts(roots)
	if len(links) == 0 {
		return 0
	}
	aligned := 0
	_ = withShell(func(shell *ole.IDispatch) error {
		for _, link := range links {
			target, ok := shortcutTargetOf(shell, link)
			if !ok || !strings.EqualFold(target, current) {
				continue
			}
			if renameShortcutForVersion(link, current) {
				aligned++
			}
		}
		return nil
	})
	return aligned
}

// retargetShortcutsIn 在给定目录列表里改指向。目录可注入，测试因此不碰
// 用户真实的桌面与开始菜单。
func retargetShortcutsIn(roots []string, oldPath, newPath string) int {
	links := findShortcuts(roots)
	if len(links) == 0 {
		return 0
	}
	updated := 0
	_ = withShell(func(shell *ole.IDispatch) error {
		for _, link := range links {
			if retargetShortcut(shell, link, oldPath, newPath) {
				updated++
			}
		}
		return nil
	})
	return updated
}

// anyShortcutPointsTo 与 retargetShortcutsIn 同一套扫描，只读不改。
//
// 读不出结果的**单个**快捷方式跳过（可能是用户自己的坏链接，不该因此永远留着
// 旧 exe），只有 COM 整个用不了才按「可能还有」处理。
func anyShortcutPointsTo(roots []string, targetPath string) bool {
	links := findShortcuts(roots)
	if len(links) == 0 {
		return false
	}
	found := false
	err := withShell(func(shell *ole.IDispatch) error {
		for _, link := range links {
			value, ok := shortcutTargetOf(shell, link)
			if ok && strings.EqualFold(value, targetPath) {
				found = true
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return true
	}
	return found
}

// withShell 在一个绑定到当前 OS 线程的 COM 单元里创建 WScript.Shell 并执行 fn。
//
// **必须锁定线程**：COM 单元是线程本地状态，CoInitializeEx 只作用于调用它的那
// 一个 OS 线程；Go 不保证 goroutine 与线程的亲缘关系。不锁定时，调度器一旦把
// goroutine 挪到别的线程，后续 CreateObject / CallMethod 就会返回
// CO_E_NOTINITIALIZED，而调用方（本包的扫描函数）会把它当成「没有可改的快捷
// 方式」静默返回 0 —— 用户看到的是「更新成功但快捷方式还指向旧版本」。
func withShell(fn func(shell *ole.IDispatch) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	uninit, ok := comReady()
	if !ok {
		return errCOMUnavailable
	}
	defer uninit()
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	return fn(shell)
}

// comReady 在当前线程上准备好 COM，返回一个「用完要不要反初始化」的函数。
//
// 不能直接判断 ole.CoInitializeEx 的返回值：调用方可能已经初始化过 COM，
// 这时它会返回 S_FALSE（成功但未增加计数）或 RPC_E_CHANGED_MODE（单元已被
// 初始化成另一种模型）。两种情况都说明 COM 可用——后者在 AgentDeck 里是常态，
// WebView2 先把主线程初始化成了 MTA。把它们当失败会让快捷方式改指向静默失效，
// 而且不报错，用户只会看到「更新了但快捷方式还是旧的」。
//
// S_FALSE 与 RPC_E_CHANGED_MODE 都不能调 CoUninitialize：这两种情况本次并没有
// 增加初始化计数，多减一次会减掉调用方的计数（同一线程调两次就把计数减穿），
// 之后该线程上的 COM 调用会失败。
func comReady() (func(), bool) {
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_DISABLE_OLE1DDE)
	counted, ok := comOutcome(err)
	if !ok {
		return nil, false
	}
	if counted {
		return ole.CoUninitialize, true
	}
	return func() {}, true
}

// comOutcome 判定 CoInitializeEx 的结果：COM 是否可用，以及本次调用是否增加了
// 引用计数（只有增加过才该调 CoUninitialize）。
//
// 抽成纯函数是为了能直接测——S_FALSE 与 RPC_E_CHANGED_MODE 依赖线程既有的 COM
// 状态，在测试进程里很难稳定构造出来。
func comOutcome(err error) (counted bool, ok bool) {
	const (
		sFalse          = 0x00000001
		rpcEChangedMode = 0x80010106
	)
	if err == nil {
		return true, true
	}
	var oleErr *ole.OleError
	if errors.As(err, &oleErr) {
		switch uint32(oleErr.Code()) {
		case sFalse, rpcEChangedMode:
			// 线程已初始化过，COM 本身可用，本次没有增加计数。
			return false, true
		}
	}
	return false, false
}

// shortcutTargetOf 读一个快捷方式的目标路径；COM 读不动时返回 false。
func shortcutTargetOf(shell *ole.IDispatch, link string) (string, bool) {
	shortcut, err := oleutil.CallMethod(shell, "CreateShortcut", link)
	if err != nil {
		return "", false
	}
	dispatch := shortcut.ToIDispatch()
	if dispatch == nil {
		return "", false
	}
	defer dispatch.Release()
	target, err := oleutil.GetProperty(dispatch, "TargetPath")
	if err != nil {
		return "", false
	}
	value := target.ToString()
	target.Clear()
	return value, true
}

// retargetShortcut 把 link 从 oldPath 改指向 newPath；改成功（或本来就无需改）
// 返回 true。
//
// 除目标与工作目录，还会把**文件名**一并跟随版本：开始菜单「所有应用」显示的是
// .lnk 的文件名，不是它指向的 exe 名。只改目标会留下一个「指向 v0.16.0、名字
// 却写着 v0.15.3」的条目，用户看到的就是版本没更新（实测如此）。
//
// 改名只在两种情况下做：旧名是版本派生的产物名（AgentDeck-v1.2.3-win-x64），
// 且新名在同目录里没被占用。用户自己起的名字（AgentDeck.lnk、我的工作台.lnk）
// 一概不动——那是他选的叫法。
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
	if _, err = oleutil.CallMethod(dispatch, "Save"); err != nil {
		return false
	}
	renameShortcutForVersion(link, newPath)
	return true
}

// renameShortcutForVersion 让版本派生的 .lnk 文件名跟上新版本，返回是否真的改了名。
//
// 失败一律忽略：改指向已经生效，文件名只是显示问题，不值得让整条更新路径
// 因此判定为失败（那会导致旧 exe 永远删不掉）。占用检查与改名之间仍有极短的
// 竞态，此时 os.Rename 自己会失败，同样落到忽略分支。
func renameShortcutForVersion(link, newPath string) bool {
	dir, oldStem := filepath.Dir(link), strings.TrimSuffix(filepath.Base(link), filepath.Ext(link))
	if !VersionedStem(oldStem) {
		return false
	}
	newStem := strings.TrimSuffix(filepath.Base(newPath), filepath.Ext(newPath))
	if !VersionedStem(newStem) || strings.EqualFold(oldStem, newStem) {
		return false
	}
	destination := filepath.Join(dir, newStem+filepath.Ext(link))
	if _, err := os.Stat(destination); err == nil {
		// 已有同名快捷方式（例如上一次改名成功、用户又手工建了一个）：不覆盖。
		return false
	}
	return os.Rename(link, destination) == nil
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
