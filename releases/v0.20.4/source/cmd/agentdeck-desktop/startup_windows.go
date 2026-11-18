package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/enpiaohj/agentdeck/internal/updateinstall"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const startupKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupValueName = "AgentDeck"

type startupValue interface {
	read() (string, error)
	write(string) error
	remove() error
}

type startupRegistration struct {
	value      startupValue
	executable func() (string, error)
}

func (r startupRegistration) enabled() (bool, error) {
	v, err := r.value.read()
	return strings.TrimSpace(v) != "", err
}

func (r startupRegistration) set(enabled bool) error {
	if !enabled {
		return r.value.remove()
	}
	exe, err := r.executable()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(exe) || strings.ContainsAny(exe, "\"\x00\r\n") {
		return fmt.Errorf("无效的程序路径")
	}
	return r.value.write(`"` + exe + `" ` + startupFlag)
}

type windowsStartupValue struct{}

func (windowsStartupValue) read() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, startupKey, registry.QUERY_VALUE)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(startupValueName)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return "", nil
	}
	return v, err
}

func (windowsStartupValue) write(value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, startupKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(startupValueName, value)
}

func (windowsStartupValue) remove() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, startupKey, registry.SET_VALUE)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	err = k.DeleteValue(startupValueName)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	return err
}

func desktopStartup() startupRegistration {
	return startupRegistration{value: windowsStartupValue{}, executable: os.Executable}
}

// retarget 把自启动项里指向旧路径的路径换成 newPath，返回是否确认「改完之后
// 不再有指向旧路径的启动项」。
//
// 应用内更新会让可执行文件换名（AgentDeck-v0.14.0 → v0.15.0），自启动项仍写着
// 旧路径，不跟着改就会在下次开机时启动一个已经不存在的文件。返回值用于善后：
// 只有确认改完，才敢删掉旧文件——读不到注册表或写失败时返回 false，宁可把旧
// 文件多留一个版本，也不要留下一个开机启动一个不存在文件的项。
//
// 只比对 oldPath 是不够的：跨版本手工替换文件、或中途失败又重来的更新，会让
// 启动项停在更早的版本名上，此后每次 oldPath 都是「上一版」，永远匹配不上，
// 于是永远没人修（本机实测停在 v0.13.0，而那个文件早已不在）。所以额外处理
// 一种情况：指向本产品同目录下的旧产物、且那个文件已经不存在。指向用户自己
// 配置的其他程序、或指向仍然存在的另一份副本，一概不碰。
func (r startupRegistration) retarget(oldPath, newPath string) bool {
	current, err := r.value.read()
	if err != nil {
		return false
	}
	// 读得到但为空：用户没开自启动，不存在指向旧路径的项。
	if current == "" {
		return true
	}
	if strings.Contains(strings.ToLower(current), strings.ToLower(oldPath)) {
		return r.value.write(strings.ReplaceAll(current, oldPath, newPath)) == nil
	}
	if staleStartupTarget(current, newPath) {
		// 重写整条命令而不是替换子串：旧路径可能出现在带引号的参数里。
		// --startup 是自启动专用标记，用户手动改过参数的情况不在自愈范围内。
		return r.value.write(`"`+newPath+`" `+startupFlag) == nil
	}
	return true
}

// healStaleTarget 在启动时把「指向本产品同目录旧产物、且该文件已不存在」的
// 自启动项改指向 current。
//
// 与 retarget 分开是因为触发条件不同：retarget 只在版本变化时被调用（拿得到
// oldPath），而停更几个版本的启动项在之后的每次更新里都比对不上，永远等不到
// 属于自己的那一次。这里每次启动检查一次，代价是一次注册表读。
//
// 只修确定坏掉的项（文件不存在），不碰仍然可用的配置：用户可能有意指向另一份
// 副本。失败静默——自启动项不是本次启动的必要条件，为此弹错只会打扰用户。
func healStaleTarget(current string) {
	r := desktopStartup()
	command, err := r.value.read()
	if err != nil || command == "" {
		return
	}
	if staleStartupTarget(command, current) {
		_ = r.value.write(`"` + current + `" ` + startupFlag)
	}
}

// staleStartupTarget 报告启动项里的程序是否「本产品同目录下的旧产物、且文件
// 已经不存在」——那种项开机必然失败，改指向新版本只有好处。
func staleStartupTarget(command, newPath string) bool {
	stale := startupExePath(command)
	if stale == "" || strings.EqualFold(stale, newPath) {
		return false
	}
	// 只认同目录下的发布产物名：用户放在别处、或自己起的名字，不替他改。
	if !strings.EqualFold(filepath.Dir(stale), filepath.Dir(newPath)) ||
		!updateinstall.ReleaseArtifact.MatchString(filepath.Base(stale)) {
		return false
	}
	_, err := os.Stat(stale)
	return errors.Is(err, fs.ErrNotExist)
}

// startupExePath 从启动项的命令行里取出可执行文件路径。
//
// 本程序写进去的是 `"<exe>" --startup`，但注册表里可能是别人或旧版本写的
// 任意形式，所以两种都认：带引号取引号内，不带引号取第一个空白之前。
func startupExePath(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	if strings.HasPrefix(command, `"`) {
		if end := strings.Index(command[1:], `"`); end >= 0 {
			return command[1 : end+1]
		}
		return ""
	}
	if i := strings.IndexAny(command, " \t"); i > 0 {
		return command[:i]
	}
	return command
}

const activationEventName = `Local\AgentDeck-ActivateWindow`

// 在已有实例中调用 Wails Show，确保隐藏的 WebView 也被恢复。
func activateExistingWindow() {
	notifyActivation(activationEventName)
}

func notifyActivation(eventName string) {
	name, _ := windows.UTF16PtrFromString(eventName)
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.SetEvent(h)
}

func listenForActivation(eventName string, open func()) (func(), error) {
	name, _ := windows.UTF16PtrFromString(eventName)
	h, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		return nil, err
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			result, err := windows.WaitForMultipleObjects([]windows.Handle{stop, h}, false, windows.INFINITE)
			if err != nil || result != windows.WAIT_OBJECT_0+1 {
				return
			}
			open()
		}
	}()
	return func() {
		_ = windows.SetEvent(stop)
		<-done
		windows.CloseHandle(stop)
		windows.CloseHandle(h)
	}, nil
}
