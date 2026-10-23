package app

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
	"github.com/enpiaohj/agentdeck/internal/update"
)

// updateFixture 构造一个只有更新状态与配置的 App：不打开任何数据库，
// 数据落点指向临时目录。
func updateFixture(t *testing.T) (*App, *[]string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	launched := []string{}
	a := &App{
		settings: appconfig.Default(),
		version:  "0.9.0",
		paths: appconfig.Paths{
			ConfigDir: dir,
			LocalDir:  filepath.Join(dir, "local"),
		},
		cfg: Config{
			Runner: func(name string, args ...string) error {
				launched = append(launched, name+" "+strings.Join(args, " "))
				return nil
			},
		},
	}
	return a, &launched
}

func TestUpdateStatusStartsUnknown(t *testing.T) {
	a, _ := updateFixture(t)
	view := a.NewUpdateService().Status()
	if view.Checked || view.Available || view.Checking || view.Downloading {
		t.Fatalf("未检查时不应有结论：%+v", view)
	}
	if view.Current != "0.9.0" || view.Dev {
		t.Fatalf("当前版本：%+v", view)
	}
	if !view.AutoCheck {
		t.Fatal("默认应当开启启动后自动检查")
	}
	if view.ReleasePageURL == "" {
		t.Fatal("应当给出发布页面地址")
	}
}

func TestUpdateStatusDevBuild(t *testing.T) {
	a, _ := updateFixture(t)
	a.version = ""
	view := a.NewUpdateService().Status()
	if !view.Dev || view.Current != "dev" {
		t.Fatalf("读不到版本号时应当标为开发版本：%+v", view)
	}
}

func TestSetAutoCheckPersists(t *testing.T) {
	a, _ := updateFixture(t)
	service := a.NewUpdateService()
	view, err := service.SetAutoCheck(false)
	if err != nil || view.AutoCheck {
		t.Fatalf("关闭自动检查：%+v, %v", view, err)
	}
	path, _ := appconfig.Path()
	cfg, err := appconfig.LoadFrom(path)
	if err != nil || cfg.AutoCheckUpdates {
		t.Fatalf("配置未落盘：%+v, %v", cfg, err)
	}
	if a.autoCheckSetting() {
		t.Fatal("内存中的设置没同步")
	}
}

// 旧配置没有 auto_check_updates 字段时必须按默认（开启）处理，不能因为
// bool 的零值把已有用户的自动检查关掉。
func TestUpdateAutoCheckDefaultsOnForOldConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app-config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"terminal_shell":"powershell","close_to_tray":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := appconfig.LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if !cfg.AutoCheckUpdates {
		t.Fatal("旧配置缺少字段时自动检查应保持开启")
	}
}

// 启动时的后台检查在缓存未到期时一次网络请求都不发，只沿用上次结论。
func TestAutoCheckOnStartupUsesCache(t *testing.T) {
	a, _ := updateFixture(t)
	state := update.State{
		CheckedAt: time.Now().Add(-time.Hour),
		Current:   "0.9.0",
		Latest:    "1.1.0",
		Available: true,
		Notes:     "## 变更",
		PageURL:   "https://example.test/release",
	}
	if err := update.SaveState(a.updateStatePath(), state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	a.NewUpdateService().AutoCheckOnStartup()
	view := a.updateView()
	if !view.Checked || !view.Available || view.Latest != "1.1.0" {
		t.Fatalf("应当沿用缓存结论：%+v", view)
	}
	if view.CheckedAt == "" || view.PublishedAt != "" {
		t.Fatalf("时间字段不符：%+v", view)
	}
}

// 缓存过期时才会真的去查：这里指向本机服务，确认请求发出、结论落盘。
func TestAutoCheckOnStartupFetchesWhenDue(t *testing.T) {
	a, _ := updateFixture(t)
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		io.WriteString(w, `{"tag_name":"v1.1.0","body":"## 变更","html_url":"https://example.test/release",
			"assets":[{"name":"AgentDeck-v1.1.0-win-x64.exe","browser_download_url":"http://`+r.Host+`/pkg.exe"},
			{"name":"SHA256SUMS","browser_download_url":"http://`+r.Host+`/SHA256SUMS"}]}`)
	}))
	defer server.Close()
	a.cfg.UpdateAPIURL = server.URL

	old := update.State{CheckedAt: time.Now().Add(-48 * time.Hour), Latest: "0.8.0"}
	if err := update.SaveState(a.updateStatePath(), old); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	a.NewUpdateService().AutoCheckOnStartup()
	view := a.updateView()
	if hits != 1 {
		t.Fatalf("到期时应当查询一次，实际 %d 次", hits)
	}
	if !view.Checked || !view.Available || view.Latest != "1.1.0" {
		t.Fatalf("结论不符：%+v", view)
	}
	if state := update.LoadState(a.updateStatePath()); state.Latest != "1.1.0" || state.CheckedAt.IsZero() {
		t.Fatalf("检查结果未落盘：%+v", state)
	}
}

// 自动检查关闭时，即使缓存过期也不查询。
func TestAutoCheckOnStartupRespectsSwitch(t *testing.T) {
	a, _ := updateFixture(t)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{"tag_name":"v1.1.0"}`))
	}))
	defer server.Close()
	a.cfg.UpdateAPIURL = server.URL
	a.settings.AutoCheckUpdates = false
	a.NewUpdateService().AutoCheckOnStartup()
	if hits != 0 {
		t.Fatalf("关闭后不应发起请求，实际 %d 次", hits)
	}
}

// 用户已经手动升级过 exe 时，昨天的结论不能继续提示「有新版本」。
func TestRestoreUpdateStateRecomputesAvailability(t *testing.T) {
	a, _ := updateFixture(t)
	a.version = "1.1.0"
	a.restoreUpdateState(update.State{CheckedAt: time.Now(), Latest: "1.1.0", Available: true})
	view := a.updateView()
	if !view.Checked || view.Available || view.Downloadable {
		t.Fatalf("同版本不应再提示更新：%+v", view)
	}
}

func TestDownloadWithoutKnownUpdateFails(t *testing.T) {
	a, _ := updateFixture(t)
	if _, err := a.NewUpdateService().Download(); err == nil {
		t.Fatal("没有可下载版本时应当报错")
	}
}

func TestOpenDownloadFolderCreatesDirectory(t *testing.T) {
	a, launched := updateFixture(t)
	if err := a.NewUpdateService().OpenDownloadFolder(); err != nil {
		t.Fatalf("OpenDownloadFolder: %v", err)
	}
	if len(*launched) != 1 || !strings.HasPrefix((*launched)[0], "explorer ") {
		t.Fatalf("应当调用 explorer 打开目录：%v", *launched)
	}
	if _, err := os.Stat(a.updateDir()); err != nil {
		t.Fatalf("下载目录应当被创建：%v", err)
	}
}

// 下载链路：走本机服务，校验 SHA-256 后解压到数据目录下的 updates/。
func TestDownloadWritesVerifiedPackage(t *testing.T) {
	a, _ := updateFixture(t)
	const version = "1.1.0"
	root := "AgentDeck-v" + version + "-win-x64"
	exe := []byte("exe payload")
	zipBytes := buildTestZip(t, map[string][]byte{root + "/" + update.AssetPrefix + version + update.AssetSuffix: exe})

	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == update.SumsAsset {
			w.Write([]byte(sumOf(zipBytes) + "  AgentDeck-v" + version + "-win-x64.zip\n" +
				sumOf(exe) + "  " + update.AssetPrefix + version + update.AssetSuffix + "\n"))
			return
		}
		hits++
		w.Write(zipBytes)
	}))
	defer server.Close()

	a.updateMu.Lock()
	a.update.checked = true
	a.update.result = update.Result{
		Current:   "0.9.0",
		Latest:    version,
		Available: true,
		ExeAsset:  update.Asset{Name: update.AssetPrefix + version + update.AssetSuffix, URL: server.URL + "/pkg"},
		SumsAsset: update.Asset{Name: update.SumsAsset, URL: server.URL + "/" + update.SumsAsset},
	}
	a.updateMu.Unlock()

	view, err := a.NewUpdateService().Download()
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if hits != 1 {
		t.Fatalf("应当只下载一次，实际 %d 次", hits)
	}
	want := filepath.Join(a.updateDir(), root)
	if view.DownloadedDir != want || view.Downloading || view.Progress != 1 || view.Error != "" {
		t.Fatalf("下载后的视图不符：%+v", view)
	}
	data, err := os.ReadFile(filepath.Join(want, update.AssetPrefix+version+update.AssetSuffix))
	if err != nil || string(data) != string(exe) {
		t.Fatalf("解压内容不符：%q, %v", data, err)
	}
}

func buildTestZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("构造 ZIP: %v", err)
		}
		if _, err = entry.Write(body); err != nil {
			t.Fatalf("构造 ZIP: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("构造 ZIP: %v", err)
	}
	return buf.Bytes()
}

func sumOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
