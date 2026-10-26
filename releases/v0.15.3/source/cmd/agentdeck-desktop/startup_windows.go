package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// retarget 把自启动项里指向 oldPath 的路径换成 newPath，返回是否确认「改完之后
// 不再有指向旧路径的启动项」。
//
// 应用内更新会让可执行文件换名（AgentDeck-v0.14.0 → v0.15.0），自启动项仍写着
// 旧路径，不跟着改就会在下次开机时启动一个已经不存在的文件。返回值用于善后：
// 只有确认改完，才敢删掉旧文件——读不到注册表或写失败时返回 false，宁可把旧
// 文件多留一个版本，也不要留下一个开机启动一个不存在文件的项。
func (r startupRegistration) retarget(oldPath, newPath string) bool {
	current, err := r.value.read()
	if err != nil {
		return false
	}
	// 读得到但为空：用户没开自启动，不存在指向旧路径的项。
	if current == "" || !strings.Contains(strings.ToLower(current), strings.ToLower(oldPath)) {
		return true
	}
	return r.value.write(strings.ReplaceAll(current, oldPath, newPath)) == nil
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
