package browsers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExePath(t *testing.T) {
	for in, want := range map[string]string{
		`"C:\Program Files\Google\Chrome\Application\chrome.exe" --single-argument %1`: `C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"`:               `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Quark\quark.exe -url "%1"`:                                                 `C:\Quark\quark.exe`,
		`  "D:\a b\c.exe"  `:                                                           `D:\a b\c.exe`,
		``:                                                                             ``,
		`"unterminated`:                                                                ``,
		`no executable here`:                                                           ``,
	} {
		if got := ExePath(in); got != want {
			t.Errorf("ExePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIDs(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`: "chrome",
		`C:\X\MSEdge.EXE`: "msedge",
		`firefox.exe`:     "firefox",
		``:                "",
	} {
		if got := IDOf(in); got != want {
			t.Errorf("IDOf(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"chrome": "chrome", " MsEdge ": "msedge", "": "", "../x": "", "a b": "", "-x": "", strings.Repeat("a", 41): "", "doubao": "doubao", "360se.v2": "360se.v2"} {
		if got := NormalizeID(in); got != want {
			t.Errorf("NormalizeID(%q) = %q, want %q", in, got, want)
		}
	}
}

// Only http(s) links, in one piece: nothing the browser could read as an option or another file.
func TestValidURL(t *testing.T) {
	for _, ok := range []string{"https://chatgpt.com/g/g-p-0123456789abcdef0123456789abcdef/project", "http://localhost:8080/a?b=c#d", "https://example.com/a%20b"} {
		if err := ValidURL(ok); err != nil {
			t.Errorf("%q 应被接受: %v", ok, err)
		}
	}
	for _, bad := range []string{"", " https://a.com", "https://a.com ", "https://a.com/x y", "https://a.com\nhttps://b.com", "file:///C:/Windows/System32/cmd.exe",
		"javascript:alert(1)", "--new-window", "-incognito", "chrome://settings", "ftp://a.com", "https://", "//a.com", "a.com", `C:\Windows\notepad.exe`, "https://a.com/\x00", strings.Repeat("a", 5000)} {
		if err := ValidURL(bad); err == nil {
			t.Errorf("%q 不该被接受", bad)
		}
	}
}

func TestOrderAndFind(t *testing.T) {
	list := []Browser{{ID: "z", Name: "Zed"}, {ID: "run", Name: "Running", Running: true}, {ID: "gpt", Name: "Has ChatGPT", Running: true, ChatGPT: true}, {ID: "def", Name: "Default", Default: true}, {ID: "a", Name: "Alpha"}}
	Order(list)
	var ids []string
	for _, b := range list {
		ids = append(ids, b.ID)
	}
	if got := strings.Join(ids, ","); got != "def,gpt,run,a,z" {
		t.Fatalf("顺序 = %s，应为 默认、检测到 ChatGPT、运行中、其余按名字", got)
	}
	if b, ok := Find(list, "gpt"); !ok || b.Name != "Has ChatGPT" {
		t.Fatal("Find")
	}
	if _, ok := Find(list, "nope"); ok {
		t.Fatal("Find 不该找到不存在的")
	}
}

func TestLaunchRefusesBadInput(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "browser.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string][2]string{
		"链接不是 http(s)": {exe, "file:///etc/passwd"},
		"相对路径":         {"browser.exe", "https://a.com"},
		"不是 exe":       {filepath.Join(t.TempDir(), "a.txt"), "https://a.com"},
		"文件不存在":        {filepath.Join(t.TempDir(), "missing.exe"), "https://a.com"},
	} {
		if err := Launch(c[0], c[1]); err == nil {
			t.Errorf("%s: 应被拒绝", name)
		}
	}
}
