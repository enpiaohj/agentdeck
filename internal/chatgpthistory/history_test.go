package chatgpthistory

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// ---- synthetic export fixtures (no real conversation content) ----

type tnode = map[string]any

func msgNode(id string, parent any, children []string, role, ctype string, parts []any, at float64, meta map[string]any) tnode {
	n := tnode{"id": id, "parent": parent, "children": children}
	if role != "" {
		content := tnode{"content_type": ctype}
		if parts != nil {
			content["parts"] = parts
		}
		m := tnode{"id": id, "author": tnode{"role": role}, "create_time": at, "content": content}
		if meta != nil {
			m["metadata"] = meta
		}
		n["message"] = m
	} else {
		n["message"] = nil
	}
	return n
}

// chat has a root, a system prompt, a user turn, two assistant answers (a
// regenerated branch: only the one current_node points at counts), and a
// follow-up. project is the conversation_template_id.
func chat(id, title, project string, created float64) tnode {
	var tpl any
	if project != "" {
		tpl = project
	}
	return tnode{
		"id": id, "conversation_id": id, "title": title, "create_time": created, "update_time": created + 100,
		"current_node": "a2", "conversation_template_id": tpl, "default_model_slug": "gpt-5",
		"mapping": tnode{
			"root": msgNode("root", nil, []string{"sys"}, "", "", nil, 0, nil),
			"sys":  msgNode("sys", "root", []string{"u1"}, "system", "text", []any{""}, created, nil),
			"u1":   msgNode("u1", "sys", []string{"a1", "a1b"}, "user", "text", []any{"你好，帮我看看"}, created+1, nil),
			"a1":   msgNode("a1", "u1", nil, "assistant", "text", []any{"这是被放弃的分支"}, created+2, nil),
			"a1b": msgNode("a1b", "u1", []string{"u2"}, "assistant", "text",
				[]any{"答案\ue200cite\ue202turn0search1\ue201在这里，见\ue200entity\ue202[\"city\",\"上海\",\"a city\"]\ue201。"}, created+3, tnode{"model_slug": "gpt-5"}),
			"u2": msgNode("u2", "a1b", []string{"th", "hid", "a2"}, "user", "multimodal_text",
				[]any{"看这张图", tnode{"content_type": "image_asset_pointer", "asset_pointer": "sediment://file_x"}, tnode{"content_type": "audio_transcription", "text": "语音转写"}}, created+4, nil),
			"th":  tnode{"id": "th", "parent": "u2", "children": nil},
			"hid": msgNode("hid", "u2", nil, "assistant", "text", []any{"隐藏消息"}, created+5, tnode{"is_visually_hidden_from_conversation": true}),
			"a2": tnode{"id": "a2", "parent": "u2", "children": nil, "message": tnode{
				"id": "a2", "author": tnode{"role": "assistant"}, "create_time": created + 6,
				"content": tnode{"content_type": "thoughts", "thoughts": []any{tnode{"summary": "想一想", "content": "先看图"}}},
			}},
		},
	}
}

func writeExport(t *testing.T, dir string, shards map[string][]tnode) string {
	t.Helper()
	zp := filepath.Join(dir, "export.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, convs := range shards {
		w, _ := zw.Create(name)
		if err := json.NewEncoder(w).Encode(convs); err != nil {
			t.Fatal(err)
		}
	}
	w, _ := zw.Create("file_0000attachment.dat") // an attachment: never read
	w.Write([]byte("not json at all"))
	w, _ = zw.Create("user.json")
	w.Write([]byte(`{"email":"x"}`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return zp
}

func fixtureShards() map[string][]tnode {
	return map[string][]tnode{
		"conversations-000.json": {chat("c1", "项目里的对话", "g-p-aaaaaaaa11112222", 1_760_000_000), chat("c2", "普通对话", "", 1_760_100_000)},
		"conversations-001.json": {chat("c3", "另一个项目", "g-p-bbbbbbbb33334444", 1_760_200_000), chat("c4", "再一个同项目", "g-p-aaaaaaaa11112222", 1_760_300_000)},
	}
}

func TestImportListAndTranscript(t *testing.T) {
	dir := t.TempDir()
	zp := writeExport(t, dir, fixtureShards())
	before, _ := os.ReadFile(zp)
	root := filepath.Join(dir, "store")
	rep, err := Import(root, zp, "个人账号", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Conversations != 4 || rep.Projects != 2 || rep.AlreadyImported || rep.Account != "个人账号" {
		t.Fatalf("report = %+v", rep)
	}
	if rep.SkippedBlocks == 0 {
		t.Error("图片与音频指针应计入跳过项")
	}
	if after, _ := os.ReadFile(zp); string(after) != string(before) {
		t.Fatal("导出文件被改动")
	}

	list, err := List(root, nil)
	if err != nil || len(list) != 4 {
		t.Fatalf("list = %d %v", len(list), err)
	}
	if list[0].Title != "再一个同项目" {
		t.Fatalf("应按最近活动排序: %q", list[0].Title)
	}
	byTitle := map[string]sessions.Session{}
	for _, s := range list {
		byTitle[s.Title] = s
		if s.Agent != Agent || !s.Imported || !s.ReadOnly || !s.Transcript || s.Account != "个人账号" {
			t.Fatalf("会话属性不对: %+v", s)
		}
	}
	if got := byTitle["普通对话"].Cwd; got != "" {
		t.Errorf("不在项目里的会话不该有 Cwd: %q", got)
	}
	want := filepath.Join("ChatGPT", "ChatGPT 项目 aaaaaa")
	if got := byTitle["项目里的对话"].Cwd; got != want || byTitle["再一个同项目"].Cwd != want {
		t.Errorf("同一项目应归到同一个 Cwd: %q %q want %q", got, byTitle["再一个同项目"].Cwd, want)
	}
	if byTitle["项目里的对话"].MessageCount != 4 {
		t.Errorf("消息数 = %d，应为 4（用户、助手、用户、思考；系统与隐藏消息、被放弃的分支不算）", byTitle["项目里的对话"].MessageCount)
	}

	// the stored transcript reads back through the shared transcript reader
	tr, err := sessions.TranscriptOf(byTitle["项目里的对话"])
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, p := range tr.Parts {
		all.WriteString(p.Kind + ":" + p.Text + "\n")
	}
	text := all.String()
	for _, want := range []string{"text:你好，帮我看看", "答案在这里，见上海。", "text:看这张图", "*[图片]*", "语音转写", "thinking:**想一想**\n\n先看图"} {
		if !strings.Contains(text, want) {
			t.Errorf("正文缺少 %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"被放弃的分支", "隐藏消息", "\ue200", "turn0search1"} {
		if strings.Contains(text, bad) {
			t.Errorf("正文不该含 %q:\n%s", bad, text)
		}
	}
}

// The same export imported twice changes nothing; names given later rename the
// project without re-importing.
func TestImportIdempotentAndNames(t *testing.T) {
	dir := t.TempDir()
	zp := writeExport(t, dir, fixtureShards())
	root := filepath.Join(dir, "store")
	if _, err := Import(root, zp, "A", Options{}); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(root, zp, "A", Options{Names: map[string]string{"g-p-aaaaaaaa11112222": `工作/资料: 汇总?`, "g-p-bbbbbbbb33334444": "  ", "not-a-project": "x"}})
	if err != nil || !rep.AlreadyImported || rep.Conversations != 4 {
		t.Fatalf("再次导入 = %+v %v", rep, err)
	}
	list, _ := List(root, nil)
	cwds := map[string]bool{}
	for _, s := range list {
		cwds[s.Cwd] = true
	}
	if !cwds[filepath.Join("ChatGPT", "工作 资料 汇总")] || !cwds[filepath.Join("ChatGPT", "ChatGPT 项目 bbbbbb")] || len(cwds) != 3 {
		t.Fatalf("项目名没有生效: %v", cwds)
	}
	if err := SetProjectNames(root, "a", map[string]string{"g-p-aaaaaaaa11112222": "改名后"}); err != nil {
		t.Fatal(err)
	}
	list, _ = List(root, nil)
	found := false
	for _, s := range list {
		found = found || s.Cwd == filepath.Join("ChatGPT", "改名后")
	}
	if !found {
		t.Fatal("SetProjectNames 没有生效（账号名大小写不敏感）")
	}
	if ents, _ := filepath.Glob(filepath.Join(root, "*", ".stage-*")); len(ents) != 0 {
		t.Fatalf("残留暂存目录: %v", ents)
	}
}

// A second export with an updated conversation replaces the older copy; an
// older export imported afterwards cannot.
func TestImportNewerWins(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	old := chat("c1", "旧标题", "", 1_760_000_000)
	newer := chat("c1", "新标题", "", 1_760_000_000)
	newer["update_time"] = float64(1_770_000_000)
	zNew := writeExport(t, filepath.Join(mk(t, dir, "n")), map[string][]tnode{"conversations-000.json": {newer}})
	zOld := writeExport(t, filepath.Join(mk(t, dir, "o")), map[string][]tnode{"conversations-000.json": {old}})
	for _, z := range []string{zNew, zOld} {
		if _, err := Import(root, z, "A", Options{}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := List(root, nil)
	if len(list) != 1 || list[0].Title != "新标题" {
		t.Fatalf("list = %+v", list)
	}
}

func mk(t *testing.T, base, name string) string {
	t.Helper()
	p := filepath.Join(base, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	good := writeExport(t, dir, fixtureShards())
	if _, err := Import(root, good, " ", Options{}); err == nil {
		t.Error("空账号名被接受")
	}
	if _, err := Import(root, filepath.Join(dir, "missing.zip"), "A", Options{}); err == nil {
		t.Error("不存在的文件被接受")
	}
	// a ZIP that is not a ChatGPT export
	other := filepath.Join(dir, "other.zip")
	f, _ := os.Create(other)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("readme.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	if _, err := Import(root, other, "A", Options{}); err == nil || !strings.Contains(err.Error(), "conversations") {
		t.Errorf("无会话分片的 ZIP: %v", err)
	}
	// a corrupt shard imports nothing at all
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`[{"id":"x","mapping":{}`), 0o644)
	if _, err := Import(root, bad, "A", Options{}); err == nil {
		t.Error("损坏的 JSON 被接受")
	}
	if list, _ := List(root, nil); len(list) != 0 {
		t.Fatalf("失败的导入不该留下数据: %d", len(list))
	}
	if ents, _ := filepath.Glob(filepath.Join(root, "*", ".stage-*")); len(ents) != 0 {
		t.Fatalf("残留暂存目录: %v", ents)
	}
}

func TestPreview(t *testing.T) {
	dir := t.TempDir()
	zp := writeExport(t, dir, fixtureShards())
	root := filepath.Join(dir, "store")
	res, err := Preview(root, zp, "A")
	if err != nil {
		t.Fatal(err)
	}
	if res.Conversations != 4 || len(res.Projects) != 2 {
		t.Fatalf("preview = %+v", res)
	}
	if p := res.Projects[0]; p.ID != "g-p-aaaaaaaa11112222" || p.Conversations != 2 || p.Name != "" || p.Default != "ChatGPT 项目 aaaaaa" ||
		len(p.Samples) != 2 || p.Samples[0] != "再一个同项目" {
		t.Fatalf("project[0] = %+v", p)
	}
	if _, err := Import(root, zp, "A", Options{Names: map[string]string{"g-p-aaaaaaaa11112222": "已命名"}}); err != nil {
		t.Fatal(err)
	}
	res, _ = Preview(root, zp, "A")
	if res.Projects[0].Name != "已命名" {
		t.Fatalf("已起的名字应回显: %+v", res.Projects[0])
	}
	if ents, _ := os.ReadDir(filepath.Join(dir)); len(ents) != 2 { // export.zip + store
		t.Fatalf("预览不该写任何东西: %v", ents)
	}
}

func TestCleanText(t *testing.T) {
	for in, want := range map[string]string{
		"a\ue200cite\ue202turn0search0\ue201b":           "ab",
		"x\ue200entity\ue202[\"t\",\"名字\",\"d\"]\ue201y": "x名字y",
		"\ue203引用\ue204 与 \ue206":                        "引用 与 ",
		"没有标记":                                           "没有标记",
		"\ue200entity\ue202not json\ue201":               "",
	} {
		if got := cleanText(in); got != want {
			t.Errorf("cleanText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestActivePathWithoutCurrentNode(t *testing.T) {
	c := conversation{Mapping: map[string]node{
		"r": {ID: "r", Children: []string{"a"}},
		"a": {ID: "a", Parent: ptr("r"), Children: []string{"b", "c"}},
		"b": {ID: "b", Parent: ptr("a")},
		"c": {ID: "c", Parent: ptr("a")},
	}}
	got := activePath(c)
	var ids []string
	for _, n := range got {
		ids = append(ids, n.ID)
	}
	if strings.Join(ids, ",") != "r,a,c" {
		t.Fatalf("path = %v（没有 current_node 时取最后一个孩子）", ids)
	}
}

func ptr(s string) *string { return &s }
