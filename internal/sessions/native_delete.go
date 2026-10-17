package sessions

// Native deletion is deliberately separate from discovery: a preview never
// writes native stores, and removal requires durable, independently hashed
// recovery copies. No directory is ever removed.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/enpiaohj/agentdeck/internal/proc"
)

type nativeFile struct {
	Path    string      `json:"path"`
	Backup  string      `json:"backup"`
	Hash    string      `json:"sha256"`
	Mode    fs.FileMode `json:"mode"`
	removed bool
}

// NativeDeletePlan is a prepared deletion of one explicitly identified local
// session. Files is a preview only; changing it cannot change the deletion.
type NativeDeletePlan struct {
	Files        []string
	session      Session
	files        []nativeFile
	index        *nativeIndex
	histories    []*nativeIndex
	databases    []*nativeDatabase
	backup       string
	manifestHash string
	ready        bool
	deleted      bool
	mu           sync.Mutex
}

var nativeID = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_-]{0,159}$`)

// PrepareNativeDelete validates source ownership and takes immutable content
// fingerprints. It does not create a directory or change any native store.
func PrepareNativeDelete(s Session) (*NativeDeletePlan, error) {
	if s.Imported || s.Gateway || s.WSL != "" || (s.Agent != "codex" && s.Agent != "claude" && s.Agent != "claude-desktop") {
		return nil, fmt.Errorf("原始记录删除暂不支持此来源（%s/WSL/网关/导入）", s.Agent)
	}
	if !nativeID.MatchString(s.ID) {
		return nil, errors.New("原始会话 ID 无效")
	}
	p := &NativeDeletePlan{session: s}
	paths, err := nativePaths(s)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		hash, mode, err := nativeHash(path)
		if err != nil {
			return nil, err
		}
		p.files = append(p.files, nativeFile{Path: path, Hash: hash, Mode: mode})
		p.Files = append(p.Files, path)
	}
	if s.Path != "" {
		found := false
		for _, path := range paths {
			if nativeEqualPath(path, s.Path) {
				found = true
			}
		}
		if !found {
			if _, err := os.Lstat(s.Path); err == nil || !s.Retained || !os.IsNotExist(err) || !nativeMissingOwned(s) {
				return nil, errors.New("会话路径不属于已确认的原始来源目录或 ID")
			}
		}
	} else if !s.Retained {
		return nil, errors.New("原始会话路径缺失")
	}
	if s.Agent == "codex" {
		if err = p.prepareCodexMetadata(); err != nil {
			return nil, err
		}
	}
	if err = p.prepareHistory(); err != nil {
		return nil, err
	}
	return p, nil
}

func nativeMissingOwned(s Session) bool {
	if nativeSafePath(s.Path) != nil {
		return false
	}
	if s.Agent == "codex" {
		m := rolloutName.FindStringSubmatch(filepath.Base(s.Path))
		if m == nil || m[1] != s.ID {
			return false
		}
		for _, kind := range []string{"sessions", "archived_sessions"} {
			rel, err := filepath.Rel(filepath.Join(CodexDir(), kind), s.Path)
			if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	// A missing private Desktop source must still match its full discovered
	// root. Shared CLI sources use the same projects layout.
	homes := []string{ClaudeDir()}
	if s.Agent == "claude-desktop" {
		for _, d := range DesktopDataDirs() {
			for _, kind := range []string{"local-agent-mode-sessions", "claude-code-sessions"} {
				hs, _ := filepath.Glob(filepath.Join(d, kind, "*", "*", "local_*", ".claude"))
				homes = append(homes, hs...)
			}
		}
	}
	for _, home := range homes {
		rel, err := filepath.Rel(filepath.Join(home, "projects"), s.Path)
		if err != nil {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) == 2 && parts[0] != ".." && parts[1] == s.ID+".jsonl" {
			return true
		}
	}
	return false
}

func nativeEqualPath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Validate every existing ancestor, including junctions which older Go
// versions may not flag as symlinks. Missing leaves are permitted for undo.
func nativeSafePath(path string) error {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return errors.New("原始路径必须是本机绝对路径")
	}
	for cur := filepath.Clean(path); ; cur = filepath.Dir(cur) {
		fi, err := os.Lstat(cur)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("拒绝删除链接来源: %s", cur)
			}
			real, err := proc.RealPath(cur)
			if err != nil {
				return err
			}
			if !nativeEqualPath(real, cur) {
				return fmt.Errorf("拒绝删除 junction/重定向来源: %s", cur)
			}
		}
		if filepath.Dir(cur) == cur {
			break
		}
	}
	return nil
}

func nativePaths(s Session) ([]string, error) {
	found := map[string]bool{}
	add := func(path string) error {
		if err := nativeSafePath(path); err != nil {
			return err
		}
		found[path] = true
		return nil
	}
	walk := func(root string, match func(string) bool) error {
		if err := nativeSafePath(root); err != nil {
			return err
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("来源目录含链接，无法安全确认完整范围: %s", path)
			}
			if d.IsDir() {
				return nativeSafePath(path)
			}
			if match(path) {
				return add(path)
			}
			return nil
		})
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if s.Agent == "codex" {
		for _, kind := range []string{"sessions", "archived_sessions"} {
			if err := walk(filepath.Join(CodexDir(), kind), func(path string) bool {
				m := rolloutName.FindStringSubmatch(filepath.Base(path))
				return m != nil && m[1] == s.ID
			}); err != nil {
				return nil, err
			}
		}
	} else {
		homes := []string{ClaudeDir()}
		if s.Agent == "claude-desktop" {
			for _, d := range DesktopDataDirs() {
				for _, kind := range []string{"local-agent-mode-sessions", "claude-code-sessions"} {
					hs, err := filepath.Glob(filepath.Join(d, kind, "*", "*", "local_*", ".claude"))
					if err != nil {
						return nil, err
					}
					homes = append(homes, hs...)
				}
			}
		}
		for _, home := range homes {
			projects := filepath.Join(home, "projects")
			if err := nativeSafePath(projects); err != nil {
				return nil, err
			}
			entries, err := os.ReadDir(projects)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
					continue
				}
				project := filepath.Join(projects, e.Name())
				if err := nativeSafePath(project); err != nil {
					return nil, err
				}
				main := filepath.Join(project, s.ID+".jsonl")
				if _, err := os.Lstat(main); err == nil {
					if err = add(main); err != nil {
						return nil, err
					}
				} else if !os.IsNotExist(err) {
					return nil, err
				}
				if err := walk(filepath.Join(project, s.ID, "subagents"), func(path string) bool {
					return strings.HasSuffix(path, ".jsonl") && (strings.HasPrefix(filepath.Base(path), "agent-") || filepath.Base(path) == "journal.jsonl")
				}); err != nil {
					return nil, err
				}
			}
		}
	}
	out := make([]string, 0, len(found))
	for path := range found {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func nativeHash(path string) (string, fs.FileMode, error) {
	if err := nativeSafePath(path); err != nil {
		return "", 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	if !before.Mode().IsRegular() {
		return "", 0, errors.New("原始记录不是普通文件")
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", 0, err
	}
	after, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", 0, errors.New("原始记录读取期间发生变化")
	}
	return hex.EncodeToString(h.Sum(nil)), before.Mode().Perm(), nil
}

func nativeCopy(src, dst, hash string, mode fs.FileMode) error {
	if err := nativeSafePath(src); err != nil {
		return err
	}
	if err := nativeSafePath(dst); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), in)
	if err == nil && hex.EncodeToString(h.Sum(nil)) != hash {
		err = errors.New("备份来源已发生变化")
	}
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	got, _, err := nativeHash(dst)
	if err != nil {
		return err
	}
	if got != hash {
		return errors.New("备份 SHA256 校验失败")
	}
	return nil
}

func nativeWriteExclusive(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	end := f.Close()
	if err == nil {
		err = end
	}
	return err
}

// Backup persists a manifest, targeted metadata rows, and source files. The
// supplied managed directory must be empty; existing recovery data is never
// replaced. Ready is set only after every recovery copy has been verified.
func (p *NativeDeletePlan) Backup(dir string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ready {
		return errors.New("此计划已经备份")
	}
	if err := p.validate(); err != nil {
		return err
	}
	if err := nativeSafePath(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("原始记录备份目录必须为空")
	}
	for i := range p.files {
		f := &p.files[i]
		f.Backup = filepath.Join(dir, fmt.Sprintf("%04d.source", i))
		if err = nativeCopy(f.Path, f.Backup, f.Hash, 0600); err != nil {
			return err
		}
	}
	manifest := struct {
		Version   int
		Agent, ID string
		Files     []nativeFile
		Index     *nativeIndex
		Histories []*nativeIndex
		Databases []*nativeDatabase
		Recovery  string
	}{1, p.session.Agent, p.session.ID, p.files, p.index, p.histories, p.databases, "先关闭原始客户端。仅在原路径不存在且 SHA256 校验正确时，将 .source 恢复至 Files.Path；不得覆盖重建文件。SQLite 仅在结构匹配、所选会话行均不存在时，在一个事务中依次执行保存的 Tables.Inserts，禁止替换整个数据库。仅在所选 ID 没有新索引行时追加 Index.Lines 或 Histories.Lines，保留其他会话行。此备份包含私密会话内容，请妥善保管。"}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = nativeWriteExclusive(filepath.Join(dir, "manifest.json"), b); err != nil {
		return err
	}
	p.manifestHash, _, err = nativeHash(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	if err = p.validate(); err != nil {
		return err
	}
	p.backup = dir
	p.ready = true
	return nil
}

func (p *NativeDeletePlan) validate() error {
	paths, err := nativePaths(p.session)
	if err != nil {
		return err
	}
	expected := make([]string, len(p.files))
	for i, f := range p.files {
		expected[i] = f.Path
	}
	if !reflect.DeepEqual(paths, expected) {
		return errors.New("原始记录文件范围发生变化，请重新预览")
	}
	for _, f := range p.files {
		hash, _, err := nativeHash(f.Path)
		if err != nil {
			return err
		}
		if hash != f.Hash {
			return fmt.Errorf("原始记录已发生变化，请重新预览: %s", f.Path)
		}
	}
	return p.validateMetadata()
}

// Delete requires a verified backup and revalidates all sources before the
// first mutation. A caller must invoke Restore when any later step fails.
func (p *NativeDeletePlan) Delete() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.ready {
		return errors.New("必须先完成并验证原始记录备份")
	}
	if err := p.validate(); err != nil {
		return err
	}
	manifestHash, _, err := nativeHash(filepath.Join(p.backup, "manifest.json"))
	if err != nil {
		return err
	}
	if manifestHash != p.manifestHash {
		return errors.New("恢复清单已发生变化，拒绝删除")
	}
	for _, f := range p.files {
		hash, _, err := nativeHash(f.Backup)
		if err != nil {
			return err
		}
		if hash != f.Hash {
			return errors.New("恢复备份已损坏，拒绝删除")
		}
	}
	if err := p.deleteMetadata(); err != nil {
		return err
	}
	for i := range p.files {
		f := &p.files[i]
		hash, _, err := nativeHash(f.Path)
		if err != nil {
			return err
		}
		if hash != f.Hash {
			return errors.New("原始记录已发生变化，删除已中止")
		}
		if err = os.Remove(f.Path); err != nil {
			return err
		}
		f.removed = true
	}
	p.deleted = true
	return nil
}

// Restore never overwrites a rewritten native file or a new metadata row.
// Intact sources and plans which did not mutate anything are safe no-ops.
func (p *NativeDeletePlan) Restore() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var errs []error
	for i := range p.files {
		f := &p.files[i]
		if !f.removed {
			continue
		}
		if err := nativeSafePath(f.Path); err != nil {
			errs = append(errs, err)
			continue
		}
		if hash, _, err := nativeHash(f.Path); err == nil {
			if hash == f.Hash {
				f.removed = false
			} else {
				errs = append(errs, fmt.Errorf("恢复冲突，来源已重建: %s", f.Path))
			}
			continue
		} else if !os.IsNotExist(err) {
			errs = append(errs, err)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.Path), 0700); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := nativeRestoreFile(*f); err != nil {
			errs = append(errs, err)
			continue
		}
		f.removed = false
	}
	if err := p.restoreMetadata(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// A fully copied, synced temporary file is linked into place without replacing
// an existing file. A failed copy therefore never leaves a partial source.
func nativeRestoreFile(f nativeFile) error {
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".agentdeck-restore-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	tmp.Close()
	if err = os.Remove(name); err != nil {
		return err
	}
	defer os.Remove(name)
	if err = nativeCopy(f.Backup, name, f.Hash, f.Mode); err != nil {
		return err
	}
	if err = nativeSafePath(f.Path); err != nil {
		return err
	}
	return os.Link(name, f.Path)
}
