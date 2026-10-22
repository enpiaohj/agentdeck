package app

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/enpiaohj/agentdeck/internal/claudehistory"
	"github.com/enpiaohj/agentdeck/internal/history"
	"github.com/enpiaohj/agentdeck/internal/marks"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const backupLimit int64 = 4 << 30
const backupFileLimit = 20000

type BackupInfo struct {
	CreatedAt   string `json:"created_at"`
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
	Sessions    int    `json:"sessions"`
	Versions    int    `json:"versions"`
	Preferences string `json:"preferences"`
}
type RestoreReport struct {
	RollbackPath string `json:"rollback_path"`
	Preferences  string `json:"preferences"`
	Message      string `json:"message"`
}
type backupEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type backupManifest struct {
	FormatVersion int           `json:"formatVersion"`
	CreatedAt     string        `json:"created_at"`
	Scope         string        `json:"scope"`
	Files         []backupEntry `json:"files"`
}

func validatePreferences(value string) error {
	if len(value) > 64<<10 {
		return fmt.Errorf("界面偏好超过 64 KiB")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &obj); err != nil || obj == nil {
		return fmt.Errorf("界面偏好必须是 JSON 对象")
	}
	return nil
}
func safeBackupName(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
			return false
		}
	}
	return name != "" && !strings.ContainsAny(name, "\\:\x00") && !strings.HasPrefix(name, "/") && path.Clean(name) == name && name != ".." && !strings.HasPrefix(name, "../")
}
func digestBackup(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func hashBackupFile(path string) (string, error) {
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
func copyBackupFile(dest io.Writer, source string, limit int64) (int64, string, error) {
	f, err := os.Open(source)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dest, h), io.LimitReader(f, limit+1))
	if err == nil && n > limit {
		err = fmt.Errorf("备份总大小超过 4 GiB")
	}
	return n, hex.EncodeToString(h.Sum(nil)), err
}

// Resolve the existing ancestor, so directory aliases cannot place backups in imports.
func resolvedBackupDir(value string) (string, error) {
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	suffix := []string{}
	cur := abs
	for {
		resolved, e := filepath.EvalSymlinks(cur)
		if e == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", e
		}
		suffix = append(suffix, filepath.Base(cur))
		cur = parent
	}
}

func (s *SessionService) BackupData(destDir, preferences string) (string, error) {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	s.a.captureMu.Lock()
	defer s.a.captureMu.Unlock()
	return s.backupData(destDir, preferences)
}
func (s *SessionService) backupData(destDir, preferences string) (result string, err error) {
	if err = validatePreferences(preferences); err != nil {
		return "", err
	}
	if !filepath.IsAbs(destDir) {
		return "", fmt.Errorf("备份目录必须是绝对路径")
	}
	target, e := resolvedBackupDir(destDir)
	if e != nil {
		return "", e
	}
	imports, e := resolvedBackupDir(s.a.cfg.ImportDir)
	if e != nil {
		return "", e
	}
	if strings.EqualFold(filepath.VolumeName(imports), filepath.VolumeName(target)) {
		rel, e := filepath.Rel(strings.ToLower(imports), strings.ToLower(target))
		if e != nil {
			return "", e
		}
		if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("备份目录不能位于 Claude 导入库内")
		}
	}
	if err = os.MkdirAll(destDir, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp("", "agentdeck-backup-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err = s.a.history.BackupTo(filepath.Join(stage, "history.db")); err != nil {
		return "", err
	}
	if err = s.a.marks.BackupTo(filepath.Join(stage, "marks.db")); err != nil {
		return "", err
	}
	content := map[string]string{"history.db": filepath.Join(stage, "history.db"), "marks.db": filepath.Join(stage, "marks.db")}
	if err = os.WriteFile(filepath.Join(stage, "preferences.json"), []byte(preferences), 0600); err != nil {
		return "", err
	}
	content["preferences.json"] = filepath.Join(stage, "preferences.json")
	err = filepath.WalkDir(s.a.cfg.ImportDir, func(p string, d os.DirEntry, e error) error {
		if os.IsNotExist(e) && p == s.a.cfg.ImportDir {
			return nil
		}
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("导入库包含符号链接： %s", p)
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("非普通导入文件")
		}
		rel, e := filepath.Rel(s.a.cfg.ImportDir, p)
		if e != nil {
			return e
		}
		content["imports/"+filepath.ToSlash(rel)] = p
		return nil
	})
	if err != nil {
		return "", err
	}
	for name := range content {
		if !safeBackupName(name) {
			return "", fmt.Errorf("导入库包含不安全的文件名：%s", name)
		}
	}
	out, err := os.CreateTemp(destDir, "AgentDeck-data-"+time.Now().Format("20060102-150405")+"-*.zip")
	if err != nil {
		return "", err
	}
	result = out.Name()
	outputPath := result
	defer func() {
		out.Close()
		if err != nil {
			os.Remove(outputPath)
		}
	}()
	zw := zip.NewWriter(out)
	manifest := backupManifest{FormatVersion: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Scope: "AgentDeck 自有永久历史（含全部版本）、标注、Claude 导入库、偏好；不含原生 Agent 目录、凭据及可重建全文索引。"}
	var total int64
	for name, p := range content {
		if len(manifest.Files) >= backupFileLimit {
			return "", fmt.Errorf("备份文件数量超限")
		}
		w, e := zw.Create(name)
		if e != nil {
			return "", e
		}
		n, digest, e := copyBackupFile(w, p, backupLimit-total)
		if e != nil {
			return "", e
		}
		total += n
		manifest.Files = append(manifest.Files, backupEntry{name, n, digest})
	}
	data, _ := json.Marshal(manifest)
	if len(data) > 4<<20 || total+int64(len(data)) > backupLimit {
		return "", fmt.Errorf("备份清单或总大小超限")
	}
	w, err := zw.Create("manifest.json")
	if err != nil {
		return "", err
	}
	if _, err = w.Write(data); err != nil {
		return "", err
	}
	if err = zw.Close(); err != nil {
		return "", err
	}
	if err = out.Sync(); err != nil {
		return "", err
	}
	err = out.Close()
	return result, err
}
func inspectData(src string) (info BackupInfo, stage string, err error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return info, "", err
	}
	defer zr.Close()
	if len(zr.File) > backupFileLimit+1 {
		return info, "", fmt.Errorf("备份条目过多")
	}
	stage, err = os.MkdirTemp("", "agentdeck-restore-")
	if err != nil {
		return info, "", err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(stage)
		}
	}()
	entries := map[string]backupEntry{}
	foldedNames := map[string]bool{}
	var manifest backupManifest
	var total int64
	for _, f := range zr.File {
		if !safeBackupName(f.Name) || f.Mode()&os.ModeSymlink != 0 || !f.Mode().IsRegular() {
			return info, stage, fmt.Errorf("不安全备份路径： %s", f.Name)
		}
		if foldedNames[strings.ToLower(f.Name)] {
			return info, stage, fmt.Errorf("重复备份路径")
		}
		if f.UncompressedSize64 > uint64(backupLimit) {
			return info, stage, fmt.Errorf("备份文件过大")
		}
		if f.Name != "manifest.json" && f.Name != "history.db" && f.Name != "marks.db" && f.Name != "preferences.json" && !strings.HasPrefix(f.Name, "imports/") {
			return info, stage, fmt.Errorf("备份包含未知文件")
		}
		limit := backupLimit - total
		if f.Name == "manifest.json" && limit > 4<<20 {
			limit = 4 << 20
		}
		if f.Name == "preferences.json" && limit > 64<<10 {
			limit = 64 << 10
		}
		if f.UncompressedSize64 > uint64(limit) {
			return info, stage, fmt.Errorf("备份文件大小超限")
		}
		p := filepath.Join(stage, filepath.FromSlash(f.Name))
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return info, stage, e
		}
		r, e := f.Open()
		if e != nil {
			return info, stage, e
		}
		dest, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			r.Close()
			return info, stage, e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(dest, h), io.LimitReader(r, limit+1))
		r.Close()
		closeErr := dest.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			return info, stage, e
		}
		if n > limit {
			return info, stage, fmt.Errorf("备份文件大小超限")
		}
		total += n
		foldedNames[strings.ToLower(f.Name)] = true
		entries[f.Name] = backupEntry{f.Name, n, hex.EncodeToString(h.Sum(nil))}
		if f.Name == "manifest.json" {
			data, e := os.ReadFile(p)
			if e != nil {
				return info, stage, e
			}
			if e = json.Unmarshal(data, &manifest); e != nil {
				return info, stage, fmt.Errorf("备份清单格式无效：%w", e)
			}
		}
	}
	if manifest.FormatVersion != 1 {
		return info, stage, fmt.Errorf("不支持的备份格式版本： %d", manifest.FormatVersion)
	}
	if _, e := time.Parse(time.RFC3339Nano, manifest.CreatedAt); e != nil {
		return info, stage, fmt.Errorf("无效备份时间")
	}
	if len(manifest.Files) != len(entries)-1 {
		return info, stage, fmt.Errorf("备份清单不完整")
	}
	seen := map[string]bool{}
	for _, entry := range manifest.Files {
		actual, ok := entries[entry.Path]
		if !ok || entry.Path == "manifest.json" || seen[entry.Path] || actual != entry {
			return info, stage, fmt.Errorf("备份清单或 SHA256 不匹配")
		}
		seen[entry.Path] = true
	}
	for _, name := range []string{"history.db", "marks.db", "preferences.json"} {
		if !seen[name] {
			return info, stage, fmt.Errorf("备份缺少 %s", name)
		}
	}
	if _, e := claudehistory.List(filepath.Join(stage, "imports")); e != nil {
		return info, stage, e
	}
	stats, e := history.ValidateBackup(filepath.Join(stage, "history.db"))
	if e != nil {
		return info, stage, e
	}
	if e = marks.ValidateBackup(filepath.Join(stage, "marks.db")); e != nil {
		return info, stage, e
	}
	prefs, e := os.ReadFile(filepath.Join(stage, "preferences.json"))
	if e != nil {
		return info, stage, e
	}
	if e = validatePreferences(string(prefs)); e != nil {
		return info, stage, e
	}
	return BackupInfo{manifest.CreatedAt, len(manifest.Files), total, stats.Sessions, stats.Versions, string(prefs)}, stage, nil
}
func (s *SessionService) InspectBackup(src string) (BackupInfo, error) {
	info, stage, err := inspectData(src)
	if stage != "" {
		defer os.RemoveAll(stage)
	}
	return info, err
}
func (s *SessionService) RestoreData(src, currentPreferences string) (report RestoreReport, err error) {
	if err = validatePreferences(currentPreferences); err != nil {
		return report, err
	}
	info, stage, err := inspectData(src)
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(stage)
	a := s.a
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	// Preflight every import conflict before the first mutation. Existing imports are immutable.
	type pendingFile struct{ src, dest string }
	pending := []pendingFile{}
	err = filepath.WalkDir(filepath.Join(stage, "imports"), func(p string, d os.DirEntry, e error) error {
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Join(stage, "imports"), p)
		dest := filepath.Join(a.cfg.ImportDir, rel)
		// Reject symlinks in every existing destination component.
		for cur := dest; ; cur = filepath.Dir(cur) {
			fi, e := os.Lstat(cur)
			if e == nil && fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("恢复目标含符号链接")
			}
			if cur == filepath.Dir(cur) {
				break
			}
		}
		digest, e := hashBackupFile(p)
		if e != nil {
			return e
		}
		old, e := hashBackupFile(dest)
		if e == nil {
			if digest != old {
				return fmt.Errorf("导入库存在冲突，未覆盖：%s", rel)
			}
			return nil
		}
		if !os.IsNotExist(e) {
			return e
		}
		pending = append(pending, pendingFile{p, dest})
		return nil
	})
	if err != nil {
		return report, err
	}
	report.RollbackPath, err = s.backupData(filepath.Join(filepath.Dir(a.cfg.LibraryPath), "data-recovery"), currentPreferences)
	if err != nil {
		return report, fmt.Errorf("恢复前自动备份失败： %w", err)
	}
	// Always refresh after a possible partial commit, including failed merges.
	defer func() {
		a.mu.Lock()
		a.loaded = false
		a.generation++
		a.messages = nil
		a.msgReady = false
		a.mu.Unlock()
		a.ensure()
		a.emit(EventSessionsChanged, "")
	}()
	created := []string{}
	defer func() {
		if err != nil {
			for _, p := range created {
				os.Remove(p)
			}
			err = fmt.Errorf("恢复未完成（各库独立事务；已合并历史不会删除）。自动备份： %s；%w", report.RollbackPath, err)
		}
	}()
	for _, p := range pending {
		if err = os.MkdirAll(filepath.Dir(p.dest), 0700); err != nil {
			return report, err
		}
		var f *os.File
		f, err = os.OpenFile(p.dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return report, err
		}
		created = append(created, p.dest)
		_, _, err = copyBackupFile(f, p.src, backupLimit)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return report, err
		}
	}
	if err = a.history.MergeBackup(filepath.Join(stage, "history.db")); err != nil {
		return report, err
	}
	if err = a.marks.MergeBackup(filepath.Join(stage, "marks.db")); err != nil {
		return report, err
	}

	report.Preferences = info.Preferences
	report.Message = "已合并恢复；现有历史正文及版本保留，标注采用备份。全文索引需要重建。"
	return report, nil
}
