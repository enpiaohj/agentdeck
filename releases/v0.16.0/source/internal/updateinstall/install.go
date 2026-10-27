// Package updateinstall 把校验通过的更新包在进程退出时落地为新的可执行文件。
//
// 边界：只处理本进程自己的 exe 与它所在目录；不下载、不校验（校验由
// internal/update 完成），也绝不启动更新包里的任何程序。落地失败时原文件
// 保持可用，并留下标记供下次启动提示。
//
// 一切都在进程退出的最后一步进行：正在运行的 exe 不能被覆盖，但可以被改名。
// 因此登记（Prepare）只做可写性检查，真正落地由 Finish 在窗口关闭、数据库
// 关闭、单实例互斥锁释放之后执行。
package updateinstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// BackupSuffix 是原地替换时旧文件的临时名。
	//
	// 刻意不用 ".old"：那是用户自己也会用来做备份的名字，更新流程一旦把它
	// 当成自己的回滚副本删掉，就等于删了用户的文件。带上产品名后，同目录里
	// 出现它只可能是本程序的更新流程留下的。
	BackupSuffix = ".agentdeck-backup"
	// StagingSuffix 是写入中的新文件：只有完整写完并核对大小后才改名到位，
	// 因此目录里不会出现「名字正确但内容不完整」的 exe。
	StagingSuffix = ".new"
	// ReplacesSuffix 是落在新文件旁的标记，内容是被它取代的旧文件名；
	// 新版本启动后据此删除旧文件。
	ReplacesSuffix = ".replaces"
	// FailedSuffix 是落地失败标记，放在仍在使用的原文件旁。
	FailedSuffix = ".update-failed"
)

// ReleaseArtifact 匹配发布产物的命名（AgentDeck-v1.2.3-win-x64.exe）。
// 只有当前文件与更新包都符合这个命名时才让文件名跟随版本。
var ReleaseArtifact = regexp.MustCompile(`(?i)^AgentDeck-v\d+\.\d+\.\d+-win-x64\.exe$`)

// ErrNotWritable 表示程序所在目录不可写，更新无法落地。
var ErrNotWritable = errors.New("程序所在目录没有写入权限")

// ResolveTarget 计算新版本应该落到的路径。
//
// 当前文件名是发布产物命名时，新版本用更新包自己的产物名写到同一目录，
// 文件名与版本始终一致；文件名被用户改过（如 AgentDeck.exe）时保持原路径
// 原地替换，免得替他改掉自己起的名字。
func ResolveTarget(current, packagePath string) string {
	currentName, packageName := filepath.Base(current), filepath.Base(packagePath)
	if !ReleaseArtifact.MatchString(currentName) || !ReleaseArtifact.MatchString(packageName) {
		return current
	}
	return filepath.Join(filepath.Dir(current), packageName)
}

// Probe 确认程序所在目录可写。在登记时就检查，因为退出时已经没有界面可以报错。
func Probe(current string) error {
	dir := filepath.Dir(current)
	file, err := os.CreateTemp(dir, "agentdeck-update-probe-*")
	if err != nil {
		return fmt.Errorf("%w：%s", ErrNotWritable, dir)
	}
	name := file.Name()
	file.Close()
	if err = os.Remove(name); err != nil {
		return fmt.Errorf("%w：%s", ErrNotWritable, dir)
	}
	return nil
}

// Install 把更新包落地到 newPath，返回是否成功。
//
// 成功时 package 会被删除；失败时 current 保持可用。newPath 与 current 相同
// 表示原地替换，不同表示文件名跟随版本（旧文件原样留着，由新版本删除）。
func Install(current, newPath, packagePath string) bool {
	// 更新包已经就在目标路径上（同目录同名）：无需搬运，更不能在最后把它删掉。
	if strings.EqualFold(packagePath, newPath) {
		info, err := os.Stat(newPath)
		return err == nil && info.Size() > 0
	}
	sideBySide := !strings.EqualFold(newPath, current)

	staging := newPath + StagingSuffix
	if err := copyVerified(packagePath, staging); err != nil {
		return false
	}

	if sideBySide {
		if err := os.Rename(staging, newPath); err != nil {
			remove(staging)
			return false
		}
		// 标记写不成不影响本次落地，但旧文件会因此永远留着；记在失败标记里，
		// 让下次启动至少提示一次（清理逻辑没有标记就不敢删，见 Cleanup）。
		if err := os.WriteFile(newPath+ReplacesSuffix, []byte(filepath.Base(current)), 0o600); err != nil {
			MarkFailed(newPath)
		}
		remove(packagePath)
		return true
	}

	backup := current + BackupSuffix
	remove(backup)
	if err := os.Rename(current, backup); err != nil {
		remove(staging)
		return false
	}
	if err := os.Rename(staging, current); err != nil {
		_ = os.Rename(backup, current)
		remove(staging)
		return false
	}
	remove(packagePath)
	return true
}

// copyVerified 复制 package 到 staging 并核对大小，避免留下残缺的 exe。
func copyVerified(packagePath, staging string) error {
	remove(staging)
	src, err := os.Open(packagePath)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(staging, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	written, copyErr := dst.ReadFrom(src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		remove(staging)
		return fmt.Errorf("写入新版本失败")
	}
	if written != info.Size() {
		remove(staging)
		return fmt.Errorf("新版本大小与更新包不一致")
	}
	return nil
}

// UsablePath 是落地失败后还能用来启动的文件：正常是 current；原地替换连
// 回滚都失败的极端情况下，原文件以 .old 留着。
func UsablePath(current string) string {
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if _, err := os.Stat(current + BackupSuffix); err == nil {
		return current + BackupSuffix
	}
	return current
}

// MarkFailed 在仍在使用的原文件旁留下失败标记，供下次启动提示一次。
func MarkFailed(current string) {
	_ = os.WriteFile(current+FailedSuffix, []byte("1"), 0o600)
}

// ConsumeFailureMarker 报告上次退出时更新是否落地失败，并清除标记（只提示一次）。
func ConsumeFailureMarker(current string) bool {
	marker := current + FailedSuffix
	if _, err := os.Stat(marker); err != nil {
		return false
	}
	remove(marker)
	return true
}

// Cleanup 在启动时善后上一次更新：清掉遗留的临时文件与回滚副本，文件名变过的
// 更新则确认启动项与快捷方式已改指向后删除旧文件。
//
// retarget 是「把指向旧路径的启动项与快捷方式改指向新路径」，返回是否确认改完；
// 可为 nil，表示调用方没有改指向的能力——那时**不删旧文件**（留着旧文件只是浪费
// 空间，删掉一个还有东西指着的文件会留下点不开的死链接）。它在这里再执行一次，
// 是因为退出时那一步可能没成功。
func Cleanup(current string, retarget func(oldPath, newPath string) bool) {
	remove(current + BackupSuffix)
	remove(current + StagingSuffix)

	marker := current + ReplacesSuffix
	raw, err := os.ReadFile(marker)
	if err != nil {
		// 没有标记时也可能残留 side-by-side 落地留下的 <新版本名>.new
		// （在写 staging 之后、改名之前崩溃）。按发布产物命名清一遍。
		removeStaleStaging(current)
		return
	}
	removeStaleStaging(current)
	oldName := strings.TrimSpace(string(raw))
	if !safePreviousName(oldName, filepath.Base(current)) {
		// 标记内容不可信：什么都不动，只清掉标记。
		remove(marker)
		return
	}
	oldPath := filepath.Join(filepath.Dir(current), oldName)
	// 改指向没被确认、或还有快捷方式指着旧路径，就先不删：保留标记，下次启动再试。
	if retarget == nil || !retarget(oldPath, current) || ShortcutsPointingTo(oldPath) {
		return
	}
	if err = os.Remove(oldPath); err != nil {
		// 旧进程可能还在退出途中：保留标记，下次启动再试。
		return
	}
	remove(marker)
}

// removeStaleStaging 清掉同目录下「发布产物名 + .new」的残留。
//
// side-by-side 落地时 staging 用的是目标版本名（AgentDeck-v0.15.0-win-x64.exe.new），
// 与 current 不同名，Cleanup 开头那两行清不到它。只认发布产物命名，且只删
// .new 这一种后缀，不碰用户放在同目录的其他文件。
func removeStaleStaging(current string) {
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, StagingSuffix) {
			continue
		}
		if !ReleaseArtifact.MatchString(strings.TrimSuffix(name, StagingSuffix)) {
			continue
		}
		remove(filepath.Join(dir, name))
	}
}

// safePreviousName 只接受「同目录下的发布产物名、且不是自己」。
func safePreviousName(name, currentName string) bool {
	return name != "" &&
		name == filepath.Base(name) &&
		!strings.ContainsAny(name, `/\:`) &&
		ReleaseArtifact.MatchString(name) &&
		!strings.EqualFold(name, currentName)
}

func remove(path string) {
	_ = os.Remove(path)
}
