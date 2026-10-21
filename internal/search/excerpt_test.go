package search

import (
	"strings"
	"testing"
)

// 锚定按词元：查询词里的标点只是分隔符，原文用不同的标点也落得到。
func TestExcerptAnchorsByToken(t *testing.T) {
	text := "前面垫一句话。配置文件在 D:\\Work\\Notes 里，后面再垫几句凑长度，让两侧都能截断出省略号来。"
	got := Excerpt(text, "D:/Work", 10, 20)
	if !strings.Contains(got, "Work") {
		t.Fatalf("片段没落到命中处: %q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("两侧应示截断: %q", got)
	}
}

// 取找得到的词元里最长的作锚：单字词元不抢定位。
func TestExcerptPrefersLongestToken(t *testing.T) {
	text := "短词 a 出现在开头，真正的目标是数据库连接池配置段。"
	got := Excerpt(text, "数据库 连接", 5, 10)
	if !strings.Contains(got, "数据库") {
		t.Fatalf("锚错了位置: %q", got)
	}
}

// rune 对齐：中文与 emoji 混排不切出半个字。
func TestExcerptRuneAlignment(t *testing.T) {
	text := "😀😀😀修复远程连接超时😀😀😀的问题出现在网关侧。"
	got := Excerpt(text, "远程连接", 3, 3)
	if strings.Contains(got, "\uFFFD") {
		t.Fatalf("切出了残缺 rune: %q", got)
	}
	if !strings.Contains(got, "远程连接") {
		t.Fatalf("片段: %q", got)
	}
}

// 一个词元都落不到原文上时退回开头。
func TestExcerptFallsBackToHead(t *testing.T) {
	if got := Excerpt("开头这段被显示", "不存在", 10, 5); got != "开头这段被…" {
		t.Fatalf("fallback = %q", got)
	}
	// 短文本原样返回
	if got := Excerpt("短", "不存在", 10, 5); got != "短" {
		t.Fatalf("short = %q", got)
	}
}
