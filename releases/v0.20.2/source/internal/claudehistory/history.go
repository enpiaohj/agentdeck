// Package claudehistory imports account-scoped Claude chat exports into an
// AgentDeck-owned store. Native Claude folders and source exports are read only.
package claudehistory

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const maxExport = 512 << 20

type Report struct {
	Conversations   int    `json:"conversations"`
	Account         string `json:"account"`
	AlreadyImported bool   `json:"already_imported"`
	SkippedBlocks   int    `json:"skipped_blocks"`
}

type entry struct {
	ID, Title        string
	Created, Updated time.Time
	Messages         int
}

type manifest struct {
	Version       int
	Account       string
	ImportedAt    time.Time
	SkippedBlocks int
	Entries       []entry
}

type conversation struct {
	UUID     string     `json:"uuid"`
	Name     string     `json:"name"`
	Created  time.Time  `json:"created_at"`
	Updated  time.Time  `json:"updated_at"`
	Messages *[]message `json:"chat_messages"`
}

type message struct {
	Sender  string                                  `json:"sender"`
	Text    string                                  `json:"text"`
	Created time.Time                               `json:"created_at"`
	Content []struct{ Type, Text, Thinking string } `json:"content"`
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Import accepts conversations.json or a ZIP containing that file. Every batch
// is staged and atomically published; repeated source bytes never overwrite data.
func Import(root, source, account string) (Report, error) {
	account = strings.TrimSpace(account)
	report := Report{Account: account}
	if account == "" || len([]rune(account)) > 80 {
		return report, errors.New("请填写 1–80 字的账号名称，用于区分不同账号的记录")
	}
	if root == "" {
		return report, errors.New("未配置聊天导入存储目录")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return report, err
	}
	f, err := os.Open(source)
	if err != nil {
		return report, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return report, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxExport {
		return report, errors.New("请选择不超过 512 MiB 的 Claude JSON 或 ZIP 导出文件")
	}
	hash := sha256.New()
	if size, err := io.Copy(hash, io.LimitReader(f, maxExport+1)); err != nil {
		return report, err
	} else if size > maxExport {
		return report, errors.New("导出文件超过 512 MiB")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return report, err
	}
	accountID := digest(strings.ToLower(account))
	accountRoot := filepath.Join(root, accountID)
	account, err = canonicalAccount(accountRoot, account)
	if err != nil {
		return report, err
	}
	report.Account = account
	batch := filepath.Join(accountRoot, hex.EncodeToString(hash.Sum(nil)))
	if data, err := os.ReadFile(filepath.Join(batch, "manifest.json")); err == nil {
		var prior manifest
		if json.Unmarshal(data, &prior) != nil || prior.Version != 1 {
			return report, errors.New("已有导入批次的清单损坏，已保留原文件")
		}
		report.Conversations, report.AlreadyImported, report.SkippedBlocks = len(prior.Entries), true, prior.SkippedBlocks
		return report, nil
	}
	var reader io.Reader = f
	var zipped io.ReadCloser
	if strings.EqualFold(filepath.Ext(source), ".zip") {
		archive, err := zip.NewReader(f, info.Size())
		if err != nil {
			return report, fmt.Errorf("无法读取 ZIP: %w", err)
		}
		var member *zip.File
		for _, file := range archive.File {
			if filepath.Base(strings.ReplaceAll(file.Name, "\\", "/")) == "conversations.json" && !file.FileInfo().IsDir() {
				if member != nil {
					return report, errors.New("ZIP 中含有多个 conversations.json，请分别导入")
				}
				member = file
			}
		}
		if member == nil || member.UncompressedSize64 > maxExport {
			return report, errors.New("ZIP 中未找到有效的 conversations.json，或解压后超过 512 MiB")
		}
		zipped, err = member.Open()
		if err != nil {
			return report, err
		}
		defer zipped.Close()
		reader = zipped
	} else if !strings.EqualFold(filepath.Ext(source), ".json") {
		return report, errors.New("仅支持 Claude 导出的 JSON 或 ZIP 文件")
	}
	if err := os.MkdirAll(accountRoot, 0o700); err != nil {
		return report, err
	}
	stage, err := os.MkdirTemp(accountRoot, ".stage-")
	if err != nil {
		return report, err
	}
	// stage is a directory created by this operation under the owned store.
	defer func() {
		if stage != "" {
			rel, err := filepath.Rel(accountRoot, stage)
			if err == nil && filepath.Dir(rel) == "." && strings.HasPrefix(filepath.Base(rel), ".stage-") {
				os.RemoveAll(stage)
			}
		}
	}()
	m := manifest{Version: 1, Account: account, ImportedAt: time.Now().UTC()}
	decoder := json.NewDecoder(io.LimitReader(reader, maxExport+1))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return report, errors.New("不是支持的 Claude 会话导出格式：需要 conversations.json 数组")
	}
	seen := map[string]bool{}
	for decoder.More() {
		var c conversation
		if err := decoder.Decode(&c); err != nil {
			return report, fmt.Errorf("会话 JSON 无法解析，整批未导入: %w", err)
		}
		if c.UUID == "" || c.Messages == nil {
			return report, errors.New("会话缺少 uuid 或 chat_messages，整批未导入")
		}
		id := "chat_" + digest(accountID+":"+c.UUID)
		if seen[id] {
			return report, errors.New("导出文件中存在重复的会话 UUID，整批未导入")
		}
		seen[id] = true
		e, skipped, err := writeConversation(filepath.Join(stage, id+".jsonl"), id, c)
		if err != nil {
			return report, err
		}
		m.Entries = append(m.Entries, e)
		m.SkippedBlocks += skipped
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
		return report, errors.New("导出文件未完整结束，整批未导入")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF || decoder.InputOffset() > maxExport {
		return report, errors.New("导出文件含多余内容或超过大小限制，整批未导入")
	}
	if len(m.Entries) == 0 {
		return report, errors.New("导出文件中没有会话")
	}
	current, err := f.Stat()
	if err != nil {
		return report, err
	}
	if current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
		return report, errors.New("导出文件在读取期间发生变化，整批未导入，请重试")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return report, err
	}
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), data, 0o600); err != nil {
		return report, err
	}
	if err := os.Rename(stage, batch); err != nil {
		return report, fmt.Errorf("保存导入批次失败，原数据未改动: %w", err)
	}
	stage = ""
	report.Conversations, report.SkippedBlocks = len(m.Entries), m.SkippedBlocks
	return report, nil
}

func writeConversation(path, id string, c conversation) (entry, int, error) {
	e := entry{ID: id, Title: strings.TrimSpace(c.Name), Created: c.Created, Updated: c.Updated}
	if e.Title == "" {
		e.Title = "未命名聊天"
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return e, 0, err
	}
	encoder := json.NewEncoder(f)
	if err := encoder.Encode(map[string]any{"type": "ai-title", "aiTitle": e.Title}); err != nil {
		f.Close()
		return e, 0, err
	}
	skipped := 0
	for index, msg := range *c.Messages {
		role := "assistant"
		if msg.Sender == "human" || msg.Sender == "user" {
			role = "user"
		} else if msg.Sender != "assistant" {
			f.Close()
			return e, skipped, fmt.Errorf("不支持的消息 sender %q，整批未导入", msg.Sender)
		}
		var blocks []map[string]string
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					blocks = append(blocks, map[string]string{"type": "text", "text": block.Text})
				}
			case "thinking":
				if block.Thinking != "" {
					blocks = append(blocks, map[string]string{"type": "thinking", "thinking": block.Thinking})
				}
			default:
				skipped++
			}
		}
		if len(blocks) == 0 && msg.Text != "" {
			blocks = append(blocks, map[string]string{"type": "text", "text": msg.Text})
		}
		if len(blocks) == 0 {
			continue
		}
		if e.Created.IsZero() || (!msg.Created.IsZero() && msg.Created.Before(e.Created)) {
			e.Created = msg.Created
		}
		if msg.Created.After(e.Updated) {
			e.Updated = msg.Created
		}
		if err := encoder.Encode(map[string]any{"type": role, "uuid": fmt.Sprintf("message-%d", index), "timestamp": msg.Created, "sessionId": id, "message": map[string]any{"role": role, "content": blocks}}); err != nil {
			f.Close()
			return e, skipped, err
		}
		e.Messages++
	}
	return e, skipped, f.Close()
}

var validID = regexp.MustCompile(`^chat_[a-f0-9]{64}$`)

// Account identities are case-insensitive; keep the first imported display
// label so later batches cannot split one account into multiple filter options.
func canonicalAccount(root, fallback string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "manifest.json"))
	if err != nil {
		return "", err
	}
	var first time.Time
	label := fallback
	for _, path := range paths {
		if strings.HasPrefix(filepath.Base(filepath.Dir(path)), ".stage-") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		var meta struct {
			Version    int
			Account    string
			ImportedAt time.Time
		}
		if json.Unmarshal(data, &meta) != nil || meta.Version != 1 || strings.TrimSpace(meta.Account) == "" {
			return "", fmt.Errorf("账号导入清单损坏: %s", path)
		}
		if first.IsZero() || meta.ImportedAt.Before(first) {
			first, label = meta.ImportedAt, meta.Account
		}
	}
	return label, nil
}

// List merges import batches by stable account+conversation identity. An older
// export cannot replace a newer conversation, even when imported afterwards.
func List(root string) ([]sessions.Session, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "*", "manifest.json"))
	if err != nil {
		return nil, err
	}
	type stored struct {
		session sessions.Session
		at      time.Time
	}
	byID := map[string]stored{}
	accountLabels := map[string]string{}
	for _, path := range paths {
		if strings.HasPrefix(filepath.Base(filepath.Dir(path)), ".stage-") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var m manifest
		if err := json.Unmarshal(data, &m); err != nil || m.Version != 1 {
			return nil, fmt.Errorf("导入清单损坏: %s", path)
		}
		accountRoot := filepath.Dir(filepath.Dir(path))
		label, exists := accountLabels[accountRoot]
		if !exists {
			label, err = canonicalAccount(accountRoot, m.Account)
			if err != nil {
				return nil, err
			}
			accountLabels[accountRoot] = label
		}
		for _, e := range m.Entries {
			if !validID.MatchString(e.ID) {
				return nil, errors.New("导入清单中含有无效会话 ID")
			}
			file := filepath.Join(filepath.Dir(path), e.ID+".jsonl")
			if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("导入的正文文件不可读: %s", file)
			}
			prior, exists := byID[e.ID]
			if exists && (prior.session.Last.After(e.Updated) || (prior.session.Last.Equal(e.Updated) && !m.ImportedAt.After(prior.at))) {
				continue
			}
			byID[e.ID] = stored{session: sessions.Session{Agent: "claude-desktop", ID: e.ID, Title: e.Title, Account: label, Imported: true, MessageCount: e.Messages, Start: e.Created, Last: e.Updated, ReadOnly: true, Transcript: true, Path: file, Models: []sessions.Model{}}, at: m.ImportedAt}
		}
	}
	out := make([]sessions.Session, 0, len(byID))
	for _, row := range byID {
		out = append(out, row.session)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Last.Equal(out[j].Last) {
			return out[i].Last.After(out[j].Last)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
