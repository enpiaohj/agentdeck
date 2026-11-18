package browsers

import (
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Detect lists the installed browsers, default first (see Order).
func Detect() []Browser {
	list := installed()
	if len(list) == 0 {
		return nil
	}
	if d := defaultID(); d != "" {
		for i := range list {
			list[i].Default = list[i].ID == d || strings.Contains(d, list[i].ID)
		}
	}
	running, chatgpt := windowsOf()
	for i := range list {
		list[i].Running = running[list[i].ID]
		list[i].ChatGPT = chatgpt[list[i].ID]
	}
	Order(list)
	return list
}

// installed reads the registry's list of browsers ("Clients\StartMenuInternet"):
// each has a display name and an open command naming its executable.
func installed() []Browser {
	var out []Browser
	seen := map[string]bool{}
	for _, loc := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Clients\StartMenuInternet`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Clients\StartMenuInternet`},
		{registry.CURRENT_USER, `SOFTWARE\Clients\StartMenuInternet`},
	} {
		parent, err := registry.OpenKey(loc.root, loc.path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, _ := parent.ReadSubKeyNames(-1)
		parent.Close()
		for _, sub := range names {
			exe := ExePath(regString(loc.root, loc.path+`\`+sub+`\shell\open\command`, ""))
			if exe == "" || !exists(exe) {
				continue
			}
			id := IDOf(exe)
			if id == "" || id == "iexplore" || seen[id] {
				continue
			}
			name := regString(loc.root, loc.path+`\`+sub, "")
			if name == "" {
				name = sub
			}
			seen[id] = true
			out = append(out, Browser{ID: id, Name: name, Path: exe})
		}
	}
	return out
}

func regString(root registry.Key, path, name string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}

// defaultID names the default browser for https links as a browser id. The
// system keeps a "ProgId" (ChromeHTML, MSEdgeHTM, FirefoxURL-…), not an exe.
func defaultID() string {
	prog := strings.ToLower(regString(registry.CURRENT_USER,
		`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice`, "ProgId"))
	switch {
	case prog == "":
		return ""
	case strings.HasPrefix(prog, "chromehtml"):
		return "chrome"
	case strings.HasPrefix(prog, "msedgehtm"):
		return "msedge"
	case strings.HasPrefix(prog, "firefox"):
		return "firefox"
	case strings.HasPrefix(prog, "bravehtml"):
		return "brave"
	case strings.HasPrefix(prog, "vivaldi"):
		return "vivaldi"
	case strings.HasPrefix(prog, "opera"):
		return "opera"
	}
	return prog // Detect compares it with each browser id (strings.Contains)
}

// windowsOf looks at the visible top-level windows: which executables own one,
// and which of those have "ChatGPT" in a window's title. The titles are compared
// and dropped.
func windowsOf() (running, chatgpt map[string]bool) {
	running, chatgpt = map[string]bool{}, map[string]bool{}
	cb := syscall.NewCallback(func(h uintptr, _ uintptr) uintptr {
		hwnd := windows.HWND(h)
		if !windows.IsWindowVisible(hwnd) {
			return 1
		}
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid == 0 {
			return 1
		}
		id := IDOf(imagePath(pid))
		if id == "" {
			return 1
		}
		running[id] = true
		if strings.Contains(strings.ToLower(windowTitle(hwnd)), "chatgpt") {
			chatgpt[id] = true
		}
		return 1
	})
	_ = windows.EnumWindows(cb, unsafe.Pointer(nil))
	return running, chatgpt
}

var procGetWindowTextW = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowTextW")

// windowTitle is the text of a window's title bar (x/sys has no GetWindowText).
func windowTitle(hwnd windows.HWND) string {
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || n > uintptr(len(buf)) {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH*2)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
