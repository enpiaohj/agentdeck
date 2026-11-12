package app

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
	"github.com/enpiaohj/agentdeck/internal/browsers"
)

type launched struct{ exe, link string }

// newBrowserTestApp 建一个浏览器检测与启动都被替换掉的应用：不扫描本机，也不拉起真浏览器。
func newBrowserTestApp(t *testing.T, list []browsers.Browser, launchErr error) (*SettingsService, *[]launched) {
	t.Helper()
	var calls []launched
	a, err := New(Config{
		MarksPath:    filepath.Join(t.TempDir(), "marks.db"),
		ProjectsPath: filepath.Join(t.TempDir(), "projects.db"),
		IndexPath:    ":memory:",
		Browsers:     func() []browsers.Browser { return list },
		LaunchBrowser: func(exe, link string) error {
			calls = append(calls, launched{exe, link})
			return launchErr
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a.NewSettingsService(), &calls
}

var fakeBrowsers = []browsers.Browser{
	{ID: "doubao", Name: "豆包", Path: `C:\Apps\doubao.exe`, Default: true, Running: true},
	{ID: "msedge", Name: "Microsoft Edge", Path: `C:\Edge\msedge.exe`, Running: true, ChatGPT: true},
	{ID: "chrome", Name: "Google Chrome", Path: `C:\Chrome\chrome.exe`},
}

func TestBrowserSettingsDefaultAndOptions(t *testing.T) {
	svc, _ := newBrowserTestApp(t, fakeBrowsers, nil)
	v := svc.BrowserSettings()
	if v.Setting != "" || v.Name != "豆包" || v.Error != "" || len(v.Options) != 3 {
		t.Fatalf("没有选择时显示系统默认浏览器的名字: %+v", v)
	}
	if !v.Options[0].Default || !v.Options[1].ChatGPT || !v.Options[1].Running || v.Options[2].Running {
		t.Fatalf("选项应带上默认/运行中/ChatGPT 提示: %+v", v.Options)
	}
}

func TestSetBrowserAndOpenLink(t *testing.T) {
	svc, calls := newBrowserTestApp(t, fakeBrowsers, nil)
	link := "https://chatgpt.com/g/g-p-0123456789abcdef0123456789abcdef/project"

	// 没有选择：交给调用方用系统默认浏览器，不自己启动
	if opened, err := svc.OpenLink(link); err != nil || opened || len(*calls) != 0 {
		t.Fatalf("没有选择: opened=%v err=%v calls=%v", opened, err, *calls)
	}
	// 选一个本机检测到的浏览器
	v, err := svc.SetBrowser("msedge")
	if err != nil || v.Setting != "msedge" || v.Name != "Microsoft Edge" {
		t.Fatalf("SetBrowser: %+v %v", v, err)
	}
	if opened, err := svc.OpenLink(link); err != nil || !opened || len(*calls) != 1 || (*calls)[0] != (launched{`C:\Edge\msedge.exe`, link}) {
		t.Fatalf("应用所选浏览器的路径与完整链接启动: opened=%v err=%v calls=%v", opened, err, *calls)
	}
	// 重新读配置也还是它（真正写进了文件）
	if cfg, err := appconfig.Load(); err != nil || cfg.Browser != "msedge" {
		t.Fatalf("配置文件里的浏览器 = %q, %v", cfg.Browser, err)
	}
	// 改回系统默认
	if v, err = svc.SetBrowser(""); err != nil || v.Setting != "" {
		t.Fatalf("改回默认: %+v %v", v, err)
	}
	if opened, _ := svc.OpenLink(link); opened {
		t.Fatal("改回默认后不该自己启动")
	}
}

func TestSetBrowserRejectsUnknown(t *testing.T) {
	svc, _ := newBrowserTestApp(t, fakeBrowsers, nil)
	for _, bad := range []string{"firefox", `C:\Windows\System32\cmd.exe`, "../x"} {
		if _, err := svc.SetBrowser(bad); err == nil {
			t.Errorf("%q 不是本机检测到的浏览器，应被拒绝", bad)
		}
	}
	if v := svc.BrowserSettings(); v.Setting != "" {
		t.Fatalf("被拒绝的取值不该写入: %+v", v)
	}
}

// 链接总该打得开：所选浏览器没了、启动失败，都退回系统默认浏览器（返回 false 而不是报错）。
func TestOpenLinkFallsBackToDefault(t *testing.T) {
	list := append([]browsers.Browser{}, fakeBrowsers...)
	svc, calls := newBrowserTestApp(t, list, nil)
	if _, err := svc.SetBrowser("chrome"); err != nil {
		t.Fatal(err)
	}
	// 之后 Chrome 被卸载了
	svc.a.cfg.Browsers = func() []browsers.Browser { return fakeBrowsers[:2] }
	if v := svc.BrowserSettings(); v.Error == "" || v.Name != "豆包" {
		t.Fatalf("所选浏览器找不到时要提示，并说明会用默认浏览器: %+v", v)
	}
	if opened, err := svc.OpenLink("https://example.com/"); err != nil || opened || len(*calls) != 0 {
		t.Fatalf("所选浏览器找不到: opened=%v err=%v", opened, err)
	}
	// 启动失败
	svc2, _ := newBrowserTestApp(t, fakeBrowsers, errors.New("boom"))
	if _, err := svc2.SetBrowser("msedge"); err != nil {
		t.Fatal(err)
	}
	if opened, err := svc2.OpenLink("https://example.com/"); err != nil || opened {
		t.Fatalf("启动失败应退回默认而不是报错: opened=%v err=%v", opened, err)
	}
}

// 只接受 http(s) 链接，其余（会话正文里的链接来自不可信内容）一律拒绝。
func TestOpenLinkRefusesOtherSchemes(t *testing.T) {
	svc, calls := newBrowserTestApp(t, fakeBrowsers, nil)
	if _, err := svc.SetBrowser("msedge"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"file:///C:/Windows/notepad.exe", "javascript:alert(1)", "--remote-debugging-port=1", "", "https://a.com/x y"} {
		if opened, err := svc.OpenLink(bad); err == nil || opened {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("被拒绝的链接不该启动任何东西: %v", *calls)
	}
}
