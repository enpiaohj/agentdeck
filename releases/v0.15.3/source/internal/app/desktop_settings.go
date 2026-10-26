package app

import (
	"errors"
	"fmt"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
)

// DesktopOptions 是桌面生命周期的当前设置及当前环境支持的能力。
type DesktopOptions struct {
	StartAtLogin          bool `json:"start_at_login"`
	CloseToTray           bool `json:"close_to_tray"`
	StartAtLoginSupported bool `json:"start_at_login_supported"`
	TraySupported         bool `json:"tray_supported"`
}

// Desktop 读取设置，只查询实际注册状态，不创建配置或修改系统自启动。
func (s *SettingsService) Desktop() (DesktopOptions, error) {
	a := s.a
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.desktopOptionsLocked()
}

// desktopOptionsLocked 要求调用方持有 settingsMu。
func (a *App) desktopOptionsLocked() (DesktopOptions, error) {
	view := DesktopOptions{
		CloseToTray:           a.settings.CloseToTray,
		StartAtLoginSupported: a.cfg.StartAtLoginStatus != nil && a.cfg.SetStartAtLogin != nil,
		TraySupported:         a.cfg.TrayAvailable,
	}
	if a.settingsErr != nil {
		return view, fmt.Errorf("应用配置文件无法读取：%w", a.settingsErr)
	}
	if view.StartAtLoginSupported {
		enabled, err := a.cfg.StartAtLoginStatus()
		if err != nil {
			return view, fmt.Errorf("读取开机自启动状态失败：%w", err)
		}
		view.StartAtLogin = enabled
	}
	return view, nil
}

// SetStartAtLogin 由显式用户操作改变系统注册，然后持久化配置。
// 保存失败时恢复操作前的实际注册状态，避免缓存与系统状态不一致。
func (s *SettingsService) SetStartAtLogin(enabled bool) (DesktopOptions, error) {
	a := s.a
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	view, err := a.desktopOptionsLocked()
	if err != nil {
		return view, err
	}
	if !view.StartAtLoginSupported {
		return view, errors.New("当前环境不支持开机自启动")
	}
	previous := view.StartAtLogin
	if err := a.cfg.SetStartAtLogin(enabled); err != nil {
		return view, fmt.Errorf("设置开机自启动失败：%w", err)
	}
	cfg := a.settings
	cfg.StartAtLogin = enabled
	if err := appconfig.Save(cfg); err != nil {
		saveErr := fmt.Errorf("写入应用配置失败：%w", err)
		if rollbackErr := a.cfg.SetStartAtLogin(previous); rollbackErr != nil {
			return view, errors.Join(saveErr, fmt.Errorf("恢复开机自启动状态失败：%w", rollbackErr))
		}
		return view, saveErr
	}
	a.settings = cfg
	view.StartAtLogin = enabled
	return view, nil
}

// SetCloseToTray 持久化关闭窗口行为，保存成功后立即对本次运行生效。
func (s *SettingsService) SetCloseToTray(enabled bool) (DesktopOptions, error) {
	a := s.a
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	view, err := a.desktopOptionsLocked()
	if err != nil {
		return view, err
	}
	if !view.TraySupported {
		return view, errors.New("当前环境不支持关闭到托盘")
	}
	cfg := a.settings
	cfg.CloseToTray = enabled
	if err := appconfig.Save(cfg); err != nil {
		return view, fmt.Errorf("写入应用配置失败：%w", err)
	}
	a.settings = cfg
	view.CloseToTray = enabled
	return view, nil
}

// CloseToTrayEnabled 供窗口关闭处理读取运行中状态；不可用时始终正常关闭。
func (a *App) CloseToTrayEnabled() bool {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.settingsErr == nil && a.cfg.TrayAvailable && a.settings.CloseToTray
}
