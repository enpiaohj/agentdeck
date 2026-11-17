package search

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestExcerptOriginalOffsetsRemainAlignedAfterClipping(t *testing.T) {
	text := strings.Repeat("前😀", 100) + "Work" + strings.Repeat("后", 200)
	snippet, match, start, end, a, b := ExcerptLocation(text, "work", 36, 120)
	if match != "Work" || start != 200 || end != 204 || !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") {
		t.Fatalf("原文区间或裁剪错误: %q %q %d %d", snippet, match, start, end)
	}
	units := utf16.Encode([]rune(snippet))
	if a < 0 || b > len(units) || string(utf16.Decode(units[a:b])) != match {
		t.Fatalf("摘要 UTF-16 区间错误: %d %d %q", a, b, snippet)
	}
}

func TestExcerptWithoutAnchorUsesNegativeOffsets(t *testing.T) {
	_, match, start, end, a, b := ExcerptLocation("无匹配文本", "missing", 36, 120)
	if match != "" || start != -1 || end != -1 || a != -1 || b != -1 {
		t.Fatalf("无命中区间错误: %q %d %d %d %d", match, start, end, a, b)
	}
}

func TestExcerptOriginalWidthCaseAndUTF16Offsets(t *testing.T) {
	text := "😀端口 ＡＢＣ８０８０ 连接成功"
	snippet, match, start, end, a, b := ExcerptLocation(text, "abc8080", 36, 120)
	if match != "ＡＢＣ８０８０" || start != 4 || end != 11 || a != 5 || b != 12 || snippet != text {
		t.Fatalf("%q %q %d %d %d %d", snippet, match, start, end, a, b)
	}
}
