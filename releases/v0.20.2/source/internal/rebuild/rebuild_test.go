package rebuild

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const testID = "a1b2c3d4-2222-3333-4444-5555555555ee"

func TestProjectDirName(t *testing.T) {
	for cwd, want := range map[string]string{
		`D:\AIProjects\RemoteFlow`: "D--AIProjects-RemoteFlow",
		`D:\工作\2023\海尔`:            "D-----2023---",
		`C:\Users\a b\x.y_z`:       "C--Users-a-b-x-y-z",
		`D:\temp\😀`:                "D--temp---",
	} {
		got, err := ProjectDirName(cwd)
		if err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", cwd, got, err, want)
		}
	}
	if _, err := ProjectDirName(""); err == nil {
		t.Error("空目录应报错")
	}
	if _, err := ProjectDirName(`D:\` + strings.Repeat("a", 250)); err == nil {
		t.Error("过长的目录名 Claude 会改用哈希，应拒绝")
	}
}

func TestValidID(t *testing.T) {
	if !ValidID(testID) {
		t.Fatal("合法 ID 被拒绝")
	}
	for _, bad := range []string{"", "../x", testID + "/..", strings.ToUpper(testID), "gpt_abc", testID[:35]} {
		if ValidID(bad) {
			t.Errorf("%q 不该通过", bad)
		}
	}
}

func part(role, kind, text string) sessions.Part {
	return sessions.Part{Role: role, Kind: kind, Text: text}
}

func sample() sessions.Transcript {
	return sessions.Transcript{Parts: []sessions.Part{
		part("user", "context", "<command-name>/init</command-name>"),
		part("user", "text", "修好登录\x00的 bug"),
		part("assistant", "thinking", "秘密的思考内容"),
		part("assistant", "text", "我先看看文件。"),
		{Role: "assistant", Kind: "tool_use", Name: "Read", Text: `{"file_path":"/a/login.go"}`},
		{Role: "tool", Kind: "tool_result", Name: "Read", Text: "package login"},
		part("assistant", "text", "已定位。"),
		part("user", "text", "好，改吧"),
		{Role: "assistant", Kind: "text", Text: "改好了。", Cut: 12},
	}}
}

func TestMessagesFlattenAndAlternate(t *testing.T) {
	msgs, st := Messages(sample(), 0)
	if len(msgs) != 4 {
		t.Fatalf("应是 用户/助手/用户/助手 四条: %+v", msgs)
	}
	for i, m := range msgs {
		if want := []string{"user", "assistant"}[i%2]; m.Role != want {
			t.Fatalf("第 %d 条角色 %s，应为 %s", i, m.Role, want)
		}
	}
	all := ""
	for _, m := range msgs {
		all += m.Text + "\n"
	}
	for _, want := range []string{"修好登录的 bug", "我先看看文件。", "【工具调用：Read】", `"file_path":"/a/login.go"`, "【工具结果：Read】", "package login", "已定位。", "已省略后面的 12 个字符"} {
		if !strings.Contains(all, want) {
			t.Errorf("缺少 %q:\n%s", want, all)
		}
	}
	for _, bad := range []string{"秘密的思考内容", "<command-name>", "\x00"} {
		if strings.Contains(all, bad) {
			t.Errorf("不该带回 %q", bad)
		}
	}
	if st.Turns != 2 || st.Tools != 1 || st.Kept != 4 || st.Omitted != 0 {
		t.Errorf("统计: %+v", st)
	}
}

func TestMessagesLimitsLongToolOutput(t *testing.T) {
	long := strings.Repeat("长", 10_000)
	msgs, _ := Messages(sessions.Transcript{Parts: []sessions.Part{
		part("user", "text", "跑一下"),
		{Role: "tool", Kind: "tool_result", Name: "Bash", Text: long},
	}}, 0)
	if n := utf8.RuneCountInString(msgs[1].Text); n > 2_000 {
		t.Fatalf("工具结果应被截断，实际 %d 个字符", n)
	}
	if !strings.Contains(msgs[1].Text, "已省略") {
		t.Fatal("截断要注明省略了多少")
	}
}

func TestMessagesTrimKeepsHeadAndTail(t *testing.T) {
	var parts []sessions.Part
	for i := 0; i < 40; i++ {
		parts = append(parts, part("user", "text", fmt.Sprintf("问题%02d %s", i, strings.Repeat("问", 400))))
		parts = append(parts, part("assistant", "text", fmt.Sprintf("回答%02d %s", i, strings.Repeat("答", 400))))
	}
	msgs, st := Messages(sessions.Transcript{Parts: parts}, 10_000)
	if st.Omitted == 0 || st.Chars > 12_500 {
		t.Fatalf("应当省略中间并控制总量: %+v", st)
	}
	if msgs[0].Role != "user" || !strings.HasPrefix(msgs[0].Text, "问题00") {
		t.Fatalf("开头应保留第一条用户消息: %q", msgs[0].Text[:20])
	}
	if msgs[1].Role != "assistant" || !strings.Contains(msgs[1].Text, fmt.Sprintf("%d 条", st.Omitted)) {
		t.Fatalf("第二条应是省略说明: %q", msgs[1].Text)
	}
	if msgs[2].Role != "user" {
		t.Fatal("保留的结尾必须从用户发言开始")
	}
	if last := msgs[len(msgs)-1]; !strings.HasPrefix(last.Text, "回答39") {
		t.Fatalf("结尾应保留最近的内容: %q", last.Text[:12])
	}
	for i := 1; i < len(msgs); i++ {
		if msgs[i].Role == msgs[i-1].Role {
			t.Fatalf("第 %d 条与上一条角色相同", i)
		}
	}
}

func TestBuildProducesResumableChain(t *testing.T) {
	s := sessions.Session{ID: testID, Title: "修复  登录", Start: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), Models: []sessions.Model{{Model: "claude-opus-5-5"}}}
	data, st, err := Build(s, sample(), `D:\work\app`, time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	again, _, _ := Build(s, sample(), `D:\work\app`, time.Now().Add(time.Hour), 0)
	if !bytes.Equal(data, again) {
		t.Fatal("同样的输入应得到同样的字节（预览与写入要能比对哈希）")
	}
	if st.Bytes != len(data) || bytes.ContainsRune(data, 0) {
		t.Fatal("字节数统计错误或含 NUL")
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 6 { // 说明 + 4 条消息 + 标题
		t.Fatalf("行数 %d: %s", len(lines), data)
	}
	var prev string
	for i, l := range lines[:5] {
		var e struct {
			ParentUUID *string `json:"parentUuid"`
			UUID       string  `json:"uuid"`
			Type       string  `json:"type"`
			SessionID  string  `json:"sessionId"`
			Cwd        string  `json:"cwd"`
			IsMeta     bool    `json:"isMeta"`
			Message    struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
				Model   string          `json:"model"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("第 %d 行不是合法 JSON: %v", i, err)
		}
		if e.SessionID != testID || e.Cwd != `D:\work\app` || e.Type != e.Message.Role {
			t.Fatalf("第 %d 行字段不对: %s", i, l)
		}
		if i == 0 && (e.ParentUUID != nil || !e.IsMeta) {
			t.Fatal("第一行应是没有父节点的说明（isMeta）")
		}
		if i > 0 && (e.ParentUUID == nil || *e.ParentUUID != prev) {
			t.Fatalf("第 %d 行没有接在上一行后面", i)
		}
		if e.Type == "assistant" && e.Message.Model != "claude-opus-5-5" && e.Message.Model != "" {
			t.Fatalf("助手消息的模型: %q", e.Message.Model)
		}
		if e.Type == "assistant" && !strings.HasPrefix(string(e.Message.Content), "[") {
			t.Fatal("助手内容应是块数组")
		}
		prev = e.UUID
	}
	if !strings.Contains(lines[5], `"custom-title"`) || !strings.Contains(lines[5], "修复 登录") {
		t.Fatalf("最后一行应是标题: %s", lines[5])
	}
	if strings.Contains(string(data), "秘密的思考内容") {
		t.Fatal("思考内容不该带回")
	}
	if !strings.Contains(string(data), "<system-reminder>") || strings.Contains(string(data), `\u003c`) {
		t.Fatal("说明里的标签不该被转义成 \\u003c")
	}
}

func TestBuildRejectsBadInput(t *testing.T) {
	if _, _, err := Build(sessions.Session{ID: "../x"}, sample(), `D:\a`, time.Now(), 0); err == nil {
		t.Error("非法 ID 应报错")
	}
	if _, _, err := Build(sessions.Session{ID: testID}, sample(), "", time.Now(), 0); err == nil {
		t.Error("没有目录应报错")
	}
	if _, _, err := Build(sessions.Session{ID: testID}, sessions.Transcript{Parts: []sessions.Part{part("assistant", "thinking", "x")}}, `D:\a`, time.Now(), 0); err == nil {
		t.Error("没有可重建的文字应报错")
	}
}

func TestHandoff(t *testing.T) {
	md, st, err := Handoff(sessions.Session{ID: testID, Title: "修复登录"}, sample(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# 此前的对话记录：修复登录", "## 用户", "## 助手", "修好登录的 bug", "【工具结果：Read】"} {
		if !strings.Contains(md, want) {
			t.Errorf("缺少 %q", want)
		}
	}
	if st.Kept != 4 || strings.Contains(md, "秘密的思考内容") {
		t.Errorf("%+v", st)
	}
}
