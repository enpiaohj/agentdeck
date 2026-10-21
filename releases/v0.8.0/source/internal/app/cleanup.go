package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
)

// 清理旧位置：迁移一律保留旧位置，这里提供显式的删除动作。
//
// 不变量：
//   - 只删 storageLayout 里列出的相对路径（含索引的旁文件）及其目录内容，
//     以及这些目录本身在确认已空之后；绝不 RemoveAll，绝不删旧位置里的
//     其他条目。
//   - 旧位置与当前数据目录相同（用户又改回来了）时拒绝执行。
//   - 预览给出 token 绑定清单内容，执行前重新计算，清单变了就整批拒绝。

// indexRel 是全文索引在数据目录下的相对路径；它同时用于布局表与旁文件识别。
const indexRel = "index.db"

// entryTargets 给出一个布局项在数据目录下要清理的具体路径：项本身，索引项
// 还要带上它的旁文件。
func entryTargets(dir, rel string) []string {
	targets := []string{filepath.Join(dir, rel)}
	if rel == indexRel {
		targets = append(targets, indexSidecars(dir)...)
	}
	return targets
}

// indexSidecars 列出数据目录下与全文索引同属可重建项的旁文件
// （`index.db-wal` / `-shm` / `-journal`）。迁移不复制它们（见
// migrationSkipNote），清理也就必须一并删除：只删 index.db 会在旧位置留下
// 一个可重建的日志文件，旧目录因此删不掉，用户看到的「已清理」也不完整。
func indexSidecars(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(strings.ToLower(entry.Name()), indexRel+"-") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	return paths
}

// CleanupPreview 返回将被删除的文件清单与合计。
func (s *SettingsService) CleanupPreview() (CleanupPlan, error) {
	return s.a.cleanupPlan()
}

// CleanupOld 按预览删除旧位置的数据；token 必须是同一次预览的结果。
func (s *SettingsService) CleanupOld(token string) (CleanupReport, error) {
	a := s.a
	plan, err := a.cleanupPlan()
	if err != nil {
		return CleanupReport{}, err
	}
	if plan.Files == 0 {
		return CleanupReport{}, errors.New("旧位置没有可清理的 AgentDeck 数据")
	}
	if plan.Token != token {
		return CleanupReport{}, errors.New("旧位置的内容已变化，请重新预览后再清理")
	}
	var report CleanupReport
	for _, dir := range plan.Dirs {
		for _, entry := range storageLayout {
			for _, target := range entryTargets(dir, entry.Rel) {
				info, statErr := os.Stat(target)
				if statErr != nil {
					continue
				}
				if !info.IsDir() {
					if err := os.Remove(target); err != nil {
						return report, cleanupStopped(&report, target, err)
					}
					report.Files++
					report.Bytes += info.Size()
					continue
				}
				files, bytes, err := removeTree(target)
				report.Files += files
				report.Bytes += bytes
				if err != nil {
					return report, cleanupStopped(&report, target, err)
				}
			}
		}
	}
	// 删掉确认已空的目录：只向上删到旧位置本身，且只删空的。
	report.Dirs = pruneEmpty(plan.Dirs)
	// 旧位置清理完就不再需要提示了。
	a.forgetPreviousDirs(plan.Dirs)
	report.Message = fmt.Sprintf("已清理旧位置的 %d 个文件（%s）；应用配置文件与旧位置中的其他条目未改动。", report.Files, humanBytes(report.Bytes))
	return report, nil
}

// cleanupStopped 说明清理中途停下：已经删掉的部分如实报出来，
// 已删除的文件不回滚（它们都是可重建或已迁移的副本）。
func cleanupStopped(report *CleanupReport, path string, err error) error {
	return fmt.Errorf("清理未完成：已删除 %d 个文件（%s）后停在 %s：%w；可稍后重新预览并继续清理",
		report.Files, humanBytes(report.Bytes), path, err)
}

// cleanupPlan 计算旧位置的待清理清单与 token。
func (a *App) cleanupPlan() (CleanupPlan, error) {
	a.settingsMu.Lock()
	prev := append([]string(nil), a.settings.PreviousDirs...)
	a.settingsMu.Unlock()
	plan := CleanupPlan{Dirs: []string{}}
	var lines []string
	for _, dir := range prev {
		if samePath(dir, a.paths.ConfigDir) || samePath(dir, a.paths.LocalDir) {
			return CleanupPlan{}, fmt.Errorf("旧位置 %s 就是当前数据目录，不能清理", dir)
		}
		plan.Dirs = append(plan.Dirs, dir)
		for _, entry := range storageLayout {
			for _, target := range entryTargets(dir, entry.Rel) {
				for _, file := range filesUnder(target) {
					plan.Files++
					plan.Bytes += file.size
					lines = append(lines, strings.ToLower(file.path)+"|"+fmt.Sprint(file.size))
				}
			}
		}
		plan.Other = append(plan.Other, otherEntries(dir)...)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	plan.Token = hex.EncodeToString(sum[:])
	return plan, nil
}

// forgetPreviousDirs 从配置里移除已清理的旧位置。
func (a *App) forgetPreviousDirs(cleaned []string) {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settingsErr != nil {
		return
	}
	kept := make([]string, 0, len(a.settings.PreviousDirs))
	for _, dir := range a.settings.PreviousDirs {
		if containsPath(cleaned, dir) {
			continue
		}
		kept = append(kept, dir)
	}
	if len(kept) == len(a.settings.PreviousDirs) {
		return
	}
	cfg := a.settings
	cfg.PreviousDirs = kept
	if err := appconfig.Save(cfg); err != nil {
		return
	}
	a.settings = cfg
}

// fileInfo 是清理清单里的一个文件。
type fileInfo struct {
	path string
	size int64
}

// filesUnder 列出一个文件或目录下的全部普通文件；不存在时返回空。
func filesUnder(path string) []fileInfo {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return []fileInfo{{path: path, size: info.Size()}}
	}
	var files []fileInfo
	filepath.WalkDir(path, func(p string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, statErr := entry.Info(); statErr == nil {
			files = append(files, fileInfo{path: p, size: info.Size()})
		}
		return nil
	})
	return files
}

// otherEntries 列出旧位置里不属于 AgentDeck 的顶层条目，供界面说明
// 「这些不会被删除」。
func otherEntries(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	known := []string{"library", "imports"}
	for _, entry := range storageLayout {
		if i := strings.IndexByte(entry.Rel, '/'); i < 0 {
			known = append(known, entry.Rel)
		}
	}
	var other []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if containsName(known, name) {
				continue
			}
			other = append(other, name+string(filepath.Separator))
			continue
		}
		if containsName(known, name) || strings.HasPrefix(strings.ToLower(name), indexRel+"-") {
			continue
		}
		other = append(other, name)
	}
	return other
}

// containsName 是不区分大小写的名字匹配。
func containsName(names []string, name string) bool {
	for _, candidate := range names {
		if strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}

// removeTree 逐个删除目录下的文件，然后自底向上删除已空的目录。
// 返回删除的文件数与字节数；目录非空时保留，绝不递归强删。
func removeTree(root string) (files int, bytes int64, err error) {
	var dirs []string
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return removeErr
		}
		files++
		bytes += info.Size()
		return nil
	})
	if walkErr != nil {
		return files, bytes, walkErr
	}
	// 长的路径更深，先删深的。
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir) // 非空时失败，保留
	}
	return files, bytes, nil
}

// pruneEmpty 自底向上删除已空的旧位置目录，返回删掉的目录数。
func pruneEmpty(dirs []string) int {
	removed := 0
	all := append([]string(nil), dirs...)
	for _, dir := range dirs {
		var subdirs []string
		filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				subdirs = append(subdirs, path)
			}
			return nil
		})
		all = append(all, subdirs...)
	}
	sort.Slice(all, func(i, j int) bool { return len(all[i]) > len(all[j]) })
	for _, dir := range all {
		if err := os.Remove(dir); err == nil {
			removed++
		}
	}
	return removed
}
