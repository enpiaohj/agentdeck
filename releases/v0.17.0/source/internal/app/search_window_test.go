package app

import (
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestSearchOffsetLoadsLongPartWithoutChangingSource(t *testing.T) {
	text := strings.Repeat("旧😀", 6000) + "target\n\t phrase" + strings.Repeat("后", 12000)
	full := sessions.Transcript{Parts: []sessions.Part{{Role: "user", Kind: "text", Text: text}}}
	hit, ok := sessions.SearchRetainedTranscript(full, sessions.NeedleOf("target phrase"))
	if !ok || hit.Start != 12000 || hit.End != 12015 {
		t.Fatalf("原文偏移错误: %+v", hit)
	}
	view := conversationWindowAt(full, 0, false, hit.Start)
	p := view.Parts[0]
	if view.Start != 0 || view.Total != 1 || p.OmittedBefore != 10000 || !strings.Contains(p.Text, "target\n\t phrase") || p.Cut == 0 {
		t.Fatalf("命中窗口错误: %+v", view)
	}
	next := conversationWindowAt(full, 0, false, p.OmittedBefore+len([]rune(p.Text)))
	if next.Parts[0].OmittedBefore > p.OmittedBefore+len([]rune(p.Text)) {
		t.Fatal("前后窗口不能跳字")
	}
	previous := conversationWindowAt(full, 0, false, max(0, p.OmittedBefore-6000))
	if previous.Parts[0].OmittedBefore >= p.OmittedBefore || previous.Parts[0].OmittedBefore+len([]rune(previous.Parts[0].Text)) < p.OmittedBefore {
		t.Fatal("读取前文不能停留原窗口或跳字")
	}
	beginning := conversationWindowAt(full, 0, false, 0)
	if beginning.Parts[0].OmittedBefore != 0 || !strings.HasPrefix(beginning.Parts[0].Text, "旧😀") {
		t.Fatal("显式回到提问开头必须从原文起点读取")
	}
	if full.Parts[0].Text != text {
		t.Fatal("不得改变保留/导出原文")
	}
}
