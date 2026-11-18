package sessions

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func exportFixture() (Session, Transcript) {
	start := time.Date(2026, 10, 8, 9, 0, 0, 0, time.Local)
	s := Session{Agent: "claude", ID: "sess1", Cwd: "/work/app", Title: `Fix: the "login" bug`,
		Start: start, Last: start.Add(time.Hour), Models: []Model{{Model: "claude-opus-5-5"}}}
	t := Transcript{Parts: []Part{
		{Role: "user", Kind: "context", Text: "<command-name>/init</command-name>"},
		{Role: "user", Kind: "text", Text: "Fix the login bug\nsecond line"},
		{Role: "assistant", Kind: "thinking", Text: "plan"},
		{Role: "assistant", Kind: "text", Text: "Reading the file."},
		{Role: "assistant", Kind: "tool_use", Name: "Read", Text: "{\n  \"file_path\": \"/a/login.go\"\n}"},
		{Role: "tool", Kind: "tool_result", Name: "Read", Text: "package login"},
		{Role: "user", Kind: "text", Text: "Thanks, now add a test"},
		{Role: "assistant", Kind: "text", Text: "Done."},
		{Role: "user", Kind: "text", Text: "ship it"},
	}}
	return s, t
}

func TestWriteExportMarkdown(t *testing.T) {
	s, tr := exportFixture()
	var b bytes.Buffer
	info := ExportInfo{Tags: []string{"bug", "登录"}, Note: "line1\nline2", Favorite: true, ExportedAt: time.Date(2026, 11, 12, 8, 0, 0, 0, time.Local)}
	if err := WriteExportMarkdown(&b, s, "claude", tr, info); err != nil {
		t.Fatal(err)
	}
	md := b.String()
	at := 0
	in := func(parts ...string) {
		t.Helper()
		for _, p := range parts {
			i := strings.Index(md[at:], p)
			if i < 0 {
				t.Fatalf("没有按顺序找到 %q:\n%s", p, md)
			}
			at += i + len(p)
		}
	}
	in("---\n",
		`title: "Fix: the \"login\" bug"`+"\n",
		"session_id: \"sess1\"\n",
		`agent: "claude"`+"\n",
		`project: "/work/app"`+"\n",
		"started: 2026-10-08T09:00:00",
		`models: ["claude-opus-5-5"]`+"\n",
		`tags: ["bug", "登录"]`+"\n",
		"favorite: true\n",
		`note: "line1 line2"`+"\n",
		"turns: 3\n",
		"---\n\n# Fix: the \"login\" bug\n",
		"> claude · 2026-10-08 09:00 → 2026-10-08 10:00 · 目录 `/work/app` · 3 轮\n",
		"备注：line1 line2",
		"## 对话概览\n",
		"1. Fix the login bug second line\n",
		"2. Thanks, now add a test\n",
		"3. ship it\n",
		"## 用户 · 第 1 轮\n",
		"<summary>上下文</summary>",
		"Fix the login bug\nsecond line\n",
		"## 助手\n",
		"<summary>思考过程</summary>",
		"Reading the file.\n",
		"<summary>工具调用：Read · /a/login.go</summary>", "```json\n",
		"<summary>工具结果：Read</summary>",
		"## 用户 · 第 2 轮\n",
	)
	if n := strings.Count(md, "## 助手"); n != 2 {
		t.Fatalf("工具结果不应把助手轮次拆开：%d 个助手标题\n%s", n, md)
	}
	if !strings.Contains(md, "\n"+ExportIDLine("sess1")+"\n") {
		t.Fatal("front matter 里缺少可识别的 session_id 行")
	}
}

// 轮次少时不出概览；没有的字段不写；空会话有说明。
func TestWriteExportMarkdownShortAndEmpty(t *testing.T) {
	s, tr := exportFixture()
	tr.Parts = tr.Parts[:4]
	var b bytes.Buffer
	if err := WriteExportMarkdown(&b, s, "claude", tr, ExportInfo{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "对话概览") {
		t.Fatalf("单轮会话不该有概览:\n%s", b.String())
	}
	if strings.Contains(b.String(), "exported_at") || strings.Contains(b.String(), "tags:") {
		t.Fatalf("没有的字段不该写:\n%s", b.String())
	}
	b.Reset()
	s.Title = ""
	if err := WriteExportMarkdown(&b, s, "claude", Transcript{}, ExportInfo{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "此会话中没有任何内容") || !strings.Contains(b.String(), "# （无标题）") {
		t.Fatalf("空会话:\n%s", b.String())
	}
}

// 被截断的参数不是合法 JSON，摘要退化成只有工具名，不报错。
func TestToolBrief(t *testing.T) {
	for in, want := range map[string]string{
		`{"command":["ls","-la"]}`: "ls -la",
		`{"command":"git status"}`: "git status",
		`{"file_path":"/a/b.go"}`:  "/a/b.go",
		`{"unknown":1}`:            "",
		`{"command":"git sta`:      "",
		`plain text`:               "",
	} {
		if got := toolBrief(in); got != want {
			t.Errorf("toolBrief(%q)=%q want %q", in, got, want)
		}
	}
}

// 工具输出（PDF 提取、二进制工具）里的 NUL 等控制字符不能原样写进 .md：许多编辑器见到 NUL
// 会把整个文件当二进制或猜错编码，后半段就成了乱码。
func TestWriteExportMarkdownDropsControlCharacters(t *testing.T) {
	s := Session{Agent: "claude", ID: "s", Title: "标题\x00带NUL"}
	tr := Transcript{Parts: []Part{
		{Role: "user", Kind: "text", Text: "看看这个PDF"},
		{Role: "tool", Kind: "tool_result", Name: "py\x00thon", Text: "\x00\x00海尔AD域\x00\r\n\x01项目概述\x7f\u0085\r\n\uf06c 服务范围\uf0b7 备份\ue123\t制表符保留"},
	}}
	var b bytes.Buffer
	if err := WriteExportMarkdown(&b, s, "claude", tr, ExportInfo{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, bad := range []string{"\x00", "\x01", "\x7f", "\u0085", "\uf06c", "\uf0b7"} {
		if strings.Contains(out, bad) {
			t.Fatalf("导出里不该有 %q", bad)
		}
	}
	for _, want := range []string{"海尔AD域\r\n项目概述\r\n● 服务范围• 备份", "制表符保留", "标题带NUL", "工具结果：python"} {
		if !strings.Contains(out, want) {
			t.Fatalf("缺少 %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\ue123") {
		t.Fatal("不认识的私有区字符不该被擅自删掉")
	}
	if !utf8.ValidString(out) {
		t.Fatal("导出必须是合法 UTF-8")
	}
	// 没有需要处理的字符时原样返回
	if exportClean("普通 text\n\t正常") != "普通 text\n\t正常" {
		t.Fatal("干净文本不该被改动")
	}
}
