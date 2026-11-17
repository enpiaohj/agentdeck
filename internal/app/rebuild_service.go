package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/enpiaohj/agentdeck/internal/rebuild"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// 已被 Claude Code 清理的会话（它只保留约 30 天，见 ClaudeRetention），AgentDeck 的永久库里
// 还留着内容，但没有原始 .jsonl，所以不能原样还原。这里提供两条继续的路：
//
//   - 重建后恢复（PreviewRebuild / Rebuild）：把保留的内容重建成会话文件，写回 Claude Code 的
//     项目文件夹，之后用 claude --resume 继续。这是除“手动删除会话”之外，AgentDeck 唯一写上游
//     存储的地方，约束与删除同级严格：只对用户显式选择、预览确认的会话；只新建文件，绝不覆盖
//     已有文件；只写 <Claude 目录>/projects/<由目录名推出的文件夹>/<会话 ID>.jsonl；写完回读校验。
//   - 带历史新开会话（ContinueWithHistory）：不写上游，只在 AgentDeck 自己的数据目录里写一份
//     交接文档，新开 Claude 会话让它读。

// rebuildPlan 是预览之后、确认之前保存的计划；token 绑定会话、目录与内容哈希。
type rebuildPlan struct {
	key, dir, target string
	hash             [32]byte
	expires          time.Time
}

// RebuildPreview 是重建前给用户确认的信息。
type RebuildPreview struct {
	Key            string `json:"key"`
	Title          string `json:"title"`
	Original       string `json:"original"`        // 会话记录的原目录
	Directory      string `json:"directory"`       // 将为它重建的目录；原目录不存在时为空
	NeedsDirectory bool   `json:"needs_directory"` // 原目录不存在，要先选一个目录
	File           string `json:"file"`            // 将写入的会话文件
	Turns          int    `json:"turns"`
	Kept           int    `json:"kept"`
	Omitted        int    `json:"omitted"`
	Tools          int    `json:"tools"`
	Chars          int    `json:"chars"`
	Bytes          int    `json:"bytes"`
	Token          string `json:"token"`
}

// RebuildResult 是写入之后的结果。
type RebuildResult struct {
	Key   string `json:"key"`
	File  string `json:"file"`
	Bytes int    `json:"bytes"`
}

// HandoffResult 是“带历史新开会话”启动的结果。
type HandoffResult struct {
	File    string `json:"file"`
	Omitted int    `json:"omitted"`
}

const rebuildTokenTTL = 10 * time.Minute

// rebuildable 校验会话是 Claude Code 的、已被清理（只剩永久库副本）的本机会话。
func (a *App) rebuildable(key string) (sessions.Session, error) {
	_, byKey, _ := a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return sess, fmt.Errorf("会话不存在: %s", key)
	}
	switch {
	case sess.Agent != "claude":
		return sess, errors.New("只支持 Claude Code 的会话")
	case sess.WSL != "" || sess.Gateway || sess.Imported:
		return sess, errors.New("WSL、网关与导入的会话不支持重建")
	case !sess.Retained:
		return sess, errors.New("这个会话在 Claude Code 里还在，直接继续即可，不需要重建")
	case !rebuild.ValidID(sess.ID):
		return sess, errors.New("会话 ID 不是有效的 Claude 会话标识")
	}
	return sess, nil
}

// rebuildTarget 算出会话文件的落点：<Claude 目录>/projects/<项目文件夹>/<ID>.jsonl。
// 项目文件夹名只含字母数字和 "-"，ID 已校验，所以落点不可能跳出 projects 目录。
func rebuildTarget(id, dir string) (string, error) {
	name, err := rebuild.ProjectDirName(dir)
	if err != nil {
		return "", err
	}
	root := filepath.Join(sessions.ClaudeDir(), "projects")
	target := filepath.Join(root, name, id+".jsonl")
	if rel, err := filepath.Rel(root, target); err != nil || filepath.IsLocal(rel) == false {
		return "", errors.New("会话文件的位置不在 Claude Code 的项目目录内")
	}
	return target, nil
}

func (a *App) rebuildData(sess sessions.Session, dir string) ([]byte, rebuild.Stats, error) {
	tr, _, _, err := a.readConversation(sess)
	if err != nil {
		return nil, rebuild.Stats{}, err
	}
	return rebuild.Build(sess, tr, dir, time.Now(), rebuild.DefaultBudget)
}

// PreviewRebuild 预览“重建后恢复”：dir 是要为它重建的目录，空表示用会话记录的原目录。
// 不写任何文件；返回的 token 只在 10 分钟内、对同一会话与目录有效。
func (s *ResumeService) PreviewRebuild(key, dir string) (RebuildPreview, error) {
	a := s.a
	sess, err := a.rebuildable(key)
	if err != nil {
		return RebuildPreview{}, err
	}
	out := RebuildPreview{Key: key, Title: sess.Title, Original: sess.Cwd}
	explicit := dir != ""
	if !explicit {
		dir = sess.Cwd
	}
	if dir != "" {
		if dir, err = existingDir(dir); err != nil {
			if explicit {
				return out, err
			}
			dir = ""
		}
	}
	if dir == "" {
		out.NeedsDirectory = true
		return out, nil
	}
	if _, err := os.Stat(filepath.Join(sessions.ClaudeDir())); err != nil {
		return out, fmt.Errorf("没有找到 Claude Code 的数据目录（%s）：%w", sessions.ClaudeDir(), err)
	}
	target, err := rebuildTarget(sess.ID, dir)
	if err != nil {
		return out, err
	}
	if _, err := os.Lstat(target); err == nil {
		return out, fmt.Errorf("会话文件已经存在，不会覆盖：%s", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	data, st, err := a.rebuildData(sess, dir)
	if err != nil {
		return out, err
	}
	token := rebuildToken()
	a.dataMu.Lock()
	if a.rebuildPlans == nil {
		a.rebuildPlans = map[string]*rebuildPlan{}
	}
	for k, p := range a.rebuildPlans {
		if time.Now().After(p.expires) {
			delete(a.rebuildPlans, k)
		}
	}
	a.rebuildPlans[token] = &rebuildPlan{key: key, dir: dir, target: target, hash: sha256.Sum256(data), expires: time.Now().Add(rebuildTokenTTL)}
	a.dataMu.Unlock()
	out.Directory, out.File, out.Token = dir, target, token
	out.Turns, out.Kept, out.Omitted, out.Tools, out.Chars, out.Bytes = st.Turns, st.Kept, st.Omitted, st.Tools, st.Chars, st.Bytes
	return out, nil
}

// Rebuild 执行预览确认过的重建：重新生成内容并核对哈希，只新建文件（目标已存在就拒绝，
// 绝不覆盖），先写同目录的临时文件再用硬链接放到位，写完回读校验。
func (s *ResumeService) Rebuild(token string) (RebuildResult, error) {
	a := s.a
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	plan := a.rebuildPlans[token]
	delete(a.rebuildPlans, token) // 一次性
	if plan == nil || time.Now().After(plan.expires) {
		return RebuildResult{}, errors.New("预览已过期，请重新预览")
	}
	sess, err := a.rebuildable(plan.key)
	if err != nil {
		return RebuildResult{}, err
	}
	data, st, err := a.rebuildData(sess, plan.dir)
	if err != nil {
		return RebuildResult{}, err
	}
	if sha256.Sum256(data) != plan.hash {
		return RebuildResult{}, errors.New("会话内容在预览之后变了，请重新预览")
	}
	if err := writeNewFile(plan.target, data); err != nil {
		return RebuildResult{}, err
	}
	(&SessionService{a: a}).Refresh()
	return RebuildResult{Key: plan.key, File: plan.target, Bytes: st.Bytes}, nil
}

// writeNewFile 只新建 target：目录按需创建（0700），内容先写同目录的临时文件并落盘，再用硬链接
// 放到目标名下（目标已存在则失败，不会覆盖），最后回读比对。任何一步失败都不留下半截文件。
func writeNewFile(target string, data []byte) (err error) {
	if _, e := os.Lstat(target); e == nil {
		return fmt.Errorf("会话文件已经存在，不会覆盖：%s", target)
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("无法创建项目文件夹：%w", err)
	}
	tmp, err := os.CreateTemp(dir, ".agentdeck-rebuild-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err = os.Link(tmpName, target); err != nil {
		if _, e := os.Lstat(target); e == nil {
			return fmt.Errorf("会话文件已经存在，不会覆盖：%s", target)
		}
		// 文件系统不支持硬链接：退回「确认不存在再改名」
		if err = os.Rename(tmpName, target); err != nil {
			return err
		}
	}
	back, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(back, data) {
		os.Remove(target)
		return errors.New("写入后回读校验失败，已撤销本次写入")
	}
	return nil
}

func rebuildToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// handoffDir 是交接文档所在目录：AgentDeck 自己的数据目录（与 deletions/ 同级），不在上游。
func (a *App) handoffDir() string { return filepath.Join(filepath.Dir(a.cfg.LibraryPath), "handoff") }

// ContinueWithHistory 在 dir（空表示原目录）新开一个 Claude 会话，并让它先读取保留下来的历史。
// 不写上游存储：交接文档写在 AgentDeck 自己的数据目录里，启动命令用 --add-dir 授权读取它。
func (s *ResumeService) ContinueWithHistory(key, dir string) (HandoffResult, error) {
	a := s.a
	sess, err := a.rebuildable(key)
	if err != nil {
		return HandoffResult{}, err
	}
	if dir == "" {
		dir = sess.Cwd
	}
	if dir == "" {
		return HandoffResult{}, errors.New("原目录不存在，请先选择一个目录")
	}
	if dir, err = existingDir(dir); err != nil {
		return HandoffResult{}, err
	}
	tr, _, _, err := a.readConversation(sess)
	if err != nil {
		return HandoffResult{}, err
	}
	md, st, err := rebuild.Handoff(sess, tr, rebuild.DefaultBudget)
	if err != nil {
		return HandoffResult{}, err
	}
	folder := a.handoffDir()
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return HandoffResult{}, fmt.Errorf("无法创建交接文档目录：%w", err)
	}
	file := filepath.Join(folder, sess.ID+".md")
	tmp, err := os.CreateTemp(folder, ".handoff-*.tmp")
	if err != nil {
		return HandoffResult{}, err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(md); err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return HandoffResult{}, err
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		return HandoffResult{}, err
	}
	prompt := "请先阅读 " + file + "。那是我们此前对话的文字记录（原会话已被清理，无法直接恢复）。读完后用两三句话说明你理解的上下文和进展，然后等我的下一步指示。"
	script := fmt.Sprintf("Set-Location -LiteralPath %s -ErrorAction Stop; %s --add-dir %s %s",
		quotePowerShellPath(dir), cliCommand("claude"), quotePowerShellPath(folder), quoteShellArg(prompt))
	if err := s.launchInTerminal(dir, script); err != nil {
		return HandoffResult{File: file, Omitted: st.Omitted}, err
	}
	return HandoffResult{File: file, Omitted: st.Omitted}, nil
}
