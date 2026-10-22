package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/enpiaohj/agentdeck/internal/update"
)

// 在线更新的时间参数。自动检查每天最多一次：GitHub 对匿名请求限制为
// 每小时 60 次，用户也没有理由一天内被同一条提示反复打扰。
const (
	updateCheckInterval = 24 * time.Hour
	updateCheckTimeout  = 20 * time.Second
)

// updateState 是本次运行内的更新状态，由 App.updateMu 保护。
type updateState struct {
	checking    bool
	downloading bool
	progress    float64
	// checked 表示 result 有含义（本次查到，或启动时从缓存恢复）。
	checked bool
	result  update.Result
	// dir 是上一次下载解压到的目录。
	dir string
	// errMsg 是上一次操作的错误说明。
	errMsg string
}

// UpdateService 提供「检查更新 → 下载到本地」。
//
// 刻意不自动替换正在运行的 exe：产品没有代码签名，替换失败用户手里
// 就没有能用的程序；把解压目录交给用户手动替换，出问题也回得去。
type UpdateService struct{ a *App }

// Version 返回产品版本；开发构建（读不到 VERSIONINFO）返回 "dev"。
func (a *App) Version() string {
	if a.version == "" {
		return "dev"
	}
	return a.version
}

// updateDir 是更新包与检查缓存的落点，跟随数据目录（迁移后一并搬家）。
func (a *App) updateDir() string {
	return filepath.Join(a.paths.LocalDir, "updates")
}

// updateStatePath 是「上一次检查」的缓存文件。
func (a *App) updateStatePath() string {
	return filepath.Join(a.updateDir(), "check.json")
}

// Status 返回当前更新状态，不触发任何网络请求。
func (s *UpdateService) Status() UpdateView { return s.a.updateView() }

func (a *App) updateView() UpdateView {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.updateViewLocked()
}

// updateViewLocked 组装视图，调用方需持有 updateMu。
//
// 这里会再取 settingsMu 读自动检查开关；锁序固定为 updateMu → settingsMu，
// 而写设置的路径从不反过来（SetAutoCheck 先释放 settingsMu 再取 updateMu）。
func (a *App) updateViewLocked() UpdateView {
	view := UpdateView{
		Current:        a.Version(),
		Dev:            a.version == "",
		Checked:        a.update.checked,
		Checking:       a.update.checking,
		Downloading:    a.update.downloading,
		Progress:       a.update.progress,
		DownloadedDir:  a.update.dir,
		Error:          a.update.errMsg,
		AutoCheck:      a.autoCheckSetting(),
		ReleasePageURL: update.ReleasePageURL,
	}
	if a.update.checking || a.update.downloading {
		view.Downloadable = false
		return view
	}
	if !a.update.checked {
		return view
	}
	r := a.update.result
	view.Latest = r.Latest
	view.Available = r.Available
	view.Notes = r.Notes
	view.PageURL = r.PageURL
	view.CheckedAt = rfc3339(r.CheckedAt)
	view.PublishedAt = rfc3339(r.PublishedAt)
	view.Downloadable = r.Available
	return view
}

func rfc3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

// autoCheckSetting 读「启动后自动检查」开关。
func (a *App) autoCheckSetting() bool {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.settings.AutoCheckUpdates
}

// SetAutoCheck 保存「启动后自动检查」开关。
func (s *UpdateService) SetAutoCheck(enabled bool) (UpdateView, error) {
	a := s.a
	a.settingsMu.Lock()
	cfg := a.settings
	cfg.AutoCheckUpdates = enabled
	err := appconfig.Save(cfg)
	if err == nil {
		a.settings = cfg
	}
	a.settingsMu.Unlock()
	if err != nil {
		return a.updateView(), fmt.Errorf("写入应用配置失败：%w", err)
	}
	view := a.updateView()
	a.emit(EventUpdateStatus, view)
	return view, nil
}

// Check 立即检查一次更新（用户点击「检查更新」）。不受 24 小时限制。
func (s *UpdateService) Check() (UpdateView, error) {
	a := s.a
	a.updateMu.Lock()
	if a.update.checking || a.update.downloading {
		view := a.updateViewLocked()
		a.updateMu.Unlock()
		return view, errors.New("上一次操作还没结束，请稍候")
	}
	a.update.checking = true
	a.update.errMsg = ""
	view := a.updateViewLocked()
	a.updateMu.Unlock()
	a.emit(EventUpdateStatus, view)

	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()
	return s.check(ctx)
}

// AutoCheckOnStartup 是启动后的后台检查：开关关闭、或距上次检查不足
// 一天时直接沿用缓存结论，一次网络请求都不发。
func (s *UpdateService) AutoCheckOnStartup() {
	a := s.a
	if !a.autoCheckSetting() {
		return
	}
	state := update.LoadState(a.updateStatePath())
	if !update.StateDue(state, updateCheckInterval, time.Now()) {
		a.restoreUpdateState(state)
		return
	}
	// 与手动检查共用同一个「进行中」标记：后台检查期间用户点「检查更新」
	// 只会被告知稍候，不会并发打两次接口。
	a.updateMu.Lock()
	if a.update.checking || a.update.downloading {
		a.updateMu.Unlock()
		return
	}
	a.update.checking = true
	a.updateMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()
	// 失败只写进视图的 Error，不影响启动；下一次手动检查会如实报错。
	if _, err := s.check(ctx); err != nil {
		return
	}
}

// restoreUpdateState 用昨天的结论填充视图。是否「有新版本」按当前版本
// 重新判断——用户可能已经手动替换过 exe。
func (a *App) restoreUpdateState(state update.State) {
	if state.CheckedAt.IsZero() {
		return
	}
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	a.update.result = update.Result{
		Current:     a.Version(),
		Latest:      state.Latest,
		Available:   update.IsNewer(state.Latest, a.Version()),
		Notes:       state.Notes,
		PageURL:     state.PageURL,
		PublishedAt: state.PublishedAt,
		CheckedAt:   state.CheckedAt,
	}
	a.update.checked = true
}

// check 执行一次真实的网络查询并更新状态。调用方负责把 checking 置位。
func (s *UpdateService) check(ctx context.Context) (UpdateView, error) {
	a := s.a
	client := update.Client{APIURL: a.cfg.UpdateAPIURL, Version: a.Version()}
	result, err := client.Check(ctx, a.Version())

	a.updateMu.Lock()
	a.update.checking = false
	if err != nil {
		a.update.errMsg = err.Error()
		view := a.updateViewLocked()
		a.updateMu.Unlock()
		a.emit(EventUpdateStatus, view)
		return view, err
	}
	a.update.result = result
	a.update.checked = true
	a.update.errMsg = ""
	view := a.updateViewLocked()
	a.updateMu.Unlock()
	a.saveUpdateState(result)
	a.emit(EventUpdateStatus, view)
	return view, nil
}

// saveUpdateState 记录本次检查，供下次启动的 24 小时判断使用。
//
// 写失败不外泄：这个文件只影响「下次是否再查一次」，丢了最坏是多查一次，
// 与本次已经得到的结论无关，不该在界面上报一个错。
func (a *App) saveUpdateState(result update.Result) {
	_ = update.SaveState(a.updateStatePath(), update.State{
		CheckedAt:   result.CheckedAt,
		Current:     result.Current,
		Latest:      result.Latest,
		Available:   result.Available,
		Notes:       result.Notes,
		PageURL:     result.PageURL,
		PublishedAt: result.PublishedAt,
	})
}

// Download 下载并校验新版本，解压到数据目录下的 updates/。
// 不替换正在运行的程序，失败也不留下半成品。
func (s *UpdateService) Download() (UpdateView, error) {
	a := s.a
	a.updateMu.Lock()
	if a.update.checking || a.update.downloading {
		view := a.updateViewLocked()
		a.updateMu.Unlock()
		return view, errors.New("上一次操作还没结束，请稍候")
	}
	if !a.update.checked || !a.update.result.Available {
		view := a.updateViewLocked()
		a.updateMu.Unlock()
		return view, errors.New("当前没有可下载的新版本")
	}
	result := a.update.result
	a.update.downloading = true
	a.update.progress = 0
	a.update.errMsg = ""
	view := a.updateViewLocked()
	a.updateMu.Unlock()
	a.emit(EventUpdateStatus, view)

	ctx, cancel := context.WithTimeout(context.Background(), update.DownloadTimeout)
	defer cancel()
	client := update.Client{Version: a.Version()}
	dir, err := client.Download(ctx, result, a.updateDir(), func(done, total int64) {
		if total <= 0 {
			return
		}
		a.updateMu.Lock()
		a.update.progress = float64(done) / float64(total)
		view := a.updateViewLocked()
		a.updateMu.Unlock()
		a.emit(EventUpdateStatus, view)
	})

	a.updateMu.Lock()
	a.update.downloading = false
	if err != nil {
		a.update.errMsg = err.Error()
		a.update.progress = 0
	} else {
		a.update.dir = dir
		a.update.errMsg = ""
		a.update.progress = 1
	}
	view = a.updateViewLocked()
	a.updateMu.Unlock()
	a.emit(EventUpdateStatus, view)
	if err != nil {
		return view, err
	}
	return view, nil
}

// OpenDownloadFolder 在资源管理器里打开更新包所在目录。
func (s *UpdateService) OpenDownloadFolder() error {
	dir := s.a.updateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("无法创建下载目录：%w", err)
	}
	if s.a.cfg.Runner != nil {
		return s.a.cfg.Runner("explorer", dir)
	}
	return proc.Command("explorer", dir).Start()
}
