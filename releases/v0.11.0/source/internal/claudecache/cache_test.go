package claudecache

import (
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/klauspost/compress/snappy"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// enc 是测试用的最小 V8 序列化器：对象、数组、字符串、数字、布尔、null，
// 中文走双字节串，与 Chromium 写入的形态一致。
func enc(v any) []byte { return encode([]byte{0xff, 0x0f}, v) }

func uv(b []byte, n uint64) []byte { return binary.AppendUvarint(b, n) }

func encode(b []byte, v any) []byte {
	switch x := v.(type) {
	case nil:
		return append(b, '0')
	case bool:
		if x {
			return append(b, 'T')
		}
		return append(b, 'F')
	case int:
		return uv(append(b, 'I'), uint64(x<<1)^uint64(x>>63))
	case float64:
		return binary.LittleEndian.AppendUint64(append(b, 'N'), math.Float64bits(x))
	case string:
		latin := true
		for _, r := range x {
			if r > 0xff {
				latin = false
			}
		}
		if latin {
			raw := make([]byte, 0, len(x))
			for _, r := range x {
				raw = append(raw, byte(r))
			}
			return append(uv(append(b, '"'), uint64(len(raw))), raw...)
		}
		u := utf16.Encode([]rune(x))
		if len(b)%2 == 0 { // 对齐填充，Chromium 也这么写
			b = append(b, 0x00)
		}
		b = uv(append(b, 'c'), uint64(2*len(u)))
		for _, c := range u {
			b = binary.LittleEndian.AppendUint16(b, c)
		}
		return b
	case []any:
		b = uv(append(b, 'A'), uint64(len(x)))
		for _, e := range x {
			b = encode(b, e)
		}
		return uv(uv(append(b, '$'), 0), uint64(len(x)))
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b = append(b, 'o')
		for _, k := range keys {
			b = encode(encode(b, k), x[k])
		}
		return uv(append(b, '{'), uint64(len(keys)))
	}
	panic("unsupported")
}

// blink 加 Blink 信封（含尾部偏移），wrap 再套 snappy 包装。
func blink(v any) []byte {
	b := append([]byte{0xff, 0x15, 0xfe}, make([]byte, 12)...)
	return append(b, enc(v)...)
}

func wrap(v any) []byte {
	return append([]byte{0xff, 0x11, 0x02}, snappy.Encode(nil, blink(v))...)
}

const (
	acct = "11111111-2222-4333-8444-555555555555"
	c1   = "aaaaaaaa-1111-4222-8333-444444444444"
	c2   = "bbbbbbbb-1111-4222-8333-444444444444"
)

func conv(id string, written float64, text string) map[string]any {
	return map[string]any{
		"conversationUuid": id, "accountUuid": acct, "writtenAt": written,
		"tree": map[string]any{"kind": "hub_transcript", "messages": []any{
			map[string]any{"uuid": "m2", "index": 1, "sender": "assistant", "created_at": "2026-10-20T01:19:00Z", "content": []any{
				map[string]any{"type": "thinking", "thinking": "", "summaries": []any{map[string]any{"summary": "先读文档"}}},
				map[string]any{"type": "tool_use", "id": "t1", "name": "Projects", "input": map[string]any{"summary": "规范.md"}},
				map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": []any{map[string]any{"type": "text", "text": "规范正文"}}},
				map[string]any{"type": "text", "text": "已完成"},
			}},
			map[string]any{"uuid": "m1", "index": 0, "sender": "human", "created_at": "2026-10-20T01:18:51.655Z", "content": []any{
				map[string]any{"type": "text", "text": text},
			}, "attachedFolders": []any{map[string]any{"path": `D:\Work\Demo`}}},
		}},
	}
}

func listing() map[string]any {
	return map[string]any{"buster": "x", "clientState": map[string]any{"queries": []any{
		map[string]any{"queryKey": []any{"current_account", "org"}, "state": map[string]any{"data": map[string]any{"account": map[string]any{"uuid": acct, "display_name": "demo-user"}}}},
		map[string]any{"queryKey": []any{"chat_conversation_list", map[string]any{}, "infinite"}, "state": map[string]any{"data": map[string]any{"pages": []any{map[string]any{"data": []any{
			map[string]any{"uuid": c1, "name": "迁移方案评审", "model": "claude-sonnet-5-5", "created_at": "2026-10-20T01:18:00Z", "updated_at": "2026-10-21T02:10:00Z", "project": map[string]any{"name": "Demo"}},
		}}}}}},
	}}}
}

func masked(parts ...[]byte) uint32 {
	c := uint32(0)
	for _, p := range parts {
		c = crc32.Update(c, crc32.MakeTable(crc32.Castagnoli), p)
	}
	return (c>>15 | c<<17) + 0xa282ead8
}

// logFile 写一个只含一条 FULL 记录的 LevelDB 写前日志；值为 nil 记成删除。
func logFile(t *testing.T, path string, seq uint64, kv ...[]byte) {
	t.Helper()
	batch := binary.LittleEndian.AppendUint64(nil, seq)
	batch = binary.LittleEndian.AppendUint32(batch, uint32(len(kv)/2))
	for i := 0; i < len(kv); i += 2 {
		if kv[i+1] == nil {
			batch = append(uv(append(batch, 0), uint64(len(kv[i]))), kv[i]...)
			continue
		}
		batch = append(uv(append(batch, 1), uint64(len(kv[i]))), kv[i]...)
		batch = append(uv(batch, uint64(len(kv[i+1]))), kv[i+1]...)
	}
	rec := make([]byte, 7, 7+len(batch))
	binary.LittleEndian.PutUint16(rec[4:], uint16(len(batch)))
	rec[6] = 1
	binary.LittleEndian.PutUint32(rec, masked([]byte{1}, batch))
	if err := os.WriteFile(path, append(rec, batch...), 0o600); err != nil {
		t.Fatal(err)
	}
}

// tableFile 写一个 SSTable：一个 snappy 压缩的数据块、未压缩的索引块与 footer。
func tableFile(t *testing.T, path string, seq uint64, key, val []byte) {
	t.Helper()
	blockOf := func(k, v []byte) []byte {
		b := uv(uv(uv(nil, 0), uint64(len(k))), uint64(len(v)))
		b = append(append(b, k...), v...)
		b = binary.LittleEndian.AppendUint32(b, 0)
		return binary.LittleEndian.AppendUint32(b, 1)
	}
	ikey := binary.LittleEndian.AppendUint64(append([]byte(nil), key...), seq<<8|1)
	data := snappy.Encode(nil, blockOf(ikey, val))
	file := binary.LittleEndian.AppendUint32(append(append([]byte(nil), data...), 1), masked(data, []byte{1}))
	idxOff := len(file)
	idx := blockOf(ikey, uv(uv(nil, 0), uint64(len(data))))
	file = binary.LittleEndian.AppendUint32(append(append(file, idx...), 0), masked(idx, []byte{0}))
	footer := uv(uv(nil, 0), 0)
	footer = uv(uv(footer, uint64(idxOff)), uint64(len(idx)))
	footer = append(footer, make([]byte, 40-len(footer))...)
	footer = binary.LittleEndian.AppendUint64(footer, 0xdb4775248b80fb57)
	if err := os.WriteFile(path, append(file, footer...), 0o600); err != nil {
		t.Fatal(err)
	}
}

func desktop(t *testing.T) (root, ldb, blob string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "Claude-Data", "claude_pub")
	ldb = filepath.Join(root, "IndexedDB", origin+".leveldb")
	blob = filepath.Join(root, "IndexedDB", origin+".blob", "1", "00")
	for _, d := range []string{ldb, blob} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// 大会话在 blob 文件里、小会话与列表内联在 LevelDB 里，标题与账号名取自
// 查询缓存，同一会话多份时取写入较新的那份，已删除的键不出现；转出的副本
// 能按 Claude 文件读回。
func TestSyncReadsDesktopCache(t *testing.T) {
	root, ldb, blob := desktop(t)
	put(t, filepath.Join(blob, "84"), wrap(conv(c1, 2, "新版问题")))
	put(t, filepath.Join(blob, "3f"), wrap(conv(c1, 1, "旧版问题")))
	// LevelDB 的值前面有 varint 版本号
	small := append([]byte{0x01}, blink(conv(c2, 1, "周报怎么写？\n第二行"))...)
	gone := []byte("k-gone")
	logFile(t, filepath.Join(ldb, "000022.log"), 10, []byte("k-small"), small, []byte("k-list"), append([]byte{0x01}, wrap(listing())...), gone, nil)
	tableFile(t, filepath.Join(ldb, "000024.ldb"), 5, gone, append([]byte{0x01}, blink(conv("aaaaaaaa-0000-4000-8000-000000000009", 1, "已删除"))...))

	out := filepath.Join(t.TempDir(), "cache")
	list, err := Sync([]string{root, filepath.Join(t.TempDir(), "missing")}, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 sessions, got %+v", list)
	}
	byID := map[string]sessions.Session{}
	for _, s := range list {
		byID[s.ID] = s
	}
	a, b := byID[c1], byID[c2]
	if a.Title != "迁移方案评审" || a.Account != "demo-user" || a.Agent != "claude-desktop" || !a.Imported || !a.Cached || !a.ReadOnly || a.MessageCount != 2 || a.Cwd != `D:\Work\Demo` || a.Last.Format("2006-01-02T15:04") != "2026-10-21T02:10" {
		t.Fatalf("blob session: %+v", a)
	}
	if b.Title != "周报怎么写？" || b.Account != "demo-user" {
		t.Fatalf("inline session title from first prompt: %+v", b)
	}
	tr, err := sessions.TranscriptOf(a)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range tr.Parts {
		got = append(got, p.Role+"/"+p.Kind+"/"+strings.TrimSpace(p.Text))
	}
	joined := strings.Join(got, "|")
	for _, want := range []string{"user/text/新版问题", "assistant/thinking/先读文档", "assistant/tool_use/", "tool/tool_result/规范正文", "assistant/text/已完成"} {
		if !strings.Contains(joined, want) {
			t.Errorf("transcript missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "旧版问题") {
		t.Errorf("older cached copy won: %s", joined)
	}

	// 内容不变时不重写副本
	info, _ := os.Stat(a.Path)
	again, err := Sync([]string{root}, out)
	if err != nil || len(again) != 2 {
		t.Fatalf("resync: %v %v", again, err)
	}
	if info2, _ := os.Stat(a.Path); !info2.ModTime().Equal(info.ModTime()) {
		t.Errorf("unchanged copy rewritten")
	}
}

// 上游缓存只读：扫描前后文件逐字节不变，也没有新增文件。
func TestSyncLeavesUpstreamUntouched(t *testing.T) {
	root, ldb, blob := desktop(t)
	put(t, filepath.Join(blob, "84"), wrap(conv(c1, 2, "问题")))
	logFile(t, filepath.Join(ldb, "000022.log"), 1, []byte("k"), append([]byte{0x01}, wrap(listing())...))
	snapshot := func() map[string]string {
		m := map[string]string{}
		filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
			if !d.IsDir() {
				b, _ := os.ReadFile(p)
				m[p] = string(b)
			}
			return nil
		})
		return m
	}
	before := snapshot()
	if _, err := Sync([]string{root}, filepath.Join(t.TempDir(), "out")); err != nil {
		t.Fatal(err)
	}
	after := snapshot()
	if len(before) != len(after) {
		t.Fatalf("files changed: %d -> %d", len(before), len(after))
	}
	for p, b := range before {
		if after[p] != b {
			t.Errorf("%s changed", p)
		}
	}
}

// 损坏或不认识的值按「不是会话」跳过，不报错也不 panic；会话 ID 不是 UUID
// 的记录（会拼进路径）一律拒绝。
func TestSyncSkipsBadValues(t *testing.T) {
	root, ldb, blob := desktop(t)
	put(t, filepath.Join(blob, "01"), wrap(conv("../../escape", 1, "x")))
	put(t, filepath.Join(blob, "02"), []byte{0xff, 0x11, 0x02, 0x80})
	put(t, filepath.Join(blob, "03"), blink(conv(c1, 1, "好的"))[:40])
	logFile(t, filepath.Join(ldb, "000001.log"), 1, []byte("k"), []byte{0x01, 0xff, 0x0f, 'B', 0x04})
	out := filepath.Join(t.TempDir(), "out")
	list, err := Sync([]string{root}, out)
	if err != nil || len(list) != 0 {
		t.Fatalf("want nothing, got %v %v", list, err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("nothing to copy, yet %s was created", out)
	}
}

func TestDecodeValueKinds(t *testing.T) {
	v, err := decodeValue(blink(map[string]any{"a": []any{1, -2, 1.5, true, nil, "中文", "latin"}}))
	if err != nil {
		t.Fatal(err)
	}
	a := v.(map[string]any)["a"].([]any)
	if a[0] != 1.0 || a[1] != -2.0 || a[2] != 1.5 || a[3] != true || a[4] != nil || a[5] != "中文" || a[6] != "latin" {
		t.Fatalf("decoded %#v", a)
	}
	// 对象引用指回先前的对象：[{}, ^1]
	v, err = decodeValue([]byte{0xff, 0x0f, 'A', 2, 'o', '{', 0, '^', 1, '$', 0, 2})
	if err != nil {
		t.Fatal(err)
	}
	if arr := v.([]any); len(arr) != 2 || arr[1] == nil {
		t.Fatalf("ref %#v", v)
	}
}

// 有分支（重新生成过回答）时只取当前显示的那条：按会话列表给的叶子回溯。
// 没有附加文件夹的会话，从 Desktop 记下的授权文件夹里挑出工作目录。
func TestSyncFollowsBranchAndWorkspace(t *testing.T) {
	root, ldb, blob := desktop(t)
	msg := func(id, parent string, index int, sender, text string) map[string]any {
		return map[string]any{"uuid": id, "parent_message_uuid": parent, "index": index, "sender": sender, "created_at": "2026-10-20T01:00:00Z",
			"content": []any{map[string]any{"type": "text", "text": text}}}
	}
	root0 := "00000000-0000-4000-8000-000000000000"
	put(t, filepath.Join(blob, "01"), wrap(map[string]any{"conversationUuid": c1, "accountUuid": acct, "writtenAt": 1.0,
		"tree": map[string]any{"kind": "hub_transcript", "messages": []any{
			msg("q", root0, 0, "human", "问题"), msg("a1", "q", 1, "assistant", "旧回答"), msg("a2", "q", 1, "assistant", "新回答"),
		}}}))
	list := listing()
	rows := list["clientState"].(map[string]any)["queries"].([]any)[1].(map[string]any)["state"].(map[string]any)["data"].(map[string]any)["pages"].([]any)[0].(map[string]any)["data"].([]any)
	rows[0].(map[string]any)["current_leaf_message_uuid"] = "a2"
	rows[0].(map[string]any)["workspace_session_id"] = "cse_01ABC"
	logFile(t, filepath.Join(ldb, "000001.log"), 1, []byte("k"), append([]byte{0x01}, wrap(list)...))
	spaces := filepath.Join(root, "local-agent-mode-sessions", acct, "org", "remote-session-spaces.json")
	os.MkdirAll(filepath.Dir(spaces), 0o700)
	doc, _ := json.Marshal(map[string]any{"entries": []any{map[string]any{"sessionId": "session_01ABC", "folders": []string{`D:\Work\Demo\weekly`, `D:\Work\Demo`, "relative"}}}})
	put(t, spaces, doc)

	got, err := Sync([]string{root}, filepath.Join(t.TempDir(), "out"))
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if got[0].Cwd != `D:\Work\Demo` || got[0].MessageCount != 2 {
		t.Fatalf("session: %+v", got[0])
	}
	tr, _ := sessions.TranscriptOf(got[0])
	var text []string
	for _, p := range tr.Parts {
		text = append(text, p.Text)
	}
	if j := strings.Join(text, "|"); !strings.Contains(j, "新回答") || strings.Contains(j, "旧回答") {
		t.Fatalf("branch: %s", j)
	}
}

// 校验和不对的日志记录与表块整条丢弃，不会把半截数据当成会话。
func TestSyncRejectsBadChecksums(t *testing.T) {
	root, ldb, _ := desktop(t)
	logPath := filepath.Join(ldb, "000001.log")
	logFile(t, logPath, 1, []byte("k"), append([]byte{0x01}, blink(conv(c1, 1, "日志里的"))...))
	b, _ := os.ReadFile(logPath)
	b[len(b)-3] ^= 0xff
	put(t, logPath, b)
	tablePath := filepath.Join(ldb, "000002.ldb")
	tableFile(t, tablePath, 1, []byte("k2"), append([]byte{0x01}, blink(conv(c2, 1, "表里的"))...))
	b, _ = os.ReadFile(tablePath)
	b[2] ^= 0xff
	put(t, tablePath, b)
	if got, _ := Sync([]string{root}, filepath.Join(t.TempDir(), "out")); len(got) != 0 {
		t.Fatalf("corrupt records were read: %+v", got)
	}
}

func TestSyncNeedsAbsoluteOutput(t *testing.T) {
	if _, err := Sync(nil, filepath.Join("imports", "claude-desktop-cache")); err == nil {
		t.Fatal("relative output accepted")
	}
}
