package sessions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// dsh 的正文要读得出来：现在的格式是 session.v4.jsonl.zstd（头一行之后一
// 事件一行，块状 content），旧格式的 content 还直接是字符串。列表里的
// 「可读」标记、正文、单会话检索三条路都要以同一份文件为准。
func TestDshTranscriptReadsPackedV4Session(t *testing.T) {
	home := dshHome(t)
	dir := filepath.Join(home, ".dsh", "sessions", "work-dsh-9a", "dsh-t1")
	lines := []string{
		`{"type":"session","version":4,"id":"dsh-t1","cwd":"/work/dsh","createdAt":1790503200000}`,
		`{"type":"job/registered","seq":0,"time":1790503200100,"data":{"jobId":"j1","kind":"shell"}}`,
		`{"type":"user/message","seq":1,"time":1790503200500,"data":{"role":"user","content":[{"type":"text","text":"Rebuild the build script"}],"source":{"kind":"user"}}}`,
		// dsh 自己塞进去的那条不是用户说的话，展示时收起来
		`{"type":"user/message","seq":2,"time":1790503200600,"data":{"role":"user","content":[{"type":"text","text":"<runtime context: tools>"}],"source":{"kind":"runtime-context"}}}`,
		// 一条回复里的块按顺序说：推理、调用、正文；stream 的分片不再单独算
		`{"type":"assistant/message","seq":3,"time":1790503205000,"data":{"step":0,"turn":0,"message":{"role":"assistant","content":[{"type":"reasoning","text":"Look at it first"},{"type":"tool-call","id":"c1","name":"read","arguments":"{\"file_path\":\"/work/dsh/build.sh\"}"},{"type":"text","text":"Now I'll fix it"}],"source":{"kind":"model","model":"deepseek-v4-pro","provider":"deepseek"}},"stream":[{"type":"text-chunks","time":1790503204000,"chunk":{"type":"text-delta","text":"Now"}}],"usage":{"inputTokens":100,"outputTokens":20}}}`,
		`{"type":"tool/result","seq":4,"time":1790503206000,"data":{"step":0,"turn":0,"message":{"role":"tool","content":[{"type":"text","text":"#!/bin/sh\nmake all"}],"source":{"kind":"tool","callId":"c1","toolCallId":""}}}}`,
		`{"type":"assistant/message","seq":5,"time":1790503207000,"data":{"message":{"role":"assistant","content":[{"type":"tool-call","id":"c2","name":"read_image","arguments":"{}"}],"source":{"kind":"model","model":"deepseek-v4-pro"}}}}`,
		`{"type":"tool/result","seq":6,"time":1790503208000,"data":{"message":{"role":"tool","content":[{"type":"text","text":"screenshot"},{"type":"image","attachment":{"kind":"image"}}],"source":{"kind":"tool","callId":"c2"}}}}`,
		`{"type":"system/message","seq":7,"time":1790503209000,"data":{"message":{"role":"system","content":[{"type":"text","text":"You are dsh"}],"source":{"kind":"system-prompt"}}}}`,
		`{"type":"session/title","seq":8,"time":1790503210000,"data":{"title":"Build script tidy","source":{"kind":"provider"}}}`,
	}
	dshWrite(t, filepath.Join(dir, "session.v4.jsonl.zstd"), strings.Join(lines, "\n")+"\n", true)

	s := find(t, List(0), "dsh", "dsh-t1")
	if !s.Transcript {
		t.Fatal("列表必须把 dsh 标成可读（否则详情页只会说读不了）")
	}
	if !strings.HasSuffix(s.Path, "session.v4.jsonl.zstd") {
		t.Fatalf("会话文件应当是压缩的那一份: %s", s.Path)
	}
	tr, err := TranscriptOf(s)
	if err != nil {
		t.Fatal(err)
	}
	want := []Part{
		{Role: "user", Kind: "text", Text: "Rebuild the build script"},
		{Role: "user", Kind: "context", Text: "<runtime context: tools>"},
		{Role: "assistant", Kind: "thinking", Text: "Look at it first", Model: "deepseek-v4-pro"},
		{Role: "assistant", Kind: "tool_use", Name: "read", Text: "{\n  \"file_path\": \"/work/dsh/build.sh\"\n}", Model: "deepseek-v4-pro"},
		{Role: "assistant", Kind: "text", Text: "Now I'll fix it", Model: "deepseek-v4-pro"},
		{Role: "tool", Kind: "tool_result", Name: "read", Text: "#!/bin/sh\nmake all"},
		{Role: "assistant", Kind: "tool_use", Name: "read_image", Text: "{}", Model: "deepseek-v4-pro"},
		{Role: "tool", Kind: "tool_result", Name: "read_image", Text: "screenshot\n[image]"},
	}
	if len(tr.Parts) != len(want) {
		t.Fatalf("段数 %d，想要 %d: %+v", len(tr.Parts), len(want), tr.Parts)
	}
	for i := range want {
		if tr.Parts[i] != want[i] {
			t.Fatalf("第 %d 段不对:\n got %+v\nwant %+v", i, tr.Parts[i], want[i])
		}
	}
	// 单会话检索也要读同一份压缩文件，命中的正是工具输出里的一行
	h, ok, err := SearchSession(context.Background(), s, NeedleOf("make all"))
	if err != nil || !ok || !strings.Contains(h.Match, "make all") {
		t.Fatalf("压缩会话里应当能检索到工具输出: ok=%v err=%v hit=%+v", ok, err, h)
	}
}

// 旧格式（content 是字符串）也要读得出，免得只认新写法。
func TestDshTranscriptReadsStringContent(t *testing.T) {
	home := dshHome(t)
	dir := filepath.Join(home, ".dsh", "sessions", "work-dsh-9b", "dsh-t2")
	lines := []string{
		`{"type":"session","version":3,"id":"dsh-t2","cwd":"/work/dsh","createdAt":1790503200000}`,
		`{"type":"user/message","seq":0,"time":1790503200500,"data":{"content":"Tidy the build script","source":{"kind":"user"}}}`,
		`{"type":"assistant/message","seq":1,"time":1790503205000,"data":{"message":{"role":"assistant","content":"Tidied it","source":{"model":"deepseek-v4-pro"}}}}`,
	}
	dshWrite(t, filepath.Join(dir, "session.v3.jsonl"), strings.Join(lines, "\n")+"\n", false)

	tr, err := TranscriptOf(find(t, List(0), "dsh", "dsh-t2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Parts) != 2 {
		t.Fatalf("段数 %d: %+v", len(tr.Parts), tr.Parts)
	}
	if tr.Parts[0] != (Part{Role: "user", Kind: "text", Text: "Tidy the build script"}) {
		t.Fatalf("用户那段: %+v", tr.Parts[0])
	}
	if tr.Parts[1] != (Part{Role: "assistant", Kind: "text", Text: "Tidied it", Model: "deepseek-v4-pro"}) {
		t.Fatalf("回复那段: %+v", tr.Parts[1])
	}
}

// dshHome 是 setup 建出来的临时 home：dsh 的会话都放在它下面的 .dsh 里。
func dshHome(t *testing.T) string {
	t.Helper()
	claude, _ := setup(t)
	return filepath.Join(filepath.Dir(claude), "home")
}

// dshWrite 写一个 dsh 会话文件，packed 为真时按 v4 那样压成 zstd。
func dshWrite(t *testing.T, path, content string, packed bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if !packed {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w, err := zstd.NewWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}
