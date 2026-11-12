package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// 界面偏好（主题、栏宽、删除前备份等）存在 WebView2 的 localStorage 里。Wails 默认把
// WebView2 数据目录放在 %APPDATA%\<exe 文件名>\，而发布产物的文件名带版本号
// （AgentDeck-vX.Y.Z-win-x64.exe），于是每次升级都换一个新目录，偏好悄悄回到默认值，
// 旧目录（每个约 70 MB）也越积越多。
//
// 现在固定使用 %LOCALAPPDATA%\AgentDeckWebView：
//   - 放在数据根之外：数据位置迁移与「清理旧位置」都按数据根枚举文件，浏览器缓存与
//     正被占用的文件不应进入那条路径。
//   - 首次使用时，从最近一次使用过的旧版本目录复制 localStorage（只读旧目录，先复制到
//     临时目录再改名，失败不留半份）。旧目录原样保留，由用户自行删除。

const webviewDirName = "AgentDeckWebView"

// localStorage 在 WebView2 数据目录下的相对位置（LevelDB）。
var localStorageRel = filepath.Join("EBWebView", "Default", "Local Storage", "leveldb")

// 旧版本按 exe 文件名建的目录：发布产物与开发构建的几种名字。
var legacyProfileName = regexp.MustCompile(`(?i)^(agentdeck-v\d+\.\d+\.\d+-win-x64|agentdeck-desktop)\.exe$`)

// webviewDataDir 返回固定的 WebView2 数据目录；取不到 LOCALAPPDATA 时返回空串，
// 交给 Wails 使用默认位置。
func webviewDataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, webviewDirName)
}

// migrateLegacyLocalStorage 在新目录还没有 localStorage 时，从 roaming 下最近使用过的
// 旧版本目录复制一份。返回复制来源（空串表示没有可迁移的或无需迁移）。
// 调用方必须已取得单实例所有权：此时不会有旧版本正在写它的 localStorage。
func migrateLegacyLocalStorage(roaming, dataDir string) (string, error) {
	if roaming == "" || dataDir == "" {
		return "", nil
	}
	target := filepath.Join(dataDir, localStorageRel)
	if _, err := os.Stat(target); err == nil {
		return "", nil // 已经有偏好：不覆盖
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	source := newestLegacyLocalStorage(roaming)
	if source == "" {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".leveldb-migrate-")
	if err != nil {
		return "", err
	}
	if err = copyFlatDir(source, tmp); err != nil {
		removeFlatDir(tmp)
		return "", fmt.Errorf("复制旧版本界面偏好失败：%w", err)
	}
	if err = os.Rename(tmp, target); err != nil {
		removeFlatDir(tmp)
		return "", err
	}
	return source, nil
}

// newestLegacyLocalStorage 在 roaming 下找最近修改过的旧版本 localStorage 目录。
func newestLegacyLocalStorage(roaming string) string {
	entries, err := os.ReadDir(roaming)
	if err != nil {
		return ""
	}
	var best string
	var bestTime time.Time
	for _, entry := range entries {
		if !entry.IsDir() || !legacyProfileName.MatchString(entry.Name()) {
			continue
		}
		dir := filepath.Join(roaming, entry.Name(), localStorageRel)
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		if modified := newestModTime(dir, info.ModTime()); best == "" || modified.After(bestTime) {
			best, bestTime = dir, modified
		}
	}
	return best
}

// newestModTime 取目录内文件的最新修改时间：LevelDB 写的是日志文件，目录本身的
// 时间不一定随之变化。
func newestModTime(dir string, fallback time.Time) time.Time {
	newest := fallback
	entries, err := os.ReadDir(dir)
	if err != nil {
		return newest
	}
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

// copyFlatDir 复制一层普通文件（LevelDB 目录没有子目录）；遇到链接或子目录拒绝，
// 不跟随也不猜测。
func copyFlatDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(src, entry.Name()))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("不是普通文件：%s", entry.Name())
		}
		if err := copyFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// removeFlatDir 只删本次复制写进临时目录的文件，再非递归地删目录本身。
func removeFlatDir(dir string) {
	if !strings.Contains(filepath.Base(dir), ".leveldb-migrate-") {
		return
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
	_ = os.Remove(dir)
}
