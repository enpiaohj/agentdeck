package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
)

// 每个测试使用独立应用配置；回调仅修改内存，不访问注册表。
func desktopSettingsFixture(t *testing.T) (*App, *bool, *[]bool) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	state := false
	calls := []bool{}
	a := &App{settings: appconfig.Default(), cfg: Config{
		TrayAvailable:      true,
		StartAtLoginStatus: func() (bool, error) { return state, nil },
		SetStartAtLogin: func(enabled bool) error {
			calls = append(calls, enabled)
			state = enabled
			return nil
		},
	}}
	return a, &state, &calls
}

func TestDesktopSettingsDefaultReadHasNoSideEffects(t *testing.T) {
	a, _, calls := desktopSettingsFixture(t)
	view, err := a.NewSettingsService().Desktop()
	if err != nil || view.StartAtLogin || view.CloseToTray || !view.StartAtLoginSupported || !view.TraySupported {
		t.Fatalf("默认桌面选项：%+v, %v", view, err)
	}
	if a.CloseToTrayEnabled() || len(*calls) != 0 {
		t.Fatalf("默认设置或只读操作产生副作用：%v", *calls)
	}
	path, _ := appconfig.Path()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("读取默认设置不应创建配置文件：%v", err)
	}
}

func TestDesktopSettingsReadActualRegistration(t *testing.T) {
	a, state, _ := desktopSettingsFixture(t)
	a.settings.StartAtLogin = true
	view, err := a.NewSettingsService().Desktop()
	if err != nil || view.StartAtLogin {
		t.Fatalf("注册已不存在时不应报告自启动已开启：%+v, %v", view, err)
	}
	*state = true
	a.settings.StartAtLogin = false
	view, err = a.NewSettingsService().Desktop()
	if err != nil || !view.StartAtLogin {
		t.Fatalf("应报告实际注册状态：%+v, %v", view, err)
	}
}

func TestDesktopSettingsSetAndPersistWithoutClobber(t *testing.T) {
	a, state, calls := desktopSettingsFixture(t)
	path, _ := appconfig.Path()
	root := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"data_root": root, "terminal_shell": "pwsh", "future": map[string]int{"x": 1}})
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var err error
	a.settings, err = appconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	svc := a.NewSettingsService()
	for _, enabled := range []bool{true, false} {
		view, err := svc.SetStartAtLogin(enabled)
		if err != nil || view.StartAtLogin != enabled || *state != enabled {
			t.Fatalf("自启动设置：%+v, %v", view, err)
		}
		view, err = svc.SetCloseToTray(enabled)
		if err != nil || view.CloseToTray != enabled || a.CloseToTrayEnabled() != enabled {
			t.Fatalf("托盘设置未立即生效：%+v, %v", view, err)
		}
		cfg, err := appconfig.Load()
		if err != nil || cfg.StartAtLogin != enabled || cfg.CloseToTray != enabled || cfg.DataRoot != root || cfg.TerminalShell != "pwsh" {
			t.Fatalf("配置丢失或未保存：%+v, %v", cfg, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(raw), `"future"`) {
			t.Fatalf("未知字段丢失：%s, %v", raw, err)
		}
	}
	if !reflect.DeepEqual(*calls, []bool{true, false}) {
		t.Fatalf("非自启动设置产生注册副作用：%v", *calls)
	}
}

func TestDesktopSettingsOSFailureDoesNotPersist(t *testing.T) {
	a, _, _ := desktopSettingsFixture(t)
	a.cfg.SetStartAtLogin = func(bool) error { return errors.New("registration denied") }
	if _, err := a.NewSettingsService().SetStartAtLogin(true); err == nil || !strings.Contains(err.Error(), "registration denied") {
		t.Fatalf("注册错误应向上返回：%v", err)
	}
	if a.settings.StartAtLogin {
		t.Fatal("注册失败不应修改内存")
	}
	path, _ := appconfig.Path()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("注册失败不应写配置：%v", err)
	}
}

func TestDesktopSettingsStatusFailureDoesNotWrite(t *testing.T) {
	a, _, calls := desktopSettingsFixture(t)
	a.cfg.StartAtLoginStatus = func() (bool, error) { return false, errors.New("status denied") }
	svc := a.NewSettingsService()
	if _, err := svc.Desktop(); err == nil || !strings.Contains(err.Error(), "status denied") {
		t.Fatalf("查询错误应返回：%v", err)
	}
	if _, err := svc.SetStartAtLogin(true); err == nil || len(*calls) != 0 {
		t.Fatalf("无法确认原状态时不应注册：%v, %v", err, *calls)
	}
}

func TestDesktopSettingsSaveFailureRollsBackActualOSState(t *testing.T) {
	for _, previous := range []bool{false, true} {
		t.Run(map[bool]string{false: "previous_off", true: "previous_on"}[previous], func(t *testing.T) {
			a, state, calls := desktopSettingsFixture(t)
			*state = previous
			a.settings.StartAtLogin = !previous // 回滚依据实际 OS 状态，不依赖缓存。
			path, _ := appconfig.Path()
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := a.NewSettingsService().SetStartAtLogin(!previous); err == nil {
				t.Fatal("配置路径为目录时保存应失败")
			}
			if *state != previous || !reflect.DeepEqual(*calls, []bool{!previous, previous}) {
				t.Fatalf("未回滚实际 OS 状态：%v, %v", *state, *calls)
			}
			if a.settings.StartAtLogin != !previous {
				t.Fatal("保存失败不应修改内存配置")
			}
			if _, err := a.NewSettingsService().SetCloseToTray(true); err == nil || a.CloseToTrayEnabled() {
				t.Fatalf("托盘保存失败不应生效：%v", err)
			}
		})
	}
}

func TestDesktopSettingsSaveFailureReportsRollbackFailure(t *testing.T) {
	a, _, _ := desktopSettingsFixture(t)
	path, _ := appconfig.Path()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.cfg.SetStartAtLogin = func(bool) error {
		calls++
		if calls == 2 {
			return errors.New("rollback denied")
		}
		return nil
	}
	if _, err := a.NewSettingsService().SetStartAtLogin(true); err == nil || !strings.Contains(err.Error(), "rollback denied") || !strings.Contains(err.Error(), "配置") {
		t.Fatalf("保存与回滚失败均须报告：%v", err)
	}
}

func TestDesktopSettingsCorruptConfigRejectsReadAndChanges(t *testing.T) {
	a, _, calls := desktopSettingsFixture(t)
	path, _ := appconfig.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte("{broken")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	a.settings, a.settingsErr = appconfig.Load()
	svc := a.NewSettingsService()
	if _, err := svc.Desktop(); err == nil {
		t.Fatal("配置损坏时读取不应静默成功")
	}
	if _, err := svc.SetStartAtLogin(true); err == nil || len(*calls) != 0 {
		t.Fatalf("配置损坏时不得注册：%v, %v", err, *calls)
	}
	if _, err := svc.SetCloseToTray(true); err == nil || a.CloseToTrayEnabled() {
		t.Fatalf("配置损坏时不得修改托盘设置：%v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(raw) {
		t.Fatalf("损坏的配置被覆盖：%s, %v", got, err)
	}
}

func TestDesktopSettingsUnsupportedCapabilities(t *testing.T) {
	for _, missing := range []string{"status", "setter", "both"} {
		t.Run(missing, func(t *testing.T) {
			a, _, calls := desktopSettingsFixture(t)
			a.cfg.TrayAvailable = false
			a.settings.CloseToTray = true
			if missing != "setter" {
				a.cfg.StartAtLoginStatus = nil
			}
			if missing != "status" {
				a.cfg.SetStartAtLogin = nil
			}
			view, err := a.NewSettingsService().Desktop()
			if err != nil || view.StartAtLoginSupported || view.TraySupported || a.CloseToTrayEnabled() {
				t.Fatalf("未注入能力必须降级：%+v, %v", view, err)
			}
			if _, err := a.NewSettingsService().SetStartAtLogin(true); err == nil || len(*calls) != 0 {
				t.Fatalf("不支持时应拒绝注册：%v, %v", err, *calls)
			}
			if _, err := a.NewSettingsService().SetCloseToTray(true); err == nil {
				t.Fatal("不支持托盘时应拒绝启用")
			}
		})
	}
}

func TestDesktopSettingsSurviveShellAndMigrationWrites(t *testing.T) {
	a, _, _ := desktopSettingsFixture(t)
	svc := a.NewSettingsService()
	if _, err := svc.SetStartAtLogin(true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetCloseToTray(true); err != nil {
		t.Fatal(err)
	}
	if err := svc.saveTerminalShell(appconfig.ShellPwsh); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := a.switchDataRoot(root); err != nil {
		t.Fatal(err)
	}
	cfg, err := appconfig.Load()
	if err != nil || !cfg.StartAtLogin || !cfg.CloseToTray || cfg.TerminalShell != appconfig.ShellPwsh || cfg.DataRoot != root {
		t.Fatalf("Shell/数据根写入丢失桌面配置：%+v, %v", cfg, err)
	}
}
