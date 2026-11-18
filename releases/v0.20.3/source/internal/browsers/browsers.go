// Package browsers finds the web browsers installed on this computer and opens
// a link in a chosen one.
//
// It exists for one thing: the user is signed in to a website (ChatGPT) in a
// particular browser, and a link meant for that site must open there, not in
// whatever the system default happens to be. Nothing about a browser's contents
// is read — no profile, history, cookie or password; what is looked at is which
// browsers are installed (the registry's list of them), which has a window
// open, and whether such a window's title says ChatGPT. The titles are only
// compared, never kept.
package browsers

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Browser is an installed web browser.
type Browser struct {
	ID   string // stable key: the executable's name without ".exe", lower case ("chrome", "msedge")
	Name string // as the system lists it ("Google Chrome")
	Path string // the executable
	// Default: it is the system's default browser for https links.
	Default bool
	// Running: it has a window open now.
	Running bool
	// ChatGPT: one of its windows has "ChatGPT" in its title — a hint that the
	// user has ChatGPT open in it. Only the tab shown in each window can be
	// seen, so the absence of this proves nothing.
	ChatGPT bool
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,39}$`)

// NormalizeID returns id when it is a well-formed browser id, "" otherwise
// ("" also means "the system default").
func NormalizeID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if idPattern.MatchString(id) {
		return id
	}
	return ""
}

// IDOf is the browser id of an executable path.
func IDOf(exe string) string {
	if strings.TrimSpace(exe) == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(exe, "\\", "/")))
	if base == "." || base == "/" {
		return ""
	}
	return strings.TrimSuffix(base, ".exe")
}

// ExePath pulls the executable out of a registry "open command" value:
//
//	"C:\Program Files\Browser\b.exe" --flag %1   → C:\Program Files\Browser\b.exe
//	C:\Browser\b.exe -url "%1"                   → C:\Browser\b.exe
//
// "" when there is none.
func ExePath(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	if command[0] == '"' {
		if end := strings.IndexByte(command[1:], '"'); end >= 0 {
			return strings.TrimSpace(command[1 : 1+end])
		}
		return ""
	}
	if i := strings.Index(strings.ToLower(command), ".exe"); i >= 0 {
		return strings.TrimSpace(command[:i+4])
	}
	return ""
}

// ErrURL is why a link is refused.
var ErrURL = errors.New("只能用浏览器打开 http 或 https 链接")

// ValidURL accepts an absolute http or https link and nothing else. Whatever the
// link, it is handed to the browser as a single argument; being http(s) it can
// never be read as one of the browser's own options.
func ValidURL(raw string) error {
	if raw == "" || len(raw) > 4096 || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\t\x00 ") {
		return ErrURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ErrURL
	}
	return nil
}

// Find returns the browser with the given id.
func Find(list []Browser, id string) (Browser, bool) {
	for _, b := range list {
		if b.ID == id {
			return b, true
		}
	}
	return Browser{}, false
}

// Order puts the system default first, then the browsers that look like they
// have ChatGPT open, then the running ones, then the rest by name.
func Order(list []Browser) {
	rank := func(b Browser) int {
		switch {
		case b.Default:
			return 0
		case b.ChatGPT:
			return 1
		case b.Running:
			return 2
		}
		return 3
	}
	sort.SliceStable(list, func(i, j int) bool {
		if ri, rj := rank(list[i]), rank(list[j]); ri != rj {
			return ri < rj
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
}

// exists reports whether path is a regular file.
func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// absExe reports whether exe is an absolute path to a .exe.
func absExe(exe string) bool {
	return filepath.IsAbs(exe) && strings.EqualFold(filepath.Ext(exe), ".exe")
}
