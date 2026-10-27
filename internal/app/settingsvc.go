package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
)

// SettingsService 是应用级设置的门面。
//
// 与前端偏好（浏览器 localStorage）不同，这些设置由 Go 侧持有：启动终端
// 发生在 Go 侧，数据落点更要由 Go 侧在打开任何数据库之前决定。
type SettingsService struct{ a *App }

// shellValues 是预置的三个终端 shell 取值，顺序即界面展示顺序。
var shellValues = []string{appconfig.ShellAuto, appconfig.ShellPowerShell, appconfig.ShellPwsh}

// TerminalShell 返回终端 shell 设置的当前状态，并探测各选项的版本。
func (s *SettingsService) TerminalShell() TerminalShellView {
	setting := s.a.terminalShellSetting()
	view := TerminalShellView{Setting: setting, ResumeMaximized: s.a.resumeMaximized()}
	view.Exe, view.Version, view.Error = s.shellState(setting)
	for _, value := range shellValues {
		option := TerminalShellOption{Value: value}
		option.Exe, option.Version, option.Error = s.shellState(value)
		view.Options = append(view.Options, option)
	}
	return view
}

// shellState 解析一个取值的可执行文件与版本；不可用时给出一句话原因。
func (s *SettingsService) shellState(value string) (exe, version, reason string) {
	exe, err := resolveShellExe(value)
	if err != nil {
		return "", "", err.Error()
	}
	version, err = s.a.shellVersionOf(exe)
	if err != nil {
		return exe, "", fmt.Sprintf("无法获取 %s 的版本：%v", filepath.Base(exe), err)
	}
	return exe, version, ""
}

// Migrate 把 AgentDeck 自有数据迁移到 picked 目录下固定的 AgentDeck\Data，
// 成功后写入数据位置指针。picked 是用户在目录选择器里选的目录，不是最终数据
// 根：补全规则见 storageDestination。详细步骤与不变量见 migrate.go；进度经
// storage:progress 推送。
//
// 运行中的进程仍写旧位置的库，因此迁移完成后必须重启才生效——界面据此
// 给出「立即重启」。旧位置一律保留，清理是另行确认的动作。
func (s *SettingsService) Migrate(picked string) (MigrateReport, error) {
	report, err := s.a.migrateStorage(picked)
	if err != nil {
		return MigrateReport{}, err
	}
	return report, nil
}

// ResolveDestination 把用户选定的目录补全成实际数据根，并如实报告那里当前的
// 状态：不存在（迁移时创建）、已存在且为空（可用）、已存在且有内容（不可用，
// 迁移绝不覆盖）。
//
// 目录是否已存在必须由 Go 侧判断并交给界面：用户可能正好选到上一次迁移用过
// 的位置，那时里面已经是完整的 AgentDeck 数据，静默追加或覆盖都会出问题。
func (s *SettingsService) ResolveDestination(picked string) DestinationView {
	dest := storageDestination(picked)
	view := DestinationView{
		Picked: strings.TrimSpace(picked),
		Dest:   dest,
		Leaf:   storageLeafRelative,
		Rule:   storageRule,
	}
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		view.Exists = true
		view.Empty = true
		if entries, err := os.ReadDir(dest); err == nil && len(entries) > 0 {
			view.Empty = false
		}
		view.Bytes, view.Files, _ = usage(dest)
	} else {
		// 不存在（或同名文件）时没有会被覆盖的内容：是否为空只看已存在的目录。
		view.Empty = true
	}
	view.Reason = s.a.destinationProblem(dest)
	view.Ready = view.Reason == ""
	return view
}

// ResetRoot 把数据位置改回默认位置。只改指针，不搬数据：默认位置没有
// 数据时会得到一个空库，因此界面必须先把 DefaultHasData 如实告诉用户。
func (s *SettingsService) ResetRoot() error {
	a := s.a
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settingsErr != nil {
		return fmt.Errorf("应用配置文件无法读取，未修改数据位置：%v", a.settingsErr)
	}
	cfg := a.settings
	if cfg.DataRoot == "" {
		return nil
	}
	previous := append([]string(nil), cfg.PreviousDirs...)
	for _, dir := range a.storageData() {
		if !containsPath(previous, dir) {
			previous = append(previous, dir)
		}
	}
	cfg.DataRoot = ""
	// 改回默认位置后，原先记下的默认位置目录又成了当前目录：从旧位置清单里
	// 去掉，否则清理会被整批拒绝（见 prunePreviousDirs）。
	cfg.PreviousDirs = prunePreviousDirs(previous, cfg)
	if err := appconfig.Save(cfg); err != nil {
		return fmt.Errorf("写入应用配置失败：%w", err)
	}
	a.settings = cfg
	return nil
}

// Restart 重启应用，让新的数据位置生效。未注入重启能力时（例如测试）
// 返回错误，界面据此提示手动重启。
func (s *SettingsService) Restart() error {
	if s.a.cfg.Restart == nil {
		return errors.New("当前环境不支持自动重启，请手动退出并重新打开 AgentDeck")
	}
	return s.a.cfg.Restart()
}

// SetTerminalShell 校验并保存终端 shell 设置，立即对新开的终端生效。
//
// 取值：appconfig 定义的三个预置值之一，或自定义可执行文件的绝对路径。
// 自定义路径必须存在且能自报 PowerShell 版本——否则设置页保存的是一个
// 起不来或不是 PowerShell 的东西，失败会推迟到用户恢复会话时才暴露。
func (s *SettingsService) SetTerminalShell(value string) (TerminalShellView, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "":
		value = appconfig.ShellAuto
	case appconfig.ShellPowerShell, appconfig.ShellAuto, appconfig.ShellPwsh:
	default:
		if !filepath.IsAbs(value) {
			return s.TerminalShell(), fmt.Errorf("终端 shell 取值无效：%s", value)
		}
	}
	value = appconfig.NormalizeShell(value)
	exe, err := resolveShellExe(value)
	if err != nil {
		return s.TerminalShell(), err
	}
	if _, err = s.a.shellVersionOf(exe); err != nil {
		return s.TerminalShell(), fmt.Errorf("%s 不是可用的 PowerShell：%v", exe, err)
	}
	if err = s.saveTerminalShell(value); err != nil {
		return s.TerminalShell(), err
	}
	s.a.setTerminalShell(value)
	return s.TerminalShell(), nil
}

// saveTerminalShell 把取值写回应用配置。
//
// 配置读取失败时拒绝写入：此时内存里是默认配置，写回去会把用户尚未修复的
// 文件（可能含本版本不认识的字段）整体覆盖。先写文件再更新内存，两者不会
// 出现内存说已保存、文件里却没有的不一致。
func (s *SettingsService) saveTerminalShell(value string) error {
	a := s.a
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settingsErr != nil {
		return fmt.Errorf("应用配置文件无法读取，未保存设置：%v", a.settingsErr)
	}
	cfg := a.settings
	cfg.TerminalShell = value
	if err := appconfig.Save(cfg); err != nil {
		return fmt.Errorf("写入应用配置失败：%w", err)
	}
	a.settings = cfg
	return nil
}

// SetResumeMaximized 保存「继续会话时最大化打开终端」，立即对新打开的终端生效。
//
// 与终端 shell 不同，这个取值没有可校验的东西：它只是一个显示偏好。但配置
// 读取失败时同样拒绝写入，理由与 saveTerminalShell 一致。
func (s *SettingsService) SetResumeMaximized(value bool) (TerminalShellView, error) {
	a := s.a
	a.settingsMu.Lock()
	if a.settingsErr != nil {
		a.settingsMu.Unlock()
		return s.TerminalShell(), fmt.Errorf("应用配置文件无法读取，未保存设置：%v", a.settingsErr)
	}
	cfg := a.settings
	cfg.ResumeMaximized = value
	if err := appconfig.Save(cfg); err != nil {
		a.settingsMu.Unlock()
		return s.TerminalShell(), fmt.Errorf("写入应用配置失败：%w", err)
	}
	a.settings = cfg
	a.settingsMu.Unlock()
	return s.TerminalShell(), nil
}
