package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileIsDefault(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "app-config.json"))
	if err != nil {
		t.Fatalf("文件不存在不应报错：%v", err)
	}
	if cfg.TerminalShell != ShellPowerShell || cfg.DataRoot != "" || cfg.Version != Version {
		t.Fatalf("应得到默认配置，实际为 %+v", cfg)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "app-config.json")
	want := Config{DataRoot: `D:\AgentDeckData`, TerminalShell: ShellPwsh}
	if err := SaveTo(path, want); err != nil {
		t.Fatalf("保存失败：%v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	if got.DataRoot != `D:\AgentDeckData` || got.TerminalShell != ShellPwsh || got.Version != Version {
		t.Fatalf("往返不一致：%+v", got)
	}
}

func TestUnknownFieldsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-config.json")
	original := `{"version":1,"terminal_shell":"pwsh","future_field":{"a":1}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	cfg.DataRoot = `E:\DeckData`
	if err = SaveTo(path, cfg); err != nil {
		t.Fatalf("保存失败：%v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"future_field"`) || !strings.Contains(text, `"a": 1`) {
		t.Fatalf("未知字段被丢弃：%s", text)
	}
	if !strings.Contains(text, `"E:\\DeckData"`) {
		t.Fatalf("新字段未写入：%s", text)
	}
}

func TestCorruptFileReturnsDefaultAndError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err == nil {
		t.Fatal("损坏的配置应返回错误")
	}
	if cfg.TerminalShell != ShellPowerShell || cfg.DataRoot != "" {
		t.Fatalf("损坏时应回退默认配置：%+v", cfg)
	}
}

func TestNormalizeShell(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "pwsh.exe")
	cases := []struct{ in, want string }{
		{"", ShellPowerShell},
		{"  ", ShellPowerShell},
		{ShellPowerShell, ShellPowerShell},
		{ShellAuto, ShellAuto},
		{ShellPwsh, ShellPwsh},
		{custom, filepath.Clean(custom)},
		{"bash", ShellPowerShell},
		{"relative/pwsh.exe", ShellPowerShell},
	}
	for _, c := range cases {
		if got := NormalizeShell(c.in); got != c.want {
			t.Errorf("NormalizeShell(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestPathsDefaultLayout 固定默认布局：标注库与项目声明在 %APPDATA%\AgentDeck，
// 索引、导入副本与永久会话库在 %LOCALAPPDATA%\AgentDeck。与引入数据根设置
// 之前的落点必须逐字节一致。
func TestPathsDefaultLayout(t *testing.T) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	configBase := filepath.Join(cfgDir, "AgentDeck")
	localBase := filepath.Join(cacheDir, "AgentDeck")

	p, err := Resolve(Default())
	if err != nil {
		t.Fatalf("计算路径失败：%v", err)
	}
	want := Paths{
		DataRoot:     "",
		ConfigDir:    configBase,
		LocalDir:     localBase,
		MarksPath:    filepath.Join(configBase, "marks.db"),
		ProjectsPath: filepath.Join(configBase, "projects.db"),
		IndexPath:    filepath.Join(localBase, "index.db"),
		LibraryPath:  filepath.Join(localBase, "library", "history.db"),
		ImportDir:    filepath.Join(localBase, "imports", "claude"),
	}
	if p != want {
		t.Fatalf("默认布局发生变化：\n得到 %+v\n期望 %+v", p, want)
	}
}

func TestPathsConfiguredRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "DeckData")
	p, err := Resolve(Config{DataRoot: root})
	if err != nil {
		t.Fatalf("计算路径失败：%v", err)
	}
	if p.ConfigDir != root || p.LocalDir != root || p.DataRoot != filepath.Clean(root) {
		t.Fatalf("配置根后两个基准目录都应等于根：%+v", p)
	}
	for _, path := range []string{p.MarksPath, p.ProjectsPath, p.IndexPath, p.LibraryPath, p.ImportDir} {
		if !strings.HasPrefix(path, filepath.Clean(root)+string(os.PathSeparator)) {
			t.Fatalf("路径未落在配置根下：%s", path)
		}
	}
	if p.LibraryPath != filepath.Join(root, "library", "history.db") || p.ImportDir != filepath.Join(root, "imports", "claude") {
		t.Fatalf("子目录布局不符合迁移后的约定：%+v", p)
	}
	if _, err = Resolve(Config{DataRoot: "relative/data"}); err == nil {
		t.Fatal("相对路径应被拒绝")
	}
}

func TestEnsureCreatesDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "DeckData")
	p, err := Resolve(Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Ensure(); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		t.Fatalf("根目录未创建：%v", statErr)
	}
}

// TestSaveLeavesNoTempFile 确认原子写完成后目录里只剩配置文件。
func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app-config.json")
	if err := SaveTo(path, Default()); err != nil {
		t.Fatalf("保存失败：%v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "app-config.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("目录里残留了临时文件：%v", names)
	}
}
