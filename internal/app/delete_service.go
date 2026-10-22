package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/history"
	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

type DeletePreview struct {
	Token       string   `json:"token"`
	Sessions    int      `json:"sessions"`
	NativeFiles int      `json:"native_files"`
	Metadata    int      `json:"metadata_files"`
	Children    int      `json:"children"`
	Titles      []string `json:"titles"`
	Paths       []string `json:"paths"`
	Bytes       int64    `json:"bytes"`
}
type DeleteReport struct {
	DeletedKeys []string `json:"deleted_keys"`
	BackupPath  string   `json:"backup_path"`
	UndoToken   string   `json:"undo_token"`
	Message     string   `json:"message"`
}
type deleteJob struct {
	created         time.Time
	keys, ownedKeys []string
	sessions        []sessions.Session
	native          []*sessions.NativeDeletePlan
	dir             string
}
type deletionRecord struct {
	Version int      `json:"version"`
	State   string   `json:"state"`
	Keys    []string `json:"keys"`
	Owned   []string `json:"owned_keys"`
}

func deleteToken() string {
	var data [24]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}
func deletionDir(library string) string { return filepath.Join(filepath.Dir(library), "deletions") }

func loadDeletedKeys(library string) (map[string]bool, error) {
	out := map[string]bool{}
	entries, err := os.ReadDir(deletionDir(library))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "delete-") {
			continue
		}
		path := filepath.Join(deletionDir(library), entry.Name(), "operation.json")
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil, fmt.Errorf("无法校验删除恢复记录：%s", path)
		}
		data, err := os.ReadFile(path)
		var record deletionRecord
		if err != nil || json.Unmarshal(data, &record) != nil || record.Version != 1 {
			return nil, fmt.Errorf("删除恢复记录损坏：%s", path)
		}
		if record.State == "deleting" || record.State == "deleted" || record.State == "restoring" || record.State == "needs-recovery" {
			for _, key := range append(record.Keys, record.Owned...) {
				out[key] = true
			}
		}
	}
	return out, nil
}
func (job *deleteJob) saveState(state string) error {
	data, err := json.MarshalIndent(deletionRecord{1, state, job.keys, job.ownedKeys}, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(job.dir, ".operation-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(job.dir, "operation.json"))
}

// The token binds confirmation to the exact selection and native fingerprints.
func (s *SessionService) PreviewDeleteSessions(keys []string, includeChildren bool) (DeletePreview, error) {
	a := s.a
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	list, byKey, ms := a.snapshot()
	if len(keys) == 0 || len(keys) > 1000 {
		return DeletePreview{}, errors.New("请选择 1 至 1000 个会话")
	}
	chosen, extras := map[string]bool{}, map[string]bool{}
	for _, key := range keys {
		if _, ok := byKey[key]; !ok {
			return DeletePreview{}, fmt.Errorf("会话已不存在，请刷新列表：%s", key)
		}
		chosen[key] = true
	}
	for changed := true; changed; {
		changed = false
		for _, child := range list {
			key := child.Agent + ":" + child.ID
			if child.Agent != "codex" || child.Role != "subagent" || chosen[key] || extras[key] {
				continue
			}
			if chosen["codex:"+child.ParentSession] || extras["codex:"+child.ParentSession] || chosen["codex:"+child.RootSession] {
				extras[key], changed = true, true
			}
		}
	}
	if includeChildren {
		for key := range extras {
			chosen[key] = true
		}
	}
	if len(chosen) > 1000 {
		return DeletePreview{}, errors.New("包含子会话后超过单次删除上限 1000 个")
	}
	job := &deleteJob{created: time.Now()}
	preview := DeletePreview{Token: deleteToken(), Children: len(extras), Titles: []string{}, Paths: []string{}}
	paths, owned := map[string]bool{}, map[string]bool{}
	for _, sess := range list {
		key := sess.Agent + ":" + sess.ID
		if !chosen[key] {
			continue
		}
		plan, err := sessions.PrepareNativeDelete(sess)
		if err != nil {
			return DeletePreview{}, fmt.Errorf("%s：%w", displayTitle(sess, ms[sessionMarkKey(sess, key, ms)]), err)
		}
		job.keys, job.sessions, job.native = append(job.keys, key), append(job.sessions, sess), append(job.native, plan)
		owned[key] = true
		if legacy := sessionMarkKey(sess, key, ms); legacy != key {
			if _, separate := byKey[legacy]; !separate {
				owned[legacy] = true
			}
		}
		preview.Titles = append(preview.Titles, displayTitle(sess, ms[sessionMarkKey(sess, key, ms)]))
		// Only the unlinked files count toward "deleted files" and bytes; the
		// rewritten state DB and shared indexes keep their file on disk.
		for _, path := range plan.Files {
			if paths[path] {
				continue
			}
			paths[path] = true
			preview.Paths = append(preview.Paths, path)
			info, err := os.Stat(path)
			if err != nil {
				return DeletePreview{}, err
			}
			if slices.Contains(plan.Remove, path) {
				preview.Bytes += info.Size()
			} else {
				preview.Metadata++
			}
		}
	}
	for key := range owned {
		job.ownedKeys = append(job.ownedKeys, key)
	}
	sort.Strings(job.ownedKeys)
	sort.Strings(preview.Paths)
	preview.Sessions = len(job.keys)
	removed := map[string]bool{}
	for _, plan := range job.native {
		for _, path := range plan.Remove {
			if removed[path] {
				continue
			}
			removed[path] = true
			preview.NativeFiles++
		}
	}
	if a.deletePlans == nil {
		a.deletePlans = map[string]*deleteJob{}
	}
	for token, existing := range a.deletePlans {
		if time.Since(existing.created) > 10*time.Minute {
			delete(a.deletePlans, token)
		}
	}
	if len(a.deletePlans) >= 64 {
		return DeletePreview{}, errors.New("待确认的删除请求过多，请稍后重试")
	}
	a.deletePlans[preview.Token] = job
	return preview, nil
}
func (a *App) blockDeleted(keys []string, blocked bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, key := range keys {
		if blocked {
			a.deleted[key] = true
		} else {
			delete(a.deleted, key)
		}
	}
}
func (s *SessionService) refreshAfterDelete() {
	sessions.Reset()
	s.Refresh()
	s.a.NewSearchService().Rebuild()
}

// DeleteSessions 先写恢复备份再删除，成功后可在本次运行内撤销。
func (s *SessionService) DeleteSessions(token string) (DeleteReport, error) {
	return s.deleteSessions(token, true)
}

// DeleteSessionsPermanently 是「直接删除」：流程、校验与失败回滚都与
// DeleteSessions 完全相同——恢复备份仍会先写、先验，因为失败时自动回滚
// 全靠它——区别只在删除全部成功之后：丢弃这份备份，不提供撤销。
// 要换来的是磁盘上不再留有被删会话的私密副本。
func (s *SessionService) DeleteSessionsPermanently(token string) (DeleteReport, error) {
	return s.deleteSessions(token, false)
}

func (s *SessionService) deleteSessions(token string, keepBackup bool) (DeleteReport, error) {
	a := s.a
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	job := a.deletePlans[token]
	delete(a.deletePlans, token)
	if job == nil || time.Since(job.created) > 10*time.Minute {
		return DeleteReport{}, errors.New("删除预览已失效，请重新核对并确认")
	}
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	if a.closing {
		return DeleteReport{}, errors.New("程序正在关闭")
	}
	for i, sess := range job.sessions {
		fresh, err := sessions.PrepareNativeDelete(sess)
		if err != nil {
			return DeleteReport{}, fmt.Errorf("原始记录范围已变化，请重新预览：%w", err)
		}
		if !slices.Equal(fresh.Files, job.native[i].Files) || !slices.Equal(fresh.Remove, job.native[i].Remove) {
			return DeleteReport{}, errors.New("原始记录范围已变化，请重新预览")
		}
	}
	root := deletionDir(a.cfg.LibraryPath)
	if err := os.MkdirAll(root, 0700); err != nil {
		return DeleteReport{}, fmt.Errorf("无法创建恢复备份，未删除任何数据：%w", err)
	}
	dir, err := os.MkdirTemp(root, "delete-"+time.Now().Format("20060102-150405")+"-")
	if err != nil {
		return DeleteReport{}, err
	}
	job.dir = dir
	backupError := func(err error) (DeleteReport, error) {
		return DeleteReport{}, fmt.Errorf("备份未完成，未开始删除。备份目录：%s；%w", dir, err)
	}
	// 只备份被删会话自己的行，不再整库 VACUUM INTO：恢复路径与校验都按 key
	// 工作，整库拷贝的只是那几行，库越大越慢（实测 145 MB 的库要 1.1 秒）。
	if err = a.history.BackupKeysTo(filepath.Join(dir, "history.db"), job.ownedKeys); err != nil {
		return backupError(err)
	}
	if _, err = history.ValidateBackup(filepath.Join(dir, "history.db")); err != nil {
		return backupError(err)
	}
	if err = a.marks.BackupKeysTo(filepath.Join(dir, "marks.db"), job.ownedKeys); err != nil {
		return backupError(err)
	}
	if err = marks.ValidateBackup(filepath.Join(dir, "marks.db")); err != nil {
		return backupError(err)
	}
	for i, plan := range job.native {
		if err = plan.Backup(filepath.Join(dir, fmt.Sprintf("native-%04d", i))); err != nil {
			return backupError(err)
		}
	}
	if err = job.saveState("deleting"); err != nil {
		return backupError(err)
	}
	a.blockDeleted(job.ownedKeys, true)
	started, historyGone, marksGone := 0, false, false
	rollback := func(cause error) (DeleteReport, error) {
		var failures []error
		for i := started - 1; i >= 0; i-- {
			failures = append(failures, job.native[i].Restore())
		}
		if historyGone {
			failures = append(failures, a.history.RestoreKeysFrom(filepath.Join(dir, "history.db"), job.ownedKeys))
		}
		if marksGone {
			failures = append(failures, a.marks.RestoreKeysFrom(filepath.Join(dir, "marks.db"), job.ownedKeys))
		}
		if restoreErr := errors.Join(failures...); restoreErr != nil {
			_ = job.saveState("needs-recovery")
			s.refreshAfterDelete()
			return DeleteReport{}, fmt.Errorf("删除未完成，自动恢复未全部成功。恢复备份：%s；%w；恢复：%v", dir, cause, restoreErr)
		}
		if err := job.saveState("restored"); err != nil {
			return DeleteReport{}, fmt.Errorf("数据已回退，恢复状态写入失败。备份：%s；%w", dir, err)
		}
		a.blockDeleted(job.ownedKeys, false)
		s.refreshAfterDelete()
		return DeleteReport{}, fmt.Errorf("删除未完成，已自动恢复。备份：%s；%w", dir, cause)
	}
	for i, plan := range job.native {
		if err = plan.RebaseAfterOwnBatchChange(job.native[:i]); err != nil {
			return rollback(err)
		}
		started++
		if a.nativeDelete != nil {
			err = a.nativeDelete(plan)
		} else {
			err = plan.Delete()
		}
		if err != nil {
			return rollback(err)
		}
	}
	if err = a.history.DeleteKeys(job.ownedKeys); err != nil {
		return rollback(err)
	}
	historyGone = true
	if err = a.marks.DeleteKeys(job.ownedKeys); err != nil {
		return rollback(err)
	}
	marksGone = true
	if err = job.saveState("deleted"); err != nil {
		return rollback(err)
	}
	if !keepBackup {
		return s.finishPermanent(job), nil
	}
	undo := deleteToken()
	if a.deleteUndos == nil {
		a.deleteUndos = map[string]*deleteJob{}
	}
	a.deleteUndos[undo] = job
	s.refreshAfterDelete()
	a.emit("sessions:deleted", append([]string(nil), job.keys...))
	return DeleteReport{append([]string(nil), job.keys...), dir, undo, fmt.Sprintf("已删除 %d 个会话，原始记录与本地副本已移除。", len(job.keys))}, nil
}

// finishPermanent 收尾「直接删除」：丢弃恢复备份，只留下 operation.json
// （仅含会话 key，不含内容），让已删会话不会被其他来源重新收录。
// 丢弃失败不影响删除结果——此时数据已删净，只是备份文件没清干净，如实告知。
func (s *SessionService) finishPermanent(job *deleteJob) DeleteReport {
	var errs []error
	for _, plan := range job.native {
		errs = append(errs, plan.DiscardBackup())
	}
	for _, name := range []string{"history.db", "marks.db"} {
		if err := os.Remove(filepath.Join(job.dir, name)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	s.refreshAfterDelete()
	s.a.emit("sessions:deleted", append([]string(nil), job.keys...))
	message := fmt.Sprintf("已直接删除 %d 个会话，未保留恢复备份，无法撤销。", len(job.keys))
	backupPath := ""
	if err := errors.Join(errs...); err != nil {
		message += fmt.Sprintf("但部分临时备份文件未能清理，可手动删除目录：%s（%v）", job.dir, err)
		backupPath = job.dir
	}
	return DeleteReport{append([]string(nil), job.keys...), backupPath, "", message}
}

// Undo takes an opaque token, never a caller-supplied filesystem path.
func (s *SessionService) RestoreDeletedSessions(token string) (string, error) {
	a := s.a
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	job := a.deleteUndos[token]
	if job == nil {
		return "", errors.New("撤销请求已失效；请使用删除时保存的恢复备份")
	}
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	if a.closing {
		return "", errors.New("程序正在关闭")
	}
	if err := job.saveState("restoring"); err != nil {
		return "", err
	}
	fail := func(err error) (string, error) {
		return "", fmt.Errorf("恢复未完成，未覆盖冲突内容；恢复备份：%s；%w", job.dir, err)
	}
	for i := len(job.native) - 1; i >= 0; i-- {
		if err := job.native[i].Restore(); err != nil {
			return fail(err)
		}
	}
	if err := a.history.RestoreKeysFrom(filepath.Join(job.dir, "history.db"), job.ownedKeys); err != nil {
		return fail(err)
	}
	if err := a.marks.RestoreKeysFrom(filepath.Join(job.dir, "marks.db"), job.ownedKeys); err != nil {
		return fail(err)
	}
	if err := job.saveState("restored"); err != nil {
		return fail(err)
	}
	a.blockDeleted(job.ownedKeys, false)
	delete(a.deleteUndos, token)
	s.refreshAfterDelete()
	return fmt.Sprintf("已恢复 %d 个会话的原始记录与本地内容。", len(job.keys)), nil
}
