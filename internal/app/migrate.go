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
	"time"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
	"github.com/enpiaohj/agentdeck/internal/history"
	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/projects"
)

// 数据存储位置：查看占用、迁移到其他目录、清理旧位置。
//
// 迁移的不变量（改动本文件必须保持）：
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

// checkDestination 校验迁移目标：不满足任一条即拒绝，且不动任何文件。
func (a *App) checkDestination(dest string, required int64) (string, error) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return "", errors.New("请选择数据存储位置")
	}
	if !filepath.IsAbs(filepath.Clean(dest)) {
		return "", fmt.Errorf("数据存储位置必须是绝对路径：%s", dest)
	}
	dest = filepath.Clean(dest)
	for _, base := range a.storageData() {
		if samePath(dest, base) {
			return "", errors.New("目标目录就是当前数据目录，无需迁移")
		}
		if within(dest, base) {
			return "", fmt.Errorf("目标目录不能位于当前数据目录之内：%s", base)
		}
		if within(base, dest) {
			return "", fmt.Errorf("目标目录不能包含当前数据目录：%s", base)
		}
	}
	if info, err := os.Stat(dest); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("目标位置已存在同名文件：%s", dest)
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			return "", fmt.Errorf("目标目录无法读取：%w", err)
		}
		if len(entries) > 0 {
			return "", fmt.Errorf("目标目录必须是空目录，以免覆盖已有文件：%s", dest)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("目标目录无法访问：%w", err)
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
func (a *App) migrateStorage(dest string) (MigrateReport, error) {
	if !a.beginMigration() {
		return MigrateReport{}, errors.New("已有迁移正在进行")
	}
	defer a.endMigration()
	plan, err := a.buildMigrationPlan(dest)
	if err != nil {
		return MigrateReport{}, err
	}
	dest, err = a.checkDestination(dest, plan.total)
	if err != nil {
		return MigrateReport{}, err
	}
	progress := &migrationProgress{a: a, total: plan.total, files: len(plan.files)}
	written := make([]string, 0, len(plan.files))
	fail := func(err error) (MigrateReport, error) {
		progress.stage("failed", "迁移未完成，正在清理本次写入的文件")
		removed := removeFiles(written)
		progress.done()
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
		if err := verifyCopy(file); err != nil {
			return fail(err)
		}
		progress.verified(file)
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

// migrationProgress 按时间节流地推送迁移进度。
type migrationProgress struct {
	a          *App
	total      int64
	files      int
	bytes      int64
	doneFiles  int
	lastEmit   time.Time
	emitPeriod time.Duration
}

// migrationEmitPeriod 是进度事件的最小间隔：4.58 GB 有上千个文件，逐个
// 推送会把事件通道灌满。
const migrationEmitPeriod = 100 * time.Millisecond

func (p *migrationProgress) stage(stage, message string) {
	p.a.emit(EventStorageProgress, StorageProgress{Stage: stage, Total: p.total, Files: p.files, Message: message})
	p.lastEmit = time.Now()
}

func (p *migrationProgress) copied(file migrationFile) {
	p.bytes += file.size
	p.doneFiles++
	p.maybeEmit("copy", filepath.Base(file.dest))
}

func (p *migrationProgress) verified(file migrationFile) {
	p.maybeEmit("verify", filepath.Base(file.dest))
}

func (p *migrationProgress) done() {
	p.a.emit(EventStorageProgress, StorageProgress{Stage: "done", Done: p.bytes, Total: p.total, Files: p.files, DoneFiles: p.doneFiles})
}

func (p *migrationProgress) maybeEmit(stage, message string) {
	period := p.emitPeriod
	if period == 0 {
		period = migrationEmitPeriod
	}
	if time.Since(p.lastEmit) < period {
		return
	}
	p.lastEmit = time.Now()
	p.a.emit(EventStorageProgress, StorageProgress{Stage: stage, Done: p.bytes, Total: p.total, Files: p.files, DoneFiles: p.doneFiles, Message: message})
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
