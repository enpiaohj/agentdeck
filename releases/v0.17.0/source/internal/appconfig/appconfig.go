// Package appconfig 保存 AgentDeck 的应用级配置。
//
// 与前端偏好（浏览器 localStorage）不同，这里的配置必须由 Go 侧在打开任何
// 数据库之前读取：数据根目录决定各数据库的落点，终端 shell 决定新建与恢复
// 会话时启动哪个可执行文件。
//
// 引导文件固定位于 %APPDATA%\AgentDeck\app-config.json。数据根目录迁移时
// 该文件不随数据移动，因此不会出现「用配置决定配置自身位置」的循环。
package appconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Version 是当前配置文件格式版本。
const Version = 1

// 终端 shell 的取值。空串与无法识别的取值按 ShellAuto 处理：默认选版本最高的
// 可用 PowerShell。已保存的显式取值（包括 powershell）不受影响。
const (
	// ShellPowerShell 是 Windows PowerShell 5.1（powershell.exe）。
	ShellPowerShell = "powershell"
	// ShellAuto 优先使用 PowerShell 7+（pwsh.exe），不可用时回退 Windows PowerShell。
	ShellAuto = "auto"
	// ShellPwsh 是 PowerShell 7+（pwsh.exe）。
	ShellPwsh = "pwsh"
)

// 终端打开方式的取值。只有 Windows Terminal（wt.exe）能复用已有窗口开标签；
// 回退到直接启动 shell 时一律是新窗口，见 app.terminalCommand。
const (
	// TerminalWindow 每次都开一个新的终端窗口（默认，与引入本设置之前一致）。
	TerminalWindow = "window"
	// TerminalTab 有活动终端窗口时在其中开新标签，没有时开新窗口。
	TerminalTab = "tab"
)

// Config 是应用级配置。DataRoot 为空表示使用默认位置
// （%APPDATA%\AgentDeck 与 %LOCALAPPDATA%\AgentDeck 两处）。
type Config struct {
	Version       int    `json:"version"`
	DataRoot      string `json:"data_root,omitempty"`
	TerminalShell string `json:"terminal_shell,omitempty"`
	// 桌面生命周期选项均为显式启用，旧配置缺少字段时默认关闭。
	StartAtLogin bool `json:"start_at_login"`
	CloseToTray  bool `json:"close_to_tray"`
	// ResumeMaximized 让「继续会话」打开的终端最大化显示。默认关闭：铺满
	// 屏幕是需要用户明确要求的显示方式，不能作为默认行为。
	ResumeMaximized bool `json:"resume_maximized"`
	// ResumeOpenMode 决定继续会话时是开新终端窗口还是在活动窗口里开新标签。
	// 空串按 TerminalWindow 处理，与引入本设置之前的行为一致。
	ResumeOpenMode string `json:"resume_open_mode,omitempty"`
	// AutoCheckUpdates 控制启动后的后台检查更新。与上面两项相反，它默认
	// 开启，因此旧配置缺少该字段时按 Default() 的取值保留，而不是 false。
	AutoCheckUpdates bool `json:"auto_check_updates"`
	// PreviousDirs 是上一次迁移数据位置之前实际使用的数据目录（默认位置
	// 时为 ConfigDir 与 LocalDir 两个）。迁移一律保留旧位置，这里记下来
	// 供界面提示与「清理旧位置数据」使用；清理是显式动作，不自动执行。
	PreviousDirs []string `json:"previous_dirs,omitempty"`

	// extra 保存本版本不认识的字段，写回时原样保留，避免降级运行时丢配置。
	extra map[string]json.RawMessage
}

// Default 返回默认配置：默认数据位置，终端优先使用 PowerShell 7+，
// 启动后检查更新。
func Default() Config {
	return Config{Version: Version, TerminalShell: ShellAuto, ResumeOpenMode: TerminalWindow, AutoCheckUpdates: true}
}

// NormalizeOpenMode 把终端打开方式归一化：空串与无法识别的取值回退到
// 「新窗口」。
func NormalizeOpenMode(value string) string {
	if strings.TrimSpace(value) == TerminalTab {
		return TerminalTab
	}
	return TerminalWindow
}

// NormalizeShell 把终端 shell 取值归一化：空串与无法识别的取值回退到
// 自动（优先 7+）；绝对路径（自定义可执行文件）原样保留，存在性由
// 启动终端时校验。
func NormalizeShell(value string) string {
	v := strings.TrimSpace(value)
	switch v {
	case "", ShellAuto:
		return ShellAuto
	case ShellPowerShell, ShellPwsh:
		return v
	}
	if filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	return ShellAuto
}

// Path 返回引导文件位置：%APPDATA%\AgentDeck\app-config.json。
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "AgentDeck", "app-config.json"), nil
}

// Load 读取引导文件。文件不存在时返回 Default() 且不报错；内容损坏或
// 无法读取时返回 Default() 与错误，由调用方决定如何提示。
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	return LoadFrom(path)
}

// LoadFrom 与 Load 相同，但显式指定配置文件路径（供测试与工具使用）。
func LoadFrom(path string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return cfg, fmt.Errorf("应用配置文件不是有效的 JSON：%w", err)
	}
	for key, target := range map[string]*string{
		"data_root":        &cfg.DataRoot,
		"terminal_shell":   &cfg.TerminalShell,
		"resume_open_mode": &cfg.ResumeOpenMode,
	} {
		if value, ok := fields[key]; ok {
			if err = json.Unmarshal(value, target); err != nil {
				return Default(), fmt.Errorf("应用配置字段 %s 无效：%w", key, err)
			}
		}
		delete(fields, key)
	}
	for key, target := range map[string]*bool{
		"start_at_login":     &cfg.StartAtLogin,
		"close_to_tray":      &cfg.CloseToTray,
		"auto_check_updates": &cfg.AutoCheckUpdates,
		"resume_maximized":   &cfg.ResumeMaximized,
	} {
		if value, ok := fields[key]; ok {
			if err = json.Unmarshal(value, target); err != nil {
				return Default(), fmt.Errorf("应用配置字段 %s 无效：%w", key, err)
			}
		}
		delete(fields, key)
	}
	if value, ok := fields["previous_dirs"]; ok {
		if err = json.Unmarshal(value, &cfg.PreviousDirs); err != nil {
			return Default(), fmt.Errorf("应用配置字段 previous_dirs 无效：%w", err)
		}
		delete(fields, "previous_dirs")
	}
	if value, ok := fields["version"]; ok {
		if err = json.Unmarshal(value, &cfg.Version); err != nil {
			cfg.Version = Version
		}
		delete(fields, "version")
	}
	cfg.DataRoot = strings.TrimSpace(cfg.DataRoot)
	cfg.TerminalShell = NormalizeShell(cfg.TerminalShell)
	cfg.ResumeOpenMode = NormalizeOpenMode(cfg.ResumeOpenMode)
	cfg.extra = fields
	return cfg, nil
}

// Save 写入引导文件。
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SaveTo(path, cfg)
}

// SaveTo 与 Save 相同，但显式指定配置文件路径。写入采用「同目录临时文件
// + 原子改名」，中途失败不会留下半截配置。
func SaveTo(path string, cfg Config) error {
	fields := make(map[string]json.RawMessage, len(cfg.extra)+3)
	for key, value := range cfg.extra {
		fields[key] = value
	}
	version, _ := json.Marshal(Version)
	fields["version"] = version
	if cfg.DataRoot != "" {
		raw, _ := json.Marshal(filepath.Clean(strings.TrimSpace(cfg.DataRoot)))
		fields["data_root"] = raw
	}
	shell, _ := json.Marshal(NormalizeShell(cfg.TerminalShell))
	fields["terminal_shell"] = shell
	startAtLogin, _ := json.Marshal(cfg.StartAtLogin)
	fields["start_at_login"] = startAtLogin
	closeToTray, _ := json.Marshal(cfg.CloseToTray)
	fields["close_to_tray"] = closeToTray
	autoCheck, _ := json.Marshal(cfg.AutoCheckUpdates)
	fields["auto_check_updates"] = autoCheck
	maximized, _ := json.Marshal(cfg.ResumeMaximized)
	fields["resume_maximized"] = maximized
	openMode, _ := json.Marshal(NormalizeOpenMode(cfg.ResumeOpenMode))
	fields["resume_open_mode"] = openMode
	if len(cfg.PreviousDirs) > 0 {
		dirs := make([]string, 0, len(cfg.PreviousDirs))
		for _, dir := range cfg.PreviousDirs {
			if dir = filepath.Clean(strings.TrimSpace(dir)); filepath.IsAbs(dir) && !slices.Contains(dirs, dir) {
				dirs = append(dirs, dir)
			}
		}
		if len(dirs) > 0 {
			raw, _ := json.Marshal(dirs)
			fields["previous_dirs"] = raw
		}
	}

	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "app-config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(name)
	}
	if err = tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err = tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err = tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err = os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Paths 是 AgentDeck 自有数据的全部落点。
type Paths struct {
	// DataRoot 是配置的数据根；为空表示默认位置（ConfigDir 与 LocalDir 分开）。
	DataRoot string
	// ConfigDir 保存标注库与项目声明。
	ConfigDir string
	// LocalDir 保存全文索引、导入副本与永久会话库。
	LocalDir string

	MarksPath    string
	ProjectsPath string
	IndexPath    string
	LibraryPath  string
	ImportDir    string
}

// Resolve 由配置计算全部数据落点。
//
// 未配置数据根时与引入本设置之前逐字节一致：标注库与项目声明位于
// %APPDATA%\AgentDeck，索引、导入副本与永久会话库位于 %LOCALAPPDATA%\AgentDeck。
// 配置数据根后两者合并为该目录，即迁移后的布局。
func Resolve(cfg Config) (Paths, error) {
	root := strings.TrimSpace(cfg.DataRoot)
	var configDir, localDir string
	if root != "" {
		if !filepath.IsAbs(root) {
			return Paths{}, fmt.Errorf("数据根目录必须是绝对路径：%s", root)
		}
		root = filepath.Clean(root)
		configDir, localDir = root, root
	} else {
		base, err := os.UserConfigDir() // Windows 上即 %APPDATA%
		if err != nil {
			return Paths{}, err
		}
		configDir = filepath.Join(base, "AgentDeck")
		cache, err := os.UserCacheDir() // Windows 上即 %LOCALAPPDATA%
		if err != nil {
			return Paths{}, err
		}
		localDir = filepath.Join(cache, "AgentDeck")
	}
	return Paths{
		DataRoot:     root,
		ConfigDir:    configDir,
		LocalDir:     localDir,
		MarksPath:    filepath.Join(configDir, "marks.db"),
		ProjectsPath: filepath.Join(configDir, "projects.db"),
		IndexPath:    filepath.Join(localDir, "index.db"),
		LibraryPath:  filepath.Join(localDir, "library", "history.db"),
		ImportDir:    filepath.Join(localDir, "imports", "claude"),
	}, nil
}

// Ensure 创建配置目录与本机数据目录（已存在时不报错）。
func (p Paths) Ensure() error {
	for _, dir := range []string{p.ConfigDir, p.LocalDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// DefaultPaths 返回默认位置（不读取配置），用于界面展示「默认位置」。
func DefaultPaths() (Paths, error) { return Resolve(Default()) }
