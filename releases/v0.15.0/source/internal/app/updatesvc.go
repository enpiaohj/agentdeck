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
	// packagePath 是已登记待落地的更新包；空表示没有。
	packagePath string
	// targetPath 是新版本落地后的路径。
	targetPath string
	// errMsg 是上一次操作的错误说明。
	errMsg string
}

// UpdateService 提供「检查更新 → 下载校验 → 退出时落地 → 重启」。
//
// 落地一律推迟到进程退出的最后一步：正在运行的 exe 不能被覆盖，而落地
// 失败会让用户手里没有能用的程序。产品没有代码签名，校验只说明下载内容与
// 发布清单一致，不构成发布者身份证明。
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
		ReadyToRestart: a.update.packagePath != "",
		TargetPath:     a.update.targetPath,
		CanInstall:     a.cfg.StageUpdate != nil,
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

// Download 下载并校验新版本，然后登记为「退出时落地」。
//
// 不覆盖正在运行的程序：登记只做可写性检查，落地在进程退出、数据库关闭与
// 单实例锁释放之后进行（见 FinishUpdate）。失败也不留下半成品。
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
	if a.cfg.StageUpdate == nil {
		view := a.updateViewLocked()
		a.updateMu.Unlock()
		return view, errors.New("当前运行方式不支持应用内更新，请到发布页下载")
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

	if err == nil {
		// 登记必须在下载结果之外单独报错：可写性检查失败（程序装在 Program
		// Files 之类只读目录）是用户最可能遇到的情况，得说清楚下一步做什么。
		dir, err = a.stageUpdate(dir, result)
	}

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

// stageUpdate 从解压目录里取出 exe 并登记为待落地，返回解压目录。
//
// 更新包里的 exe 名与版本一致（AgentDeck-vX.Y.Z-win-x64.exe），落地后的名字
// 由 StageUpdate 决定：当前文件名是发布产物命名时跟着版本走，否则原地替换。
func (a *App) stageUpdate(dir string, result update.Result) (string, error) {
	exe := filepath.Join(dir, result.ExeAsset.Name)
	if info, err := os.Stat(exe); err != nil || info.IsDir() {
		return dir, fmt.Errorf("更新包中缺少 %s，请重试或到发布页手动更新", result.ExeAsset.Name)
	}
	target, err := a.cfg.StageUpdate(exe)
	if err != nil {
		return dir, err
	}
	a.updateMu.Lock()
	a.update.packagePath = exe
	a.update.targetPath = target
	a.updateMu.Unlock()
	return dir, nil
}

// SetUpdateInstaller 接入应用内更新：stage 登记已校验的更新包并返回落地路径，
// finish 在退出的最后一步执行落地并返回应启动的程序路径。
//
// 由 cmd 层装配：服务层不知道 exe 在哪、也不能在退出时自己动手，那样会与
// 数据库关闭和单实例锁释放的先后顺序打架。
func (a *App) SetUpdateInstaller(stage func(string) (string, error), finish func() string) {
	a.cfg.StageUpdate = stage
	a.cfg.FinishUpdate = finish
}

// ReportUpdateFailure 报告上次退出时更新落地失败，写进状态让界面提示一次。
func (s *UpdateService) ReportUpdateFailure() {
	s.a.updateMu.Lock()
	s.a.update.errMsg = "上次退出时未能完成更新，仍在运行原版本。请到发布页手动下载，或把 AgentDeck 移到有写入权限的目录后重试。"
	view := s.a.updateViewLocked()
	s.a.updateMu.Unlock()
	s.a.emit(EventUpdateStatus, view)
}

// RestartToInstall 在退出时落地已登记的更新包，并重启到新版本。
//
// 与「数据位置迁移后重启」共用 Config.Restart：它走正常退出路径，因此
// 数据库会关闭、单实例锁会释放，新实例不会因为抢不到锁直接退出。
func (s *UpdateService) RestartToInstall() error {
	a := s.a
	a.updateMu.Lock()
	ready := a.update.packagePath != ""
	a.updateMu.Unlock()
	if !ready {
		return errors.New("当前没有待安装的更新，请先下载")
	}
	if a.cfg.Restart == nil {
		return errors.New("当前运行方式不支持自动重启，请手动退出并重新打开")
	}
	return a.cfg.Restart()
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
