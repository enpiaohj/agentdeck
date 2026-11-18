package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopSettingsDefaultAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-config.json")
	cfg, err := LoadFrom(path)
	if err != nil || cfg.StartAtLogin || cfg.CloseToTray {
		t.Fatalf("默认桌面设置应关闭：%+v, %v", cfg, err)
	}
	for _, enabled := range []bool{true, false} {
		cfg.StartAtLogin, cfg.CloseToTray = enabled, enabled
		if err := SaveTo(path, cfg); err != nil {
			t.Fatal(err)
		}
		cfg, err = LoadFrom(path)
		if err != nil || cfg.StartAtLogin != enabled || cfg.CloseToTray != enabled {
			t.Fatalf("桌面设置往返不一致：%+v, %v", cfg, err)
		}
		var fields map[string]json.RawMessage
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"start_at_login", "close_to_tray"} {
			if string(fields[key]) != map[bool]string{true: "true", false: "false"}[enabled] {
				t.Fatalf("%s 应明确保存当前布尔值：%s", key, raw)
			}
		}
	}
}

func TestDesktopSettingsRejectInvalidTypes(t *testing.T) {
	for _, key := range []string{"start_at_login", "close_to_tray"} {
		t.Run(key, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app-config.json")
			if err := os.WriteFile(path, []byte(`{"`+key+`":"true"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadFrom(path); err == nil {
				t.Fatal("错误类型应返回配置错误")
			}
		})
	}
}

func TestDesktopSettingsPreservedWithOtherAndUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-config.json")
	raw := []byte(`{"start_at_login":true,"close_to_tray":true,"terminal_shell":"pwsh","future":{"x":1}}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataRoot = t.TempDir()
	if err = SaveTo(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFrom(path)
	if err != nil || !got.StartAtLogin || !got.CloseToTray || got.TerminalShell != ShellPwsh || got.DataRoot != cfg.DataRoot {
		t.Fatalf("其他设置写入后丢失桌面设置：%+v, %v", got, err)
	}
	var future map[string]int
	if err := json.Unmarshal(got.extra["future"], &future); err != nil || future["x"] != 1 {
		t.Fatalf("未知字段丢失：%s, %v", got.extra["future"], err)
	}
}

// TestResumeMaximizedDefaultOffAndRoundTrip 最大化偏好默认关闭，保存后能原样
// 读回；字段必须显式写入，避免降级运行时读不到而按默认处理。
func TestResumeMaximizedDefaultOffAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-config.json")
	cfg, err := LoadFrom(path)
	if err != nil || cfg.ResumeMaximized {
		t.Fatalf("默认应为关闭：%+v, %v", cfg, err)
	}
	for _, enabled := range []bool{true, false} {
		cfg.ResumeMaximized = enabled
		if err := SaveTo(path, cfg); err != nil {
			t.Fatal(err)
		}
		cfg, err = LoadFrom(path)
		if err != nil || cfg.ResumeMaximized != enabled {
			t.Fatalf("最大化偏好往返不一致：%+v, %v", cfg, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if string(fields["resume_maximized"]) != map[bool]string{true: "true", false: "false"}[enabled] {
			t.Fatalf("resume_maximized 应明确保存当前布尔值：%s", raw)
		}
	}
	// 类型不对的取值要报配置错误，而不是静默当成 false
	bad := filepath.Join(t.TempDir(), "app-config.json")
	if err := os.WriteFile(bad, []byte(`{"resume_maximized":"true"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(bad); err == nil {
		t.Fatal("错误类型应返回配置错误")
	}
}

// TestResumeOpenModeNormalizeAndRoundTrip 打开方式默认 window，tab 能原样读回，
// 无法识别的取值按 window 处理（不把界面不认识的值留在文件里）。
func TestResumeOpenModeNormalizeAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-config.json")
	cfg, err := LoadFrom(path)
	if err != nil || cfg.ResumeOpenMode != TerminalWindow {
		t.Fatalf("默认应为新窗口：%+v, %v", cfg, err)
	}
	for _, mode := range []string{TerminalTab, TerminalWindow} {
		cfg.ResumeOpenMode = mode
		if err := SaveTo(path, cfg); err != nil {
			t.Fatal(err)
		}
		got, err := LoadFrom(path)
		if err != nil || got.ResumeOpenMode != mode {
			t.Fatalf("打开方式往返不一致：%+v, %v", got, err)
		}
	}
	for _, bad := range []string{"", "bogus", "TAB"} {
		if got := NormalizeOpenMode(bad); got != TerminalWindow {
			t.Fatalf("%q 应归一化为新窗口，实际 %q", bad, got)
		}
	}
	// 文件里写了无法识别的值时，读出来也要按新窗口处理
	raw := []byte(`{"resume_open_mode":"bogus"}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFrom(path)
	if err != nil || got.ResumeOpenMode != TerminalWindow {
		t.Fatalf("无法识别的取值应按新窗口处理：%+v, %v", got, err)
	}
}
