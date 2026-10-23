package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
	"github.com/enpiaohj/agentdeck/internal/history"
	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/projects"
)

// 数据存储位置：查看占用、迁移到其他目录、清理旧位置。
//
// 迁移的不变量（改动本文件必须保持）：
//   - 目标数据根固定取所选定目录下的 AgentDeck\Data（见 storageDestination）；
//     所选目录本身叫 AgentDeck 时只追加 Data，已经选到数据根时原样使用。
//   - 目标目录必须是绝对路径、不能是当前数据目录或其子目录、不能包含当前
//     数据目录、不存在或为空目录、可写、剩余空间足够。任一条不满足即拒绝，
//     不动任何文件。
//   - 不复制正在写入的 SQLite 文件：三个库用 VACUUM INTO 生成一致副本。
//   - 全文索引不复制（可重建，且通常是最大的文件）；崩溃标记与应用配置
//     文件不动。
//   - 每个复制出的文件都比对大小与 SHA256，三个库再各做一次结构校验；
//     全部通过才写指针（appconfig 的 data_root），重启后生效。
//   - 失败时只删本次自己写出的文件，源目录一个字节都不动。
//   - 旧位置一律保留，清理是迁移成功后另行确认的动作。

// storageCopyRatio 是迁移所需的余量系数：目标盘剩余空间必须大于待复制
// 数据量的这个倍数，避免复制到一半空间不足。
const storageCopyRatio = 11 // 分子
const storageCopyDiv = 10   // 分母

// indexRebuildNote 是全文索引项的固定说明。
const indexRebuildNote = "可重建：迁移后由 AgentDeck 重新构建，不参与复制"

// storageDir 与 storageLeaf 组成数据根的固定相对路径 AgentDeck\Data：用户选择
// 的是「数据放在哪个目录下」，不是「这个目录叫什么」。固定下来路径可预期，
// 同一台机器上也不会出现多个语义不明的数据目录；父目录 AgentDeck 留出产品
// 自己的位置，要迁移的库与副本都在它下面的 Data 里。
const (
	storageDir  = "AgentDeck"
	storageLeaf = "Data"
)

// storageLeafRelative 是固定的相对路径，界面据此说明数据位置会被放在哪里。
var storageLeafRelative = filepath.Join(storageDir, storageLeaf)

// storageRule 是补全规则的一句话说明，与补全逻辑同放在这里：界面直接展示，
// 不自己拼路径，也不自己拼文案。
var storageRule = fmt.Sprintf("数据位置固定取所选目录下的 %s 子目录；所选目录本身就叫 %s 时只追加 %s。",
	storageLeafRelative, storageDir, storageLeaf)

// storageDestination 把用户选定的目录补全成实际数据根：在其下追加固定的
// AgentDeck\Data；所选目录本身就叫 AgentDeck 时只追加 Data（用户常常直接选到
// `D:\software\AgentDeck`）；已经选到数据根本身时原样返回，既避免再套一层
// `…\Data\AgentDeck\Data`，也让「选到当前数据目录」报成无需迁移，而不是嵌进
// 自己里面。
//
// 只比末级目录名，不做前缀匹配：`D:\AgentDeckData` 得到的是
// `D:\AgentDeckData\AgentDeck\Data`。
func storageDestination(picked string) string {
	picked = strings.TrimSpace(picked)
	if picked == "" {
		return ""
	}
	clean := filepath.Clean(picked)
	// 只剩盘符（`D:`）时要补上根分隔符，否则 Join 得到的是盘符相对路径
	// `D:AgentDeck\Data`，指的是 D 盘当前目录而不是根目录。
	if volume := filepath.VolumeName(clean); volume != "" && !strings.ContainsRune(clean[len(volume):], filepath.Separator) {
		clean = volume + string(filepath.Separator)
	}
	switch base := filepath.Base(clean); {
	case strings.EqualFold(base, storageLeaf) && strings.EqualFold(filepath.Base(filepath.Dir(clean)), storageDir):
		return clean
	case strings.EqualFold(base, storageDir):
		return filepath.Join(clean, storageLeaf)
	}
	return filepath.Join(clean, storageDir, storageLeaf)
}

// storageData 是本次运行实际使用的数据目录（去重后的基准目录）。
func (a *App) storageData() []string {
	dirs := []string{a.paths.ConfigDir}
	if !samePath(a.paths.LocalDir, a.paths.ConfigDir) {
		dirs = append(dirs, a.paths.LocalDir)
	}
	return dirs
}

// samePath 比较两个路径是否指向同一位置（Windows 路径不区分大小写）。
func samePath(a, b string) bool {
	x, y := projects.Normalize(a), projects.Normalize(b)
	return x != "" && strings.EqualFold(x, y)
}

// within 说 path 是否位于 dir 之内（不含 dir 自身）。
func within(path, dir string) bool {
	rel, err := filepath.Rel(projects.Normalize(dir), projects.Normalize(path))
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(strings.ToLower(rel), ".."+string(filepath.Separator))
}

// usage 统计一个文件或目录的字节数与文件数；不存在时返回 0, 0, false。
func usage(path string) (bytes int64, files int, ok bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, false
	}
	if !info.IsDir() {
		return info.Size(), 1, true
	}
	err = filepath.WalkDir(path, func(_ string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, statErr := entry.Info(); statErr == nil {
			bytes += info.Size()
			files++
		}
		return nil
	})
	return bytes, files, err == nil
}

// storageItem 组装一个展示项。
func storageItem(name, path, note string, rebuild bool) StorageItem {
	bytes, files, present := usage(path)
	return StorageItem{Name: name, Path: path, Bytes: bytes, Files: files, Present: present, Rebuild: rebuild, Note: note}
}

// storageLayout 是数据根下的固定布局：一个目录里的哪些相对路径属于
// AgentDeck。列出与清理都按这张表走，绝不做目录级删除。
var storageLayout = []struct {
	Name    string
	Rel     string
	Note    string
	Rebuild bool
}{
	{"永久会话库", "library/history.db", "恢复与保留的会话正文", false},
	{"删除备份", "library/deletions", "手动删除会话前写入的备份", false},
	{"恢复前备份", "library/data-recovery", "恢复完整备份前的自动备份", false},
	{"全文索引", indexRel, indexRebuildNote, true},
	{"Claude 导入副本", "imports/claude", "导入的 Claude 会话副本", false},
	{"标注库", "marks.db", "收藏、归档、标签、备注与本地标题", false},
	{"项目声明", "projects.db", "声明但尚无会话的项目目录", false},
	{"崩溃标记", "session.lock", "运行标记，正常退出时删除", false},
}

// storageItems 列出某个数据基准目录里的各项占用。dir 可以是当前的数据
// 目录，也可以是迁移前的旧位置——两者布局相同，因此同一张表通用。
func (a *App) storageItems(dir string) []StorageItem {
	items := make([]StorageItem, 0, len(storageLayout))
	for _, entry := range storageLayout {
		items = append(items, storageItem(entry.Name, filepath.Join(dir, entry.Rel), entry.Note, entry.Rebuild))
	}
	return items
}

// Storage 返回数据存储位置的完整状态。
func (s *SettingsService) Storage() StorageView {
	a := s.a
	view := StorageView{
		ConfigDir: a.paths.ConfigDir,
		LocalDir:  a.paths.LocalDir,
		Leaf:      storageLeafRelative,
		Note:      a.storageNote,
	}
	a.settingsMu.Lock()
	view.Root = a.settings.DataRoot
	prev := append([]string(nil), a.settings.PreviousDirs...)
	a.settingsMu.Unlock()
	view.UsingDefault = view.Root == ""
	view.Restartable = a.cfg.Restart != nil
	if defaults, err := appconfig.DefaultPaths(); err == nil {
		view.DefaultConfigDir, view.DefaultLocalDir = defaults.ConfigDir, defaults.LocalDir
		view.DefaultHasData = anyPresent(a.storageItems(defaults.ConfigDir)) || anyPresent(a.storageItems(defaults.LocalDir))
	}
	if free, err := diskFree(a.paths.ConfigDir); err == nil {
		view.FreeBytes = free
	}
	// 当前两个基准目录按路径去重后合并成一份清单（默认位置时两者不同）。
	seen := map[string]bool{}
	for _, dir := range a.storageData() {
		for _, item := range a.storageItems(dir) {
			key := strings.ToLower(item.Path)
			if seen[key] {
				continue
			}
			seen[key] = true
			view.Items = append(view.Items, item)
			view.TotalBytes += item.Bytes
		}
	}
	view.PrevDirs = prev
	// 迁移后尚未重启时，旧位置仍是本次运行正在使用的目录：如实列出来，
	// 但清理必须等到重启之后（那时它才真的不再是当前数据目录）。
	view.PrevInUse = len(prev) > 0
	for _, dir := range prev {
		if !samePath(dir, a.paths.ConfigDir) && !samePath(dir, a.paths.LocalDir) {
			view.PrevInUse = false
		}
	}
	for _, dir := range prev {
		for _, item := range a.storageItems(dir) {
			if !item.Present {
				continue
			}
			view.PrevItems = append(view.PrevItems, item)
			view.PrevBytes += item.Bytes
		}
	}
	return view
}

// anyPresent 说清单里是否有任一项存在。
func anyPresent(items []StorageItem) bool {
	for _, item := range items {
		if item.Present {
			return true
		}
	}
	return false
}

// migrationFile 是一个待复制的文件。
type migrationFile struct {
	src  string
	dest string
	size int64
	// kind 区分三类需要 VACUUM INTO 的库与普通文件。
	kind string // database | files
	// validate 校验目标副本（仅数据库项）。
	validate func(string) error
	// copied 记录复制出来的字节数与源内容的 SHA256；校验按它比对，不重新
	// 读源文件——源文件在复制过程中可能被改写，重读会把「源变了」误报成
	// 「副本坏了」。
	copied copyRecord
}

// copyRecord 是一次复制的记录。
type copyRecord struct {
	bytes int64
	sha   string
}

// migrationPlan 是本次迁移要复制的全部文件与跳过的项。
type migrationPlan struct {
	files   []migrationFile
	skipped []StorageItem
	total   int64
}

// migrationSkipNote 说明某个文件是否不参与迁移。
func (a *App) migrationSkipNote(path string) (string, bool) {
	base := strings.ToLower(filepath.Base(path))
	index := a.cfg.IndexPath
	if index != "" && index != ":memory:" {
		if samePath(path, index) {
			return indexRebuildNote, true
		}
		if strings.HasPrefix(base, strings.ToLower(filepath.Base(index))+"-") {
			return indexRebuildNote, true // 索引的 -wal / -shm / -journal 同属可重建项
		}
	}
	// 三个数据库的日志与共享内存文件不单独复制：VACUUM INTO 的副本自带
	// 完整内容，把源库的 -wal 一起复制过去反而可能让副本读到不一致的页。
	for _, db := range []string{a.cfg.LibraryPath, a.cfg.MarksPath, a.cfg.ProjectsPath} {
		if db == "" || strings.EqualFold(path, db) {
			continue
		}
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if strings.EqualFold(path, db+suffix) {
				return "数据库日志包含在一致性副本中，不单独复制", true
			}
		}
	}
	switch base {
	case "session.lock":
		return "运行标记不迁移，重启时会重新写入", true
	case "app-config.json":
		return "应用配置文件固定在 %APPDATA%\\AgentDeck，不随数据移动", true
	}
	return "", false
}

// buildMigrationPlan 枚举源基准目录下的全部普通文件，给出目标路径。
// 白名单之外的文件一律复制；不复制目录结构以外的任何东西。
func (a *App) buildMigrationPlan(dest string) (*migrationPlan, error) {
	plan := &migrationPlan{}
	destRel := map[string]string{} // 相对路径（小写）→ 源文件，用于检出冲突
	bases := a.storageData()
	for _, base := range bases {
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if note, skip := a.migrationSkipNote(path); skip {
				item := storageItem(filepath.Base(path), path, note, strings.HasPrefix(note, "可重建"))
				plan.skipped = append(plan.skipped, item)
				return nil
			}
			rel, err := filepath.Rel(base, path)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("源文件在数据目录之外：%s", path)
			}
			key := strings.ToLower(rel)
			if other, ok := destRel[key]; ok && !samePath(other, path) {
				return fmt.Errorf("两个数据目录下都有 %s（%s 与 %s），无法合并到同一目标位置", rel, other, path)
			}
			destRel[key] = path
			info, err := entry.Info()
			if err != nil {
				return err
			}
			file := migrationFile{src: path, dest: filepath.Join(dest, rel), size: info.Size(), kind: "files"}
			if kind, validate, ok := a.databaseKind(base, rel); ok {
				file.kind, file.validate = kind, validate
			}
			plan.files = append(plan.files, file)
			plan.total += file.size
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(plan.files, func(i, j int) bool { return plan.files[i].dest < plan.files[j].dest })
	sort.Slice(plan.skipped, func(i, j int) bool { return plan.skipped[i].Path < plan.skipped[j].Path })
	return plan, nil
}

// databaseKind 判断相对路径是否是 AgentDeck 自己的一个库，是则返回
// VACUUM INTO 的类别与对应的结构校验函数。
func (a *App) databaseKind(base, rel string) (string, func(string) error, bool) {
	switch {
	case samePath(filepath.Join(base, rel), a.cfg.LibraryPath):
		return "database", func(path string) error {
			_, err := history.ValidateBackup(path)
			return err
		}, true
	case samePath(filepath.Join(base, rel), a.cfg.MarksPath):
		return "database", marks.Validate, true
	case samePath(filepath.Join(base, rel), a.cfg.ProjectsPath):
		return "database", projects.ValidateBackup, true
	}
	return "", nil, false
}

// destinationProblem 对补全后的数据根做无副作用的校验（不建目录、不写探针、
// 不看剩余空间），返回不能使用的一句话原因；可用时返回空串。
//
// 「确认前预览」与「执行迁移」共用这一份判断：界面在用户确认之前就能看到
// 目标位置为什么不能用，而不是点下「开始迁移」之后才被拒绝。
func (a *App) destinationProblem(dest string) string {
	if dest == "" {
		return "请选择数据存储位置"
	}
	if !filepath.IsAbs(dest) {
		return fmt.Sprintf("数据存储位置必须是绝对路径：%s", dest)
	}
	for _, base := range a.storageData() {
		if samePath(dest, base) {
			return "目标目录就是当前数据目录，无需迁移"
		}
		if within(dest, base) {
			return fmt.Sprintf("目标目录不能位于当前数据目录之内：%s", base)
		}
		if within(base, dest) {
			return fmt.Sprintf("目标目录不能包含当前数据目录：%s", base)
		}
	}
	if info, err := os.Stat(dest); err == nil {
		if !info.IsDir() {
			return fmt.Sprintf("目标位置已存在同名文件：%s", dest)
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			return fmt.Sprintf("目标目录无法读取：%v", err)
		}
		if len(entries) > 0 {
			return fmt.Sprintf("目标目录必须是空目录，以免覆盖已有文件：%s", dest)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Sprintf("目标目录无法访问：%v", err)
	}
	return ""
}

// checkDestination 在 destinationProblem 之上补上必须有副作用的两步——建目录
// 与写入探针——以及剩余空间预检。返回规范化后的目标路径。
func (a *App) checkDestination(dest string, required int64) (string, error) {
	dest = filepath.Clean(strings.TrimSpace(dest))
	if reason := a.destinationProblem(dest); reason != "" {
		return "", errors.New(reason)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("目标目录无法创建：%w", err)
	}
	// 可写性用一次真实写入确认，不能只看目录权限位。
	probe, err := os.CreateTemp(dest, "agentdeck-write-*.tmp")
	if err != nil {
		return "", fmt.Errorf("目标目录不可写：%w", err)
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	free, err := diskFree(dest)
	if err != nil {
		return "", fmt.Errorf("无法获取目标位置的剩余空间：%w", err)
	}
	if need := required * storageCopyRatio / storageCopyDiv; free < need {
		return "", fmt.Errorf("目标位置剩余空间不足：需要约 %s，当前可用 %s", humanBytes(need), humanBytes(free))
	}
	return dest, nil
}

// humanBytes 把字节数写成便于阅读的形式（迁移对话框与错误消息用）。
func humanBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}

// migrateStorage 执行迁移：快照复制 → 逐文件校验 → 写指针。
// 失败时只删本次写出的文件；源目录与配置都不动。
//
// picked 是用户在目录选择器里选定的目录，不是最终数据根：先按 storageLeaf
// 补全（见 storageDestination），之后的一切都以补全后的路径为准。
func (a *App) migrateStorage(picked string) (MigrateReport, error) {
	if !a.beginMigration() {
		return MigrateReport{}, errors.New("已有迁移正在进行")
	}
	defer a.endMigration()
	dest := storageDestination(picked)
	plan, err := a.buildMigrationPlan(dest)
	if err != nil {
		return MigrateReport{}, err
	}
	dest, err = a.checkDestination(dest, plan.total)
	if err != nil {
		return MigrateReport{}, err
	}
	progress := newMigrationProgress(a, plan.total, len(plan.files))
	// 心跳与迁移同生共死：迁移返回前停掉 ticker，之后不再有进度事件。
	defer progress.close()
	go progress.run()
	written := make([]string, 0, len(plan.files))
	fail := func(err error) (MigrateReport, error) {
		progress.stage("failed", "迁移未完成，正在清理本次写入的文件")
		removed := removeFiles(written)
		progress.stage("failed", fmt.Sprintf("迁移未完成，已清理本次写入的 %d 个文件", removed))
		return MigrateReport{}, fmt.Errorf("%w（已清理本次写入的 %d 个文件，原数据与配置未改动）", err, removed)
	}
	report := MigrateReport{Dest: dest, Files: len(plan.files)}
	for _, item := range plan.skipped {
		report.Skipped = append(report.Skipped, fmt.Sprintf("%s（%s）", item.Name, item.Note))
	}
	progress.stage("copy", "正在生成数据副本")
	copier := a.copyFileFn
	if copier == nil {
		copier = copyFile
	}
	for i := range plan.files {
		file := plan.files[i]
		// 先报「正在动哪个文件」再干活：VACUUM INTO 与整库校验都是不可分割的
		// 长步骤，事后再报等于最耗时的几分钟里界面一言不发。
		progress.copying(file)
		if err := os.MkdirAll(filepath.Dir(file.dest), 0o755); err != nil {
			return fail(err)
		}
		if file.kind == "database" {
			if err := a.snapshotDatabase(file); err != nil {
				return fail(err)
			}
		} else {
			record, err := copier(file.src, file.dest)
			if err != nil {
				return fail(err)
			}
			if record.bytes != file.size {
				return fail(fmt.Errorf("复制 %s 时长度不符：%d ≠ %d（源文件可能正在被改写）", filepath.Base(file.src), record.bytes, file.size))
			}
			plan.files[i].copied = record
		}
		written = append(written, file.dest)
		progress.copied(file)
		report.Bytes += file.size
	}
	progress.stage("verify", "正在校验副本")
	for _, file := range plan.files {
		progress.verifying(file)
		if err := verifyCopy(file); err != nil {
			return fail(err)
		}
	}
	progress.stage("switch", "正在切换数据位置")
	if err := a.switchDataRoot(dest); err != nil {
		return fail(err)
	}
	progress.done()
	report.Message = fmt.Sprintf("数据已复制到 %s。重启 AgentDeck 后生效；旧位置的数据仍然保留，可在重启后确认无误再清理。", dest)
	return report, nil
}

// snapshotDatabase 用 VACUUM INTO 生成一致副本，再按库的结构校验一次。
func (a *App) snapshotDatabase(file migrationFile) error {
	var err error
	switch {
	case samePath(file.src, a.cfg.LibraryPath):
		err = a.history.BackupTo(file.dest)
	case samePath(file.src, a.cfg.MarksPath):
		err = a.marks.BackupTo(file.dest)
	case samePath(file.src, a.cfg.ProjectsPath):
		err = a.declared.BackupTo(file.dest)
	default:
		return fmt.Errorf("未知数据库：%s", file.src)
	}
	if err != nil {
		return fmt.Errorf("生成 %s 的数据副本失败：%w", filepath.Base(file.src), err)
	}
	if file.validate != nil {
		if err = file.validate(file.dest); err != nil {
			return fmt.Errorf("数据副本校验失败（%s）：%w", filepath.Base(file.src), err)
		}
	}
	return nil
}

// verifyCopy 校验一个已复制文件：大小与 SHA256 都要与复制时的记录一致。
// 数据库副本由 VACUUM INTO 重新写出、字节不同，只校验结构与可读性。
func verifyCopy(file migrationFile) error {
	destInfo, err := os.Stat(file.dest)
	if err != nil {
		return fmt.Errorf("副本不存在：%s：%w", file.dest, err)
	}
	if file.kind == "database" {
		if destInfo.Size() == 0 {
			return fmt.Errorf("副本是空文件：%s", file.dest)
		}
		if file.validate != nil {
			if err = file.validate(file.dest); err != nil {
				return fmt.Errorf("副本结构校验失败（%s）：%w", filepath.Base(file.dest), err)
			}
		}
		return nil
	}
	if destInfo.Size() != file.copied.bytes {
		return fmt.Errorf("副本大小不一致（%s）：%d ≠ %d", filepath.Base(file.dest), destInfo.Size(), file.copied.bytes)
	}
	copied, err := hashFile(file.dest)
	if err != nil {
		return err
	}
	if copied != file.copied.sha {
		return fmt.Errorf("副本内容校验失败（%s）", filepath.Base(file.dest))
	}
	return nil
}

// copyFile 复制一个普通文件，目标已存在时报错。返回实际复制的字节数与
// 源内容的 SHA256（边读边算，不重读源文件）。
func copyFile(src, dest string) (copyRecord, error) {
	if _, err := os.Stat(dest); err == nil {
		return copyRecord{}, fmt.Errorf("目标已存在，不覆盖：%s", dest)
	}
	in, err := os.Open(src)
	if err != nil {
		return copyRecord{}, err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return copyRecord{}, err
	}
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(out, digest), in)
	if err != nil {
		out.Close()
		return copyRecord{}, err
	}
	if err = out.Sync(); err != nil {
		out.Close()
		return copyRecord{}, err
	}
	if err = out.Close(); err != nil {
		return copyRecord{}, err
	}
	return copyRecord{bytes: written, sha: hex.EncodeToString(digest.Sum(nil))}, nil
}

// hashFile 计算文件的 SHA256。
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// removeFiles 删除本次迁移写出的文件，返回实际删掉的个数；只删清单内的
// 文件，绝不删除目录（空目录留在原地，用户看得见迁移过程）。
func removeFiles(paths []string) int {
	removed := 0
	for _, path := range paths {
		if err := os.Remove(path); err == nil {
			removed++
		}
	}
	return removed
}

// switchDataRoot 写指针：把数据根改为 dest，记下旧位置供清理。
// 只有全部副本校验通过后才会走到这里。
func (a *App) switchDataRoot(dest string) error {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settingsErr != nil {
		return fmt.Errorf("应用配置文件无法读取，未切换数据位置：%v", a.settingsErr)
	}
	cfg := a.settings
	previous := append([]string(nil), cfg.PreviousDirs...)
	for _, dir := range a.storageData() {
		if !containsPath(previous, dir) {
			previous = append(previous, dir)
		}
	}
	cfg.DataRoot = dest
	cfg.PreviousDirs = prunePreviousDirs(previous, cfg)
	if err := appconfig.Save(cfg); err != nil {
		return fmt.Errorf("写入应用配置失败：%w", err)
	}
	a.settings = cfg
	return nil
}

// prunePreviousDirs 去掉与 cfg 解析出的当前数据目录相同的旧位置条目。
//
// 「旧位置」按定义是当前不再使用的位置：把数据位置改回默认位置之后，原先
// 记下的默认位置又成了当前目录，留着它会让「清理旧位置数据」整批被拒
// （见 cleanup.go 的保护），用户反而删不掉真正该删的那一份。
func prunePreviousDirs(dirs []string, cfg appconfig.Config) []string {
	paths, err := appconfig.Resolve(cfg)
	if err != nil {
		return dirs
	}
	kept := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if samePath(dir, paths.ConfigDir) || samePath(dir, paths.LocalDir) {
			continue
		}
		kept = append(kept, dir)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// containsPath 说 dirs 里是否已有 path（不区分大小写）。
func containsPath(dirs []string, path string) bool {
	for _, dir := range dirs {
		if samePath(dir, path) {
			return true
		}
	}
	return false
}

// migrationProgress 推送迁移进度，并为不可分割的长步骤保活。
//
// 三个库用 VACUUM INTO 生成一致副本，那是一条 SQL 语句：2.8 GB 的会话库要跑
// 几分钟，期间没有任何回调可挂；整库 integrity_check 同理。只靠「复制完一个
// 文件报一次」的写法，最耗时的这几分钟里界面一直停在 0%——那正是用户看到的
// 「没有进度」。
//
// 所以进度由两个来源合成：每个文件开始前立即报一次（说明正在动哪个文件），
// 之后每秒由 ticker 刷新一次。Done 取「已完成文件的字节数 + 正在写入的目标
// 文件现有大小」：VACUUM INTO 直接往目标文件写，普通复制也是，所以这个数
// 一直是真实的字节进度；文件大小暂时没变时，事件本身也说明还在推进，界面
// 据此显示已用时间。
type migrationProgress struct {
	a      *App
	total  int64
	files  int
	period time.Duration
	start  time.Time

	mu sync.Mutex
	// 字段名刻意与 stage()/done() 这些方法错开：Go 不允许同名字段与方法。
	curStage   string
	curMessage string
	doneBytes  int64  // 已完成文件的字节数
	doneFiles  int    // 已完成的文件数
	curPath    string // 正在写入的目标文件；空表示当前不在写文件
	curSize    int64  // 当前文件的源大小，用于把估算上界卡在真实值上
	stopped    bool

	stop     chan struct{}
	stopOnce sync.Once
}

// migrationEmitPeriod 是进度事件的间隔。它同时是长步骤的心跳：1 秒一次既能
// 让百分比连续走动，也不会把事件通道灌满。
const migrationEmitPeriod = time.Second

func newMigrationProgress(a *App, total int64, files int) *migrationProgress {
	return &migrationProgress{
		a: a, total: total, files: files, period: migrationEmitPeriod,
		start: time.Now(), stop: make(chan struct{}),
	}
}

// run 起 ticker 循环，直到 close 被调用。调用方用 `go progress.run()`。
func (p *migrationProgress) run() {
	period := p.period
	if period <= 0 {
		period = migrationEmitPeriod
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.beat()
		}
	}
}

func (p *migrationProgress) close() {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopped = true
		p.mu.Unlock()
		close(p.stop)
	})
}

// beat 是心跳：按当前文件大小重算已复制字节数再推一次。
func (p *migrationProgress) beat() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	stage, message := p.curStage, p.curMessage
	current, srcSize, done, doneFiles := p.curPath, p.curSize, p.doneBytes, p.doneFiles
	p.mu.Unlock()

	if current != "" && srcSize > 0 {
		done += writtenBytes(current, srcSize)
	}
	p.push(StorageProgress{
		Stage: stage, Done: done, Total: p.total, Files: p.files,
		DoneFiles: doneFiles, Message: message,
	})
}

// stage 切换阶段并立即推送。Done 带上当前已完成字节数：阶段切换不该让界面
// 上的百分比掉回 0（旧写法漏了这一项，校验与切换阶段会各跳回 0% 一次）。
func (p *migrationProgress) stage(stage, message string) {
	p.report(stage, message, "", 0)
}

// copying 在开始处理一个文件**之前**推送：一行字说明正在动哪个文件，长步骤
// 期间用户看到的就是这一行。current/srcSize 让心跳按目标文件大小估出真实
// 字节进度——VACUUM INTO 直接往目标文件写，普通复制也是。
func (p *migrationProgress) copying(file migrationFile) {
	message := "正在复制 " + filepath.Base(file.dest)
	if file.kind == "database" {
		message = "正在生成 " + filepath.Base(file.dest) + " 的一致副本"
	}
	p.report("copy", message, file.dest, file.size)
}

// verifying 逐个文件报校验。整库校验要跑 PRAGMA integrity_check 再逐行重算
// 哈希，是另一段不可分割的长步骤；只报一句「正在校验副本」等于什么都没说。
// 校验读的是目标文件，大小早已定型，所以不设 current——这一段只能靠已用时
// 间说明还在推进。
func (p *migrationProgress) verifying(file migrationFile) {
	p.report("verify", "正在校验 "+filepath.Base(file.dest), "", 0)
}

// report 是 copying / verifying 的公共推送口：设定阶段、说明与「正在写的目标
// 文件」，随即推一次。Done 原样带上已完成的字节数，阶段切换不会让百分比掉回 0。
func (p *migrationProgress) report(stage, message, current string, srcSize int64) {
	p.mu.Lock()
	p.curStage, p.curMessage = stage, message
	p.curPath, p.curSize = current, srcSize
	done, doneFiles := p.doneBytes, p.doneFiles
	p.mu.Unlock()
	p.push(StorageProgress{
		Stage: stage, Done: done, Total: p.total, Files: p.files,
		DoneFiles: doneFiles, Message: message,
	})
}

// copied 记下一个文件已完成，下次心跳按它累加。
func (p *migrationProgress) copied(file migrationFile) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.doneBytes += file.size
	p.doneFiles++
	p.curPath, p.curSize = "", 0
}

// done 推送最终状态：全部文件已复制并校验通过。
func (p *migrationProgress) done() {
	p.mu.Lock()
	p.curStage, p.curMessage = "done", ""
	p.curPath, p.curSize = "", 0
	done, doneFiles := p.doneBytes, p.doneFiles
	p.mu.Unlock()
	p.push(StorageProgress{
		Stage: "done", Done: done, Total: p.total, Files: p.files,
		DoneFiles: doneFiles,
	})
}

func (p *migrationProgress) push(st StorageProgress) {
	// Elapsed 在推送的最后一步补上：所有推送点共用同一条计时口径。
	if st.Elapsed == 0 {
		st.Elapsed = int64(time.Since(p.start) / time.Second)
	}
	p.a.emit(EventStorageProgress, st)
}

// writtenBytes 是目标文件已写入的字节数，上限取源大小：VACUUM INTO 的产物
// 可能比源库略小或略大，超出源大小时按 100% 算，不让百分比冲过 100。
func writtenBytes(path string, srcSize int64) int64 {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= 0 {
		return 0
	}
	return min(info.Size(), srcSize)
}

// beginMigration 取得迁移独占权；已有迁移在进行时返回 false。
func (a *App) beginMigration() bool {
	a.migrateMu.Lock()
	defer a.migrateMu.Unlock()
	if a.migrating {
		return false
	}
	a.migrating = true
	return true
}

func (a *App) endMigration() {
	a.migrateMu.Lock()
	a.migrating = false
	a.migrateMu.Unlock()
}
