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
