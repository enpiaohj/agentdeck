package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
// 数据落点指向临时目录。应用内更新按真实装配接上：stage 只记路径，
// finish 由用例自己决定是否执行。
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
	a.SetUpdateInstaller(
		func(packagePath string) (string, error) { return filepath.Join(dir, filepath.Base(packagePath)), nil },
		func() string { return filepath.Join(dir, "AgentDeck.exe") },
	)
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

// 下载链路：走本机服务，校验 SHA-256 后把更新包落到数据目录下的 updates/。
//
// 发布里 exe 与 zip 是两个内容不同的资产，这里让 exe 的地址返回**exe 字节**：
// 曾经代码下载 exe 却按 zip 的哈希校验，这个夹具形态就是当时没被发现的原因。
func TestDownloadWritesVerifiedPackage(t *testing.T) {
	a, _ := updateFixture(t)
	const version = "1.1.0"
	exe := []byte("MZ exe payload")
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write(exe)
	}))
	defer server.Close()

	prepareAvailable(a, version, server.URL, exe)

	view, err := a.NewUpdateService().Download()
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if hits != 1 {
		t.Fatalf("应当只下载一次，实际 %d 次", hits)
	}
	if view.DownloadedDir != a.updateDir() || view.Downloading || view.Progress != 1 || view.Error != "" {
		t.Fatalf("下载后的视图不符：%+v", view)
	}
	want := filepath.Join(a.updateDir(), update.AssetPrefix+version+update.AssetSuffix)
	data, err := os.ReadFile(want)
	if err != nil || string(data) != string(exe) {
		t.Fatalf("下载内容不符：%q, %v", data, err)
	}
}

// 下载成功后必须登记为待落地：界面据此显示「重启并安装」，而不是让用户
// 自己去目录里找 exe 手动替换。
func TestDownloadStagesPackageForRestart(t *testing.T) {
	a, _ := updateFixture(t)
	const version = "1.1.0"
	exe := []byte("MZ exe payload")
	server := updateServer(t, version, exe)
	defer server.Close()
	prepareAvailable(a, version, server.URL, exe)

	view, err := a.NewUpdateService().Download()
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !view.ReadyToRestart || view.TargetPath == "" || !view.CanInstall {
		t.Fatalf("下载后应可重启安装：%+v", view)
	}
	if !strings.HasSuffix(view.TargetPath, update.AssetPrefix+version+update.AssetSuffix) {
		t.Fatalf("落地路径应跟随版本：%q", view.TargetPath)
	}
}

// 未装配安装器（开发构建或非桌面入口）时不给「下载」按钮一个注定失败的承诺。
func TestDownloadWithoutInstallerReportsUnsupported(t *testing.T) {
	a, _ := updateFixture(t)
	a.cfg.StageUpdate, a.cfg.FinishUpdate = nil, nil
	const version = "1.1.0"
	exe := []byte("MZ exe payload")
	server := updateServer(t, version, exe)
	defer server.Close()
	prepareAvailable(a, version, server.URL, exe)

	view, err := a.NewUpdateService().Download()
	if err == nil || !strings.Contains(err.Error(), "不支持应用内更新") {
		t.Fatalf("应当拒绝下载：%v", err)
	}
	if view.ReadyToRestart || view.CanInstall {
		t.Fatalf("不支持时不应报告可安装：%+v", view)
	}
}

// 登记失败（程序目录不可写）要如实报错，且不留下「已就绪」的假象。
func TestDownloadStagingFailureSurfaces(t *testing.T) {
	a, _ := updateFixture(t)
	a.SetUpdateInstaller(func(string) (string, error) { return "", errors.New("程序所在目录没有写入权限") }, func() string { return "" })
	const version = "1.1.0"
	exe := []byte("MZ exe payload")
	server := updateServer(t, version, exe)
	defer server.Close()
	prepareAvailable(a, version, server.URL, exe)

	view, err := a.NewUpdateService().Download()
	if err == nil || !strings.Contains(err.Error(), "写入权限") {
		t.Fatalf("应当报出写入权限问题：%v", err)
	}
	if view.ReadyToRestart {
		t.Fatalf("登记失败后不应报告可重启：%+v", view)
	}
}

// 缓存恢复的结论必须能直接下载。
//
// 自动检查默认开启，用户检查过之后 24 小时内每次启动都走缓存恢复。只恢复版本号
// 的话，界面上「下载并准备安装」可点、点下去必然报「缺少分发文件」——必须等 24
// 小时到期重新联网检查才恢复。
func TestCachedStateKeepsDownloadUsable(t *testing.T) {
	a, _ := updateFixture(t)
	const version = "1.1.0"
	exe := []byte("MZ exe payload")
	server := updateServer(t, version, exe)
	defer server.Close()
	a.cfg.UpdateAPIURL = server.URL + "/releases/latest"
	a.settings.AutoCheckUpdates = true

	// 第一次启动：联网检查，结论连同资产地址一起落盘。
	a.NewUpdateService().AutoCheckOnStartup()
	if view := a.updateView(); !view.Available {
		t.Fatalf("应当发现新版本：%+v", view)
	}
	if state := update.LoadState(a.updateStatePath()); state.ExeURL == "" {
		t.Fatal("检查结论应当连同资产地址落盘")
	}

	// 模拟当天重新启动：内存状态清空，缓存未过期，不再联网。
	a.updateMu.Lock()
	a.update = updateState{}
	a.updateMu.Unlock()
	a.NewUpdateService().AutoCheckOnStartup()

	view := a.updateView()
	if !view.Available || !view.Downloadable {
		t.Fatalf("缓存恢复后应当仍可下载：%+v", view)
	}
	if _, err := a.NewUpdateService().Download(); err != nil {
		t.Fatalf("缓存恢复后应当能直接下载，不该要求重新检查：%v", err)
	}
}

// 旧版本写下的缓存没有资产地址：此时不能装作可以下载。
func TestLegacyCachedStateIsNotDownloadable(t *testing.T) {
	a, _ := updateFixture(t)
	a.settings.AutoCheckUpdates = true
	if err := os.MkdirAll(a.updateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"checked_at":"` + time.Now().Add(-time.Hour).Format(time.RFC3339) +
		`","latest":"1.1.0","available":true,"notes":"## 变更"}`
	if err := os.WriteFile(a.updateStatePath(), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	a.NewUpdateService().AutoCheckOnStartup()
	view := a.updateView()
	if !view.Available {
		t.Fatalf("仍应提示有新版本：%+v", view)
	}
	_, err := a.NewUpdateService().Download()
	if err == nil || !strings.Contains(err.Error(), "重新检查") {
		t.Fatalf("应当让用户先重新检查：%v", err)
	}
}

// 取消下载：不是失败，退回「有新版本」，且不留下半成品。
func TestCancelDownloadReturnsToAvailable(t *testing.T) {
	a, _ := updateFixture(t)
	const version = "1.1.0"
	exe := []byte("MZ exe payload")
	blocked := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 发出响应头之后一直不再发数据：下载会停在这里，等取消。
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-blocked
	}))
	defer server.Close()
	// defer 后进先出：先放开处理器，再关服务（Close 会等未完成的请求）。
	defer close(blocked)
	prepareAvailable(a, version, server.URL, exe)

	done := make(chan error, 1)
	go func() { _, err := a.NewUpdateService().Download(); done <- err }()

	deadline := time.Now().Add(10 * time.Second)
	for !a.updateView().Downloading {
		if time.Now().After(deadline) {
			t.Fatal("下载没有进入下载中状态")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := a.NewUpdateService().CancelDownload(); err != nil {
		t.Fatalf("CancelDownload: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, errDownloadCanceled) {
			t.Fatalf("取消下载应当返回明确的取消结果，实际 %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("取消后下载没有结束")
	}
	view := a.updateView()
	if view.Downloading || view.ReadyToRestart || view.Error != "" {
		t.Fatalf("取消后应当退回可下载状态：%+v", view)
	}
	if !view.Downloadable {
		t.Fatalf("取消后应当还能重新下载：%+v", view)
	}
	if entries, _ := os.ReadDir(a.updateDir()); len(entries) != 0 {
		t.Fatalf("取消后不该留下半成品：%v", entries)
	}
	if _, err := a.NewUpdateService().CancelDownload(); err == nil {
		t.Fatal("没有下载在跑时应当报错")
	}
}

func TestRestartToInstallRequiresStagedPackage(t *testing.T) {
	a, _ := updateFixture(t)
	if err := a.NewUpdateService().RestartToInstall(); err == nil {
		t.Fatal("没有待安装的更新时应报错")
	}
}

// updateServer 模拟一次真实发布：/releases/latest 返回接口 JSON（带 digest），
// 其余路径（也就是资产地址 /pkg）返回 exe 字节。
func updateServer(t *testing.T, version string, exe []byte) *httptest.Server {
	t.Helper()
	name := update.AssetPrefix + version + update.AssetSuffix
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			fmt.Fprintf(w, `{"tag_name":"v%s","name":"AgentDeck v%s","body":"## 变更\n- 修复",
				"html_url":"https://example.test/release",
				"assets":[{"name":"%s","browser_download_url":"http://%s/pkg","size":%d,"digest":"sha256:%s"}]}`,
				version, version, name, r.Host, len(exe), sumOf(exe))
			return
		}
		w.Write(exe)
	}))
}

// prepareAvailable 把「已经检查出有 1.1.0」的状态写进 App，资产地址指向 baseURL，
// digest 按 exe 内容算——真实的 GitHub 发布就是这么给的。
func prepareAvailable(a *App, version, baseURL string, exe []byte) {
	a.updateMu.Lock()
	a.update.checked = true
	a.update.result = update.Result{
		Current:   "0.9.0",
		Latest:    version,
		Available: true,
		ExeAsset: update.Asset{
			Name: update.AssetPrefix + version + update.AssetSuffix, URL: baseURL + "/pkg",
			Size: int64(len(exe)), Digest: "sha256:" + sumOf(exe),
		},
	}
	a.updateMu.Unlock()
}

func sumOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// 上次的结论是「更新但暂无可下载文件」：恢复出同样的提示，而不是可点必败的下载，
// 也不是误导的「已是最新版本」。
func TestRestoreUpdateStateKeepsAssetMissing(t *testing.T) {
	a, _ := updateFixture(t)
	a.version = "1.0.0"
	a.restoreUpdateState(update.State{CheckedAt: time.Now(), Latest: "1.1.0", AssetMissing: true})
	view := a.updateView()
	if !view.Checked || view.Available || view.Downloadable || !view.AssetMissing || view.Latest != "1.1.0" {
		t.Fatalf("应显示更新但无可下载文件：%+v", view)
	}
}

// 用户已手动升级后，昨天的「暂无文件」结论同样作废。
func TestRestoreUpdateStateDropsAssetMissingWhenCurrent(t *testing.T) {
	a, _ := updateFixture(t)
	a.version = "1.1.0"
	a.restoreUpdateState(update.State{CheckedAt: time.Now(), Latest: "1.1.0", AssetMissing: true})
	if view := a.updateView(); view.AssetMissing || view.Available {
		t.Fatalf("同版本不应再提示：%+v", view)
	}
}
