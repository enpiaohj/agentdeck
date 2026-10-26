// Package claudecache 读取 Claude Desktop 本机缓存的 claude.ai 聊天。
//
// Desktop 3.x 的普通聊天与项目会话正文在服务器上，本机不落 jsonl；但内嵌的
// claude.ai 页面把打开过的会话整棵消息树（hub_transcript）连同会话列表缓存在
// 自己的 IndexedDB 里（<userData>\IndexedDB\https_claude.ai_0.indexeddb.*）。
// 这里只读这些文件，转成 Claude 风格的 jsonl 副本写进 AgentDeck 自有目录，
// 再作为只读会话列出。上游文件一律不写；只缓存过的会话才有正文，没打开过
// 的会话本机没有内容，读不到。
package claudecache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const origin = "https_claude.ai_0.indexeddb"

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Conversation 是缓存里读出的一个会话。
type Conversation struct {
	Account, AccountName, UUID, Title, Project, Model, Cwd string
	Created, Updated                                       time.Time
	messages                                               []any
}

type meta struct {
	title, project, model, leaf, workspace string
	created, updated                       time.Time
}

// Scan 读取一个 Desktop userData 目录里缓存的会话；没有缓存目录时返回空。
func Scan(dataDir string) ([]Conversation, error) {
	base := filepath.Join(dataDir, "IndexedDB")
	var errs []error
	metas := map[string]meta{}
	names := map[string]string{}
	byID := map[string]Conversation{}
	written := map[string]float64{}
	// 每个值解码后立即归类，不把整库的原始字节攒在内存里
	take := func(raw []byte) {
		v, ok := decodeStored(raw)
		if !ok {
			return
		}
		o, _ := v.(map[string]any)
		if o == nil {
			return
		}
		collectMeta(o, metas, names)
		c, at, ok := transcript(o)
		if !ok {
			return
		}
		k := c.Account + "/" + c.UUID
		if prior, exists := written[k]; exists && prior >= at {
			return
		}
		byID[k], written[k] = c, at
	}
	if kv, err := scanLevelDB(filepath.Join(base, origin+".leveldb")); err == nil {
		for _, v := range kv {
			take(v)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	// 大的值不在 LevelDB 里，而是单独的 blob 文件
	blobs := filepath.Join(base, origin+".blob")
	walkErr := filepath.WalkDir(blobs, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if p == blobs && errors.Is(err, os.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		if d.Type().IsRegular() {
			if b, err := readSmall(p); err == nil {
				take(b)
			}
		}
		return nil
	})
	if walkErr != nil {
		errs = append(errs, walkErr)
	}

	spaces := workspaceFolders(dataDir)
	out := make([]Conversation, 0, len(byID))
	for _, c := range byID {
		m, ok := metas[c.UUID]
		if ok {
			c.Title, c.Project, c.Model = m.title, m.project, m.model
			if !m.created.IsZero() {
				c.Created = m.created
			}
			if m.updated.After(c.Updated) {
				c.Updated = m.updated
			}
		}
		c.messages = branch(c.messages, m.leaf)
		c.Cwd = attachedFolder(c.messages)
		if c.Cwd == "" && m.workspace != "" {
			c.Cwd = mainFolder(spaces[workspaceKey(m.workspace)])
		}
		for _, msg := range c.messages {
			at := stamp(field(msg, "created_at"))
			if !at.IsZero() && (c.Created.IsZero() || at.Before(c.Created)) {
				c.Created = at
			}
			if at.After(c.Updated) {
				c.Updated = at
			}
		}
		c.AccountName = names[c.Account]
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

// branch 取当前显示的那条分支：从叶子沿 parent_message_uuid 回溯到根。
// 叶子未知或链断了（缓存只存了一部分）就退回按 index 排序的全部消息。
func branch(msgs []any, leaf string) []any {
	byID := make(map[string]any, len(msgs))
	for _, m := range msgs {
		if id := str(field(m, "uuid")); id != "" {
			byID[id] = m
		}
	}
	sorted := append([]any(nil), msgs...)
	sort.SliceStable(sorted, func(i, j int) bool { return num(field(sorted[i], "index")) < num(field(sorted[j], "index")) })
	if _, ok := byID[leaf]; !ok {
		if len(sorted) == 0 {
			return sorted
		}
		leaf = str(field(sorted[len(sorted)-1], "uuid"))
	}
	var chain []any
	seen := map[string]bool{}
	for id := leaf; id != "" && !seen[id]; {
		m, ok := byID[id]
		if !ok {
			break
		}
		seen[id] = true
		chain = append(chain, m)
		id = str(field(m, "parent_message_uuid"))
	}
	// 回溯必须走到根（父节点不在树里）；短于最长 index 说明链不完整
	if len(chain) == 0 || len(chain) < int(num(field(chain[0], "index")))+1 {
		return sorted
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func attachedFolder(msgs []any) string {
	for _, m := range msgs {
		folders, _ := field(m, "attachedFolders").([]any)
		var paths []string
		for _, f := range folders {
			paths = append(paths, str(field(f, "path")))
		}
		if p := mainFolder(paths); p != "" {
			return p
		}
	}
	return ""
}

// mainFolder 在一组授权文件夹里挑会话的工作目录：优先是其他文件夹祖先的那个，
// 否则取第一个。只接受绝对路径。
func mainFolder(paths []string) string {
	var abs []string
	for _, p := range paths {
		if filepath.IsAbs(p) {
			abs = append(abs, filepath.Clean(p))
		}
	}
	best, cover := "", -1
	for _, p := range abs {
		n := 0
		for _, q := range abs {
			if q != p && strings.HasPrefix(strings.ToLower(q), strings.ToLower(p)+string(filepath.Separator)) {
				n++
			}
		}
		if n > cover {
			best, cover = p, n
		}
	}
	return best
}

// workspaceKey 把 cse_X 与 session_X 归成同一个键 X：会话列表给的是前者，
// Desktop 的 remote-session-spaces.json 记的是后者。
func workspaceKey(id string) string {
	_, rest, ok := strings.Cut(id, "_")
	if !ok {
		return ""
	}
	return rest
}

// workspaceFolders 读 Desktop 为云端会话记下的授权文件夹，按会话归并。
func workspaceFolders(dataDir string) map[string][]string {
	out := map[string][]string{}
	paths, _ := filepath.Glob(filepath.Join(dataDir, "local-agent-mode-sessions", "*", "*", "remote-session-spaces.json"))
	for _, p := range paths {
		data, err := readSmall(p)
		if err != nil {
			continue
		}
		var doc struct {
			Entries []struct {
				SessionID string   `json:"sessionId"`
				Folders   []string `json:"folders"`
			} `json:"entries"`
		}
		if json.Unmarshal(data, &doc) != nil {
			continue
		}
		for _, e := range doc.Entries {
			if k := workspaceKey(e.SessionID); k != "" {
				out[k] = append(out[k], e.Folders...)
			}
		}
	}
	return out
}

// decodeStored 解出 LevelDB 值或 blob 文件里的 JS 值。LevelDB 里的值前面
// 还有一个 varint 版本号，所以在开头几个字节里找 Blink 信封。
func decodeStored(raw []byte) (any, bool) {
	for j := 0; j < len(raw) && j < 12; j++ {
		if raw[j] != 0xff {
			continue
		}
		if v, err := decodeValue(raw[j:]); err == nil {
			return v, true
		}
	}
	return nil, false
}

// collectMeta 从持久化的查询缓存里取会话列表（标题、项目、模型、时间）与
// 账号显示名。
func collectMeta(o map[string]any, metas map[string]meta, names map[string]string) {
	state, _ := o["clientState"].(map[string]any)
	queries, _ := state["queries"].([]any)
	for _, q := range queries {
		qm, _ := q.(map[string]any)
		qk, _ := qm["queryKey"].([]any)
		st, _ := qm["state"].(map[string]any)
		if len(qk) == 0 || st == nil {
			continue
		}
		data := st["data"]
		switch qk[0] {
		case "chat_conversation_list":
			dm, _ := data.(map[string]any)
			pages, _ := dm["pages"].([]any)
			if pages == nil && dm != nil {
				pages = []any{dm}
			}
			for _, p := range pages {
				pm, _ := p.(map[string]any)
				rows, _ := pm["data"].([]any)
				for _, r := range rows {
					rm, _ := r.(map[string]any)
					id := str(rm["uuid"])
					if !uuidRE.MatchString(id) {
						continue
					}
					m := meta{title: strings.TrimSpace(str(rm["name"])), model: str(rm["model"]), leaf: str(rm["current_leaf_message_uuid"]),
						workspace: str(rm["workspace_session_id"]), created: stamp(rm["created_at"]), updated: stamp(rm["updated_at"])}
					if pj, ok := rm["project"].(map[string]any); ok {
						m.project = str(pj["name"])
					}
					if prior, ok := metas[id]; !ok || !prior.updated.After(m.updated) {
						metas[id] = m
					}
				}
			}
		case "current_account":
			dm, _ := data.(map[string]any)
			acct, _ := dm["account"].(map[string]any)
			id := str(acct["uuid"])
			if uuidRE.MatchString(id) {
				name := strings.TrimSpace(str(acct["display_name"]))
				if name == "" {
					name = strings.TrimSpace(str(acct["full_name"]))
				}
				if name != "" {
					names[id] = name
				}
			}
		}
	}
}

// transcript 认出一条缓存的会话消息树；返回写入时间用于多份取新。
func transcript(o map[string]any) (Conversation, float64, bool) {
	tree, _ := o["tree"].(map[string]any)
	if tree == nil || str(tree["kind"]) != "hub_transcript" {
		return Conversation{}, 0, false
	}
	msgs, _ := tree["messages"].([]any)
	c := Conversation{Account: str(o["accountUuid"]), UUID: str(o["conversationUuid"]), messages: msgs}
	if !uuidRE.MatchString(c.Account) || !uuidRE.MatchString(c.UUID) || len(msgs) == 0 {
		return Conversation{}, 0, false
	}
	if t := millis(o["conversationUpdatedAt"]); t.After(c.Updated) {
		c.Updated = t
	}
	return c, num(o["writtenAt"]), true
}

// Sync 扫描各 Desktop 目录的缓存，把会话写成 out 下的 jsonl 副本（内容没变
// 就不重写），返回对应的只读会话。
func Sync(dataDirs []string, out string) ([]sessions.Session, error) {
	if !filepath.IsAbs(out) {
		return nil, fmt.Errorf("缓存副本目录必须是绝对路径: %q", out)
	}
	var list []sessions.Session
	var errs []error
	seen := map[string]bool{}
	for _, dir := range dataDirs {
		convs, err := Scan(dir)
		if err != nil {
			errs = append(errs, err)
		}
		for _, c := range convs {
			if seen[c.UUID] {
				continue
			}
			seen[c.UUID] = true
			s, err := write(out, c)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			list = append(list, s)
		}
	}
	return list, errors.Join(errs...)
}

func write(root string, c Conversation) (sessions.Session, error) {
	title := c.Title
	if title == "" {
		title = firstPrompt(c.messages)
	}
	if title == "" {
		title = "未命名聊天"
	}
	account := c.AccountName
	if account == "" {
		account = "Claude " + c.Account[:8]
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"type": "ai-title", "aiTitle": title})
	count := 0
	for _, m := range c.messages {
		if lines := convert(c, m); len(lines) > 0 {
			count++
			for _, l := range lines {
				if err := enc.Encode(l); err != nil {
					return sessions.Session{}, err
				}
			}
		}
	}
	dir := filepath.Join(root, c.Account)
	path := filepath.Join(dir, c.UUID+".jsonl")
	if old, err := os.ReadFile(path); err != nil || !bytes.Equal(old, buf.Bytes()) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return sessions.Session{}, err
		}
		tmp, err := os.CreateTemp(dir, ".tmp-*.jsonl")
		if err != nil {
			return sessions.Session{}, err
		}
		_, werr := tmp.Write(buf.Bytes())
		cerr := tmp.Close()
		if werr == nil && cerr == nil {
			werr = os.Rename(tmp.Name(), path)
		}
		if werr != nil || cerr != nil {
			os.Remove(tmp.Name())
			return sessions.Session{}, fmt.Errorf("写入缓存副本失败: %w", errors.Join(werr, cerr))
		}
	}
	models := []sessions.Model{}
	if c.Model != "" {
		models = append(models, sessions.Model{Model: c.Model})
	}
	return sessions.Session{Agent: "claude-desktop", ID: c.UUID, Title: title, Account: account, Cwd: c.Cwd,
		Imported: true, Cached: true, ReadOnly: true, Transcript: true, MessageCount: count,
		Start: c.Created, Last: c.Updated, Path: path, Models: models}, nil
}

// convert 把一条缓存消息转成 Claude Code 风格的行：助手消息里的
// tool_result 拆成紧随其后的 user 行，与 Claude Code 自己的文件一致。
func convert(c Conversation, m any) []map[string]any {
	role := "assistant"
	switch str(field(m, "sender")) {
	case "human", "user":
		role = "user"
	case "assistant":
	default:
		return nil
	}
	id := str(field(m, "uuid"))
	at := str(field(m, "created_at"))
	var lines []map[string]any
	var blocks []map[string]any
	blockRole := role
	flush := func() {
		if len(blocks) == 0 {
			return
		}
		msg := map[string]any{"role": blockRole, "content": blocks}
		if blockRole == "assistant" && c.Model != "" {
			msg["model"] = c.Model
		}
		lines = append(lines, map[string]any{"type": blockRole, "uuid": fmt.Sprintf("%s-%d", id, len(lines)), "timestamp": at, "sessionId": c.UUID, "cwd": c.Cwd, "message": msg})
		blocks = nil
	}
	add := func(r string, b map[string]any) {
		if r != blockRole {
			flush()
			blockRole = r
		}
		blocks = append(blocks, b)
	}
	content, _ := field(m, "content").([]any)
	for _, b := range content {
		switch str(field(b, "type")) {
		case "text":
			if t := str(field(b, "text")); t != "" {
				add(role, map[string]any{"type": "text", "text": t})
			}
		case "thinking":
			t := str(field(b, "thinking"))
			if t == "" {
				var parts []string
				sums, _ := field(b, "summaries").([]any)
				for _, s := range sums {
					if v := strings.TrimSpace(str(field(s, "summary"))); v != "" {
						parts = append(parts, v)
					}
				}
				t = strings.Join(parts, "\n")
			}
			if t != "" {
				add(role, map[string]any{"type": "thinking", "thinking": t})
			}
		case "tool_use":
			input := field(b, "input")
			if input == nil {
				input = map[string]any{}
			}
			add(role, map[string]any{"type": "tool_use", "id": str(field(b, "id")), "name": str(field(b, "name")), "input": input})
		case "tool_result":
			var parts []string
			items, _ := field(b, "content").([]any)
			for _, it := range items {
				if t := str(field(it, "text")); t != "" {
					parts = append(parts, t)
				}
			}
			add("user", map[string]any{"type": "tool_result", "tool_use_id": str(field(b, "tool_use_id")), "content": strings.Join(parts, "\n")})
		}
	}
	flush()
	return lines
}

func firstPrompt(msgs []any) string {
	for _, m := range msgs {
		if s := str(field(m, "sender")); s != "human" && s != "user" {
			continue
		}
		content, _ := field(m, "content").([]any)
		for _, b := range content {
			if str(field(b, "type")) != "text" {
				continue
			}
			line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(str(field(b, "text"))), "\n", 2)[0])
			if r := []rune(line); len(r) > 60 {
				line = string(r[:60]) + "…"
			}
			if line != "" {
				return line
			}
		}
	}
	return ""
}

func field(v any, k string) any {
	m, _ := v.(map[string]any)
	return m[k]
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func stamp(v any) time.Time {
	t, err := time.Parse(time.RFC3339Nano, str(v))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func millis(v any) time.Time {
	f := num(v)
	if f <= 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return time.Time{}
	}
	return time.UnixMilli(int64(f)).UTC()
}
