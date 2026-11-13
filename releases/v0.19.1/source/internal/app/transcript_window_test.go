package app

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestLatestWindowIncludesActualEndAndPreservesSourceIndexes(t *testing.T) {
	full := sessions.Transcript{Parts: []sessions.Part{}}
	for i := 0; i < 240; i++ {
		full.Parts = append(full.Parts, sessions.Part{Role: "assistant", Kind: "text", Text: fmt.Sprintf("第%d段", i)})
	}
	view := conversationWindow(full, -1, false)
	if view.Total != 240 || !view.HasBefore || view.HasAfter || view.Start+len(view.Parts) != 240 || view.Parts[len(view.Parts)-1].Text != "第239段" {
		t.Fatalf("未读取真实末尾: %+v", view)
	}
	page := conversationWindow(full, 100, false)
	if page.Start != 100 || page.Parts[0].Text != "第100段" || !page.HasBefore || !page.HasAfter {
		t.Fatalf("搜索原始索引不一致: %+v", page)
	}
}

func TestLatestLongReplyShowsItsLastWordsAndEmptyWindowsAreSafe(t *testing.T) {
	text := strings.Repeat("旧", 9000) + "最后一句话"
	full := sessions.Transcript{Parts: []sessions.Part{{Role: "assistant", Kind: "text", Text: text}}}
	last := conversationWindow(full, -1, false)
	if len(last.Parts) != 1 {
		t.Fatalf("最新窗口没有正文: %+v", last)
	}
	if !strings.HasSuffix(last.Parts[0].Text, "最后一句话") || last.Parts[0].OmittedBefore == 0 || last.Parts[0].Cut != 0 {
		t.Fatal("最新回答不能只保留开头")
	}
	first := conversationWindow(full, 0, false)
	if strings.HasSuffix(first.Parts[0].Text, "最后一句话") || first.Parts[0].Cut == 0 {
		t.Fatal("开头预览应保留明确截断标记")
	}
	empty := conversationWindow(sessions.Transcript{}, -1, false)
	if empty.Total != 0 || len(empty.Parts) != 0 || empty.HasBefore || empty.HasAfter {
		t.Fatalf("空会话: %+v", empty)
	}
	if full.Parts[0].Text != text {
		t.Fatal("窗口裁剪不能修改持久正文")
	}
}

func TestLargeWindowsPageBackwardsWithoutSkippingAnyParts(t *testing.T) {
	full := sessions.Transcript{Parts: []sessions.Part{}}
	for i := 0; i < 240; i++ {
		full.Parts = append(full.Parts, sessions.Part{Role: "assistant", Kind: "text", Text: strings.Repeat("字", 8000)})
	}
	current := conversationWindow(full, -1, false)
	end := current.Start
	for end > 0 {
		previous := conversationWindow(full, end, true)
		if previous.Start+len(previous.Parts) != end || previous.Start >= end {
			t.Fatalf("相邻窗口跳段: previous=%d+%d, end=%d", previous.Start, len(previous.Parts), end)
		}
		end = previous.Start
	}
}

func TestRetainedTranscriptWindowKeepsLastWords(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	text := strings.Repeat("历史", 9000) + "真正最后一句"
	sess := claudeFixture(t, t.TempDir(), "latest-retained", text)
	a.setList([]sessions.Session{sess})
	svc := a.NewSessionService()
	view, err := svc.TranscriptWindow("claude:latest-retained", -1, false)
	if err != nil || len(view.Parts) != 1 || !strings.HasSuffix(view.Parts[0].Text, "真正最后一句") {
		t.Fatalf("原生末尾读取: %+v %v", view, err)
	}
	if err := os.Remove(sess.Path); err != nil {
		t.Fatal(err)
	}
	svc.Refresh()
	view, err = svc.TranscriptWindow("claude:latest-retained", -1, false)
	if err != nil || !view.Retained || !strings.HasSuffix(view.Parts[0].Text, "真正最后一句") {
		t.Fatalf("永久副本末尾读取: %+v %v", view, err)
	}
	if _, err := svc.TranscriptWindow("claude:latest-retained", -2, false); err == nil {
		t.Fatal("无效位置未拒绝")
	}
}

// 提问导航：每个窗口都带整段会话的全部提问，按全文段序号；只算用户的正文段，
// 空白段跳过，摘要取第一行非空文字并截断。
func TestWindowsCarryEveryPromptOfTheConversation(t *testing.T) {
	long := strings.Repeat("长", promptMarkRunes+5)
	full := sessions.Transcript{Parts: []sessions.Part{
		{Role: "user", Kind: "text", Text: "\n  第一问\n第二行"},
		{Role: "assistant", Kind: "text", Text: "回答"},
		{Role: "user", Kind: "context", Text: "上下文不算"},
		{Role: "user", Kind: "text", Text: "   "},
		{Role: "tool", Kind: "tool_result", Text: "结果"},
		{Role: "user", Kind: "text", Text: long},
	}}
	for i := 0; i < 200; i++ {
		full.Parts = append(full.Parts, sessions.Part{Role: "assistant", Kind: "text", Text: "填充"})
	}
	full.Parts = append(full.Parts, sessions.Part{Role: "user", Kind: "text", Text: "最后一问"})
	want := []PromptMark{{0, "第一问"}, {5, string([]rune(long)[:promptMarkRunes]) + "…"}, {206, "最后一问"}}
	for _, view := range []*TranscriptView{conversationWindow(full, -1, false), conversationWindow(full, 0, false)} {
		if fmt.Sprint(view.Prompts) != fmt.Sprint(want) {
			t.Fatalf("prompts = %v, want %v", view.Prompts, want)
		}
	}
	if empty := conversationWindow(sessions.Transcript{}, -1, false); empty.Prompts == nil || len(empty.Prompts) != 0 {
		t.Fatalf("empty transcript prompts = %#v, want empty list", empty.Prompts)
	}
}
