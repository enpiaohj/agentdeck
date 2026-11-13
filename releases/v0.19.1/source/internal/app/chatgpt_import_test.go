package app

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const chatgptShard = `[
{"id":"c1","title":"整理发票","create_time":1760000000,"update_time":1760000100,"current_node":"a1","conversation_template_id":"g-p-1234567890abcdef","default_model_slug":"gpt-5",
 "mapping":{"r":{"id":"r","parent":null,"children":["u1"],"message":null},
  "u1":{"id":"u1","parent":"r","children":["a1"],"message":{"id":"u1","author":{"role":"user"},"create_time":1760000001,"content":{"content_type":"text","parts":["查找十一月的发票"]}}},
  "a1":{"id":"a1","parent":"u1","children":[],"message":{"id":"a1","author":{"role":"assistant"},"create_time":1760000002,"content":{"content_type":"text","parts":["发票在这里citeturn0search0。"]},"metadata":{"model_slug":"gpt-5"}}}}},
{"id":"c2","title":"闲聊","create_time":1750000000,"update_time":1750000100,"current_node":"a2","conversation_template_id":null,
 "mapping":{"r":{"id":"r","parent":null,"children":["u2"],"message":null},
  "u2":{"id":"u2","parent":"r","children":["a2"],"message":{"id":"u2","author":{"role":"user"},"create_time":1750000001,"content":{"content_type":"text","parts":["你好"]}}},
  "a2":{"id":"a2","parent":"u2","children":[],"message":{"id":"a2","author":{"role":"assistant"},"create_time":1750000002,"content":{"content_type":"text","parts":["你好！"]}}}}}
]`

func TestImportChatGPTEndToEnd(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	source := filepath.Join(t.TempDir(), "conversations.json")
	if err := os.WriteFile(source, []byte(chatgptShard), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := a.NewSessionService()

	pre, err := svc.PreviewChatGPT(source, "个人")
	if err != nil || pre.Conversations != 2 || len(pre.Projects) != 1 || pre.Projects[0].ID != "g-p-1234567890abcdef" ||
		pre.Projects[0].Samples[0] != "整理发票" || pre.Projects[0].Default != "ChatGPT 项目 123456" {
		t.Fatalf("preview: %+v %v", pre, err)
	}
	report, err := svc.ImportChatGPT(source, "个人", map[string]string{"g-p-1234567890abcdef": "财务资料"}, false, nil)
	if err != nil || report.Conversations != 2 || report.Projects != 1 {
		t.Fatalf("import: %+v %v", report, err)
	}

	rows := svc.List(Filter{Agent: "chatgpt"})
	if len(rows) != 2 || rows[0].Source != "import" || rows[0].Resume != "" || rows[0].Account != "个人" {
		t.Fatalf("list: %+v", rows)
	}
	var key string
	for _, r := range rows {
		if r.Title == "整理发票" {
			key = r.Key
			if filepath.Base(r.Cwd) != "财务资料" || r.Messages != 2 {
				t.Fatalf("项目会话: %+v", r)
			}
		}
	}
	if key == "" {
		t.Fatalf("没有找到项目会话: %+v", rows)
	}
	if got := svc.List(Filter{Project: filepath.Join("ChatGPT", "财务资料")}); len(got) != 1 {
		t.Fatalf("按项目筛选: %d", len(got))
	}
	if command, err := a.NewResumeService().Command(key); err != nil || command != "" {
		t.Fatalf("导入的 ChatGPT 会话不该有恢复命令: %q %v", command, err)
	}
	tr, err := svc.Transcript(key)
	if err != nil || len(tr.Parts) != 2 || !strings.Contains(tr.Parts[1].Text, "发票在这里。") || strings.Contains(tr.Parts[1].Text, "turn0") {
		t.Fatalf("transcript: %+v %v", tr, err)
	}
	if got, err := a.NewSearchService().Query("十一月", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("scan: %+v %v", got, err)
	}
	a.buildIndex(true)
	// 命中按段落计：用户与助手两段都含「发票」，但都属于这一个会话
	if got, err := a.NewSearchService().Query("发票", 20); err != nil || got.Mode != "index" || len(got.Hits) != 2 || got.Hits[0].Key != key || got.Hits[1].Key != key {
		t.Fatalf("index: %+v %v", got, err)
	}

	// 导出：文件名用会话开始日期与标题，front matter 带身份
	out := t.TempDir()
	p, err := svc.ExportMarkdown(key, out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(p), "2025-10-09-整理发票-gpt_") || !strings.HasSuffix(p, ".md") {
		t.Fatalf("导出文件名: %s", filepath.Base(p))
	}
	if body, _ := os.ReadFile(p); !strings.Contains(string(body), `agent: "chatgpt"`) || !strings.Contains(string(body), "发票在这里。") {
		t.Fatalf("导出内容:\n%s", body)
	}

	// 改项目名不需要重新导入
	if err := svc.SetChatGPTProjectNames("个人", map[string]string{"g-p-1234567890abcdef": "报销"}); err != nil {
		t.Fatal(err)
	}
	if got := svc.List(Filter{Project: filepath.Join("ChatGPT", "报销")}); len(got) != 1 {
		t.Fatalf("改名后: %d", len(got))
	}
	// 再导入同一份：不重复
	if again, err := svc.ImportChatGPT(source, "个人", nil, false, nil); err != nil || !again.AlreadyImported {
		t.Fatalf("重复导入: %+v %v", again, err)
	}
	if rows := svc.List(Filter{Agent: "chatgpt"}); len(rows) != 2 {
		t.Fatalf("重复导入后: %d", len(rows))
	}
}

// 导入图片后：阅读区的地址能被服务，导出 Markdown 时图片拷到 assets/ 并改成相对路径。
func TestChatGPTImagesImportAndExport(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89}
	shard := `[{"id":"ci","title":"带图的对话","create_time":1760000000,"update_time":1760000100,"current_node":"a","conversation_template_id":null,
	 "mapping":{"r":{"id":"r","parent":null,"children":["u"],"message":null},
	 "u":{"id":"u","parent":"r","children":["a"],"message":{"id":"u","author":{"role":"user"},"create_time":1760000001,"content":{"content_type":"multimodal_text","parts":["看这张图",{"content_type":"image_asset_pointer","asset_pointer":"sediment://file_img1"}]}}},
	 "a":{"id":"a","parent":"u","children":[],"message":{"id":"a","author":{"role":"assistant"},"create_time":1760000002,"content":{"content_type":"text","parts":["看到了"]}}}}}]`
	zp := filepath.Join(t.TempDir(), "export.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("conversations-000.json")
	w.Write([]byte(shard))
	w, _ = zw.Create("file_img1.dat")
	w.Write(png)
	zw.Close()
	f.Close()

	svc := a.NewSessionService()
	pre, err := svc.PreviewChatGPT(zp, "个人")
	if err != nil || pre.Images != 1 || pre.ImageBytes != int64(len(png)) {
		t.Fatalf("preview: %+v %v", pre, err)
	}
	rep, err := svc.ImportChatGPT(zp, "个人", nil, true, nil)
	if err != nil || rep.Images != 1 {
		t.Fatalf("import: %+v %v", rep, err)
	}
	rows := svc.List(Filter{Agent: "chatgpt"})
	if len(rows) != 1 {
		t.Fatalf("rows: %+v", rows)
	}
	tr, err := svc.Transcript(rows[0].Key)
	// 图片是独立的「图片」段落：全文索引不收它，地址不会出现在搜索结果里
	if err != nil || len(tr.Parts) != 3 || tr.Parts[1].Kind != "image" || !strings.HasPrefix(tr.Parts[1].Text, "![图片](/_chatgpt/") {
		t.Fatalf("阅读区应拿到可显示的图片地址: %+v %v", tr, err)
	}
	a.buildIndex(true)
	for _, q := range []string{"chatgpt", "图片"} {
		if got, err := a.NewSearchService().Query(q, 20); err != nil || len(got.Hits) != 0 {
			t.Fatalf("搜索 %q 不该命中图片地址: %+v %v", q, got, err)
		}
	}
	if got, err := a.NewSearchService().Query("看这张图", 20); err != nil || len(got.Hits) != 1 || strings.Contains(got.Hits[0].Snippet, "_chatgpt") {
		t.Fatalf("摘要里不该有图片地址: %+v %v", got, err)
	}

	// 阅读区用到的地址能被服务
	address := regexp.MustCompile(`\(/_chatgpt/[^)]+\)`).FindString(tr.Parts[1].Text)
	address = strings.Trim(address, "()")
	rec := httptest.NewRecorder()
	a.ChatGPTAssets()(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", address, nil))
	if rec.Code != 200 || rec.Body.Len() != len(png) {
		t.Fatalf("服务图片: %d %d", rec.Code, rec.Body.Len())
	}

	out := t.TempDir()
	p, err := svc.ExportMarkdown(rows[0].Key, out)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(p)
	m := regexp.MustCompile(`!\[图片\]\(assets/([0-9a-f]{64}\.png)\)`).FindStringSubmatch(string(body))
	if m == nil || strings.Contains(string(body), "/_chatgpt/") {
		t.Fatalf("导出应改成相对路径:\n%s", body)
	}
	if b, err := os.ReadFile(filepath.Join(out, "assets", m[1])); err != nil || string(b) != string(png) {
		t.Fatalf("assets 里的图片: %v", err)
	}
	// 再导出一次：同一个文件，图片不重复
	if p2, err := svc.ExportMarkdown(rows[0].Key, out); err != nil || p2 != p {
		t.Fatalf("重复导出: %q %q %v", p, p2, err)
	}
	if ents, _ := os.ReadDir(filepath.Join(out, "assets")); len(ents) != 1 {
		t.Fatalf("assets 里应只有 1 个文件: %d", len(ents))
	}
}

// 预览把本机 ChatGPT 桌面应用里的项目名当作命名建议（导出本身没有项目名）。
func TestPreviewChatGPTSuggestsNamesFromDesktopApp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	state := `{"electron-persisted-atom-state":{"chatgpt-sidebar-state-v2":{"acct":{"projects":[{"id":"g-p-1234567890abcdef","name":"桌面应用里的名字"}]}}}}`
	if err := os.WriteFile(filepath.Join(home, ".codex-global-state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	a, _, _, _ := newTestApp(t)
	source := filepath.Join(t.TempDir(), "conversations.json")
	if err := os.WriteFile(source, []byte(chatgptShard), 0o600); err != nil {
		t.Fatal(err)
	}
	pre, err := a.NewSessionService().PreviewChatGPT(source, "个人")
	if err != nil || len(pre.Projects) != 1 || pre.Projects[0].Suggested != "桌面应用里的名字" || pre.Projects[0].Name != "" {
		t.Fatalf("%+v %v", pre, err)
	}
	// 状态文件还是原样
	if b, _ := os.ReadFile(filepath.Join(home, ".codex-global-state.json")); string(b) != state {
		t.Fatal("状态文件被改动")
	}
}
