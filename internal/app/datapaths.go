package app

import (
	"fmt"
	"path/filepath"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
)

// dataPaths 是启动时解析出的应用配置与数据落点。
type dataPaths struct {
	// cfg 是应用配置；读取失败时为默认值。
	cfg appconfig.Config
	// cfgErr 是配置读取失败的原因。非空时界面须提示，且不得回写配置——
	// 否则会把用户尚未修复的文件整体覆盖。
	cfgErr error
	// paths 是各数据库与数据目录的实际落点。
	paths appconfig.Paths
	// note 是面向用户的数据位置说明（配置损坏或配置根不可用时的降级原因）。
	note string
}

// pathsFromConfig 由实际生效的配置给出各数据落点，供数据位置迁移与清理
// 使用。装配后的 cfg 各路径都已填好，因此这里不再区分默认位置与配置根，
// 只按真实路径归类：标注库与项目声明在配置目录，索引、库与导入副本在
// 本机目录；测试等显式给出路径时，两个基准就是那些路径所在的目录。
func pathsFromConfig(cfg Config) appconfig.Paths {
	configDir := filepath.Dir(cfg.MarksPath)
	localDir := filepath.Dir(cfg.IndexPath)
	if cfg.IndexPath == "" || cfg.IndexPath == ":memory:" {
		localDir = configDir
	}
	return appconfig.Paths{
		ConfigDir:    configDir,
		LocalDir:     localDir,
		MarksPath:    cfg.MarksPath,
		ProjectsPath: cfg.ProjectsPath,
		IndexPath:    cfg.IndexPath,
		LibraryPath:  cfg.LibraryPath,
		ImportDir:    cfg.ImportDir,
	}
}

// resolveDataPaths 读取应用配置并解析数据落点。
//
// 三种情况：
//   - 配置正常：使用配置的数据根（为空即默认位置）。
//   - 配置损坏：按默认位置启动，并在 note 中说明原因。
//   - 配置的数据根不可用（例如盘符不存在）：本次临时使用默认位置，note 说明
//     原因；用户可在「设置 → 数据与诊断」改回默认位置或重新指定。
//
// 迁移数据位置时旧位置一律保留，因此回退默认位置仍能看到原有数据。
func resolveDataPaths() (dataPaths, error) {
	var r dataPaths
	cfg, cfgErr := appconfig.Load()
	if cfgErr != nil {
		r.cfgErr = cfgErr
		cfg = appconfig.Default()
		r.note = "应用配置无法读取，本次使用默认数据位置：" + cfgErr.Error()
	}
	r.cfg = cfg
	paths, err := appconfig.Resolve(cfg)
	if err != nil {
		return r, err
	}
	if ensureErr := paths.Ensure(); ensureErr != nil {
		if cfg.DataRoot == "" {
			return r, ensureErr
		}
		r.note = fmt.Sprintf("配置的数据位置 %s 不可用（%v），本次临时使用默认位置；可在「设置 → 数据与诊断」中改回默认位置。", cfg.DataRoot, ensureErr)
		fallback, fallbackErr := appconfig.DefaultPaths()
		if fallbackErr != nil {
			return r, fallbackErr
		}
		if fallbackErr = fallback.Ensure(); fallbackErr != nil {
			return r, fallbackErr
		}
		r.paths = fallback
		return r, nil
	}
	r.paths = paths
	return r, nil
}
