package search

// 命中片段从原文上切（不从分词后的串上切——后者与渲染的原文对不上，
// 是设计评审指出过的坑）。锚定用 Segment 的词元：分词契约里标点只是
// 分隔符，查询 D:/Work 命中的原文可能写作 D:\Work，按查询
// 原文找字面子串是找不到的；取找得到的词元里最长的作锚，避开单字
// 词元的噪声。

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Excerpt 从 text 上切 query 命中处附近的一段：命中词前 before、后
// after 个 rune，越界侧以 … 示意截断。query 与索引侧共用 Segment
// 定位；一个词元都落不到原文上时退回开头 after 个 rune。
func Excerpt(text, query string, before, after int) string {
	low := foldRunes(text)
	at, n := -1, 0
	for _, tok := range Segment(query) {
		if i := strings.Index(low, tok); i >= 0 {
			if n0 := utf8.RuneCountInString(tok); n0 > n {
				at, n = len([]rune(low[:i])), n0
			}
		}
	}
	runes := []rune(text)
	if at < 0 {
		if len(runes) > after {
			return string(runes[:after]) + "…"
		}
		return text
	}
	from, to := max(0, at-before), min(len(runes), at+n+after)
	out := string(runes[from:to])
	if from > 0 {
		out = "…" + strings.TrimLeft(out, " ")
	}
	if to < len(runes) {
		out = strings.TrimRight(out, " ") + "…"
	}
	return out
}

// Anchor 返回 query 的词元里在 text（折叠后）中实际存在的最长词元，
// 一个都没有时为空串。搜索结果的高亮词用它——Excerpt 也按同一规则
// 锚定，两者不会指到不同位置。
func Anchor(text, query string) string {
	low := foldRunes(text)
	best := ""
	for _, tok := range Segment(query) {
		if n0 := utf8.RuneCountInString(tok); n0 > utf8.RuneCountInString(best) &&
			strings.Contains(low, tok) {
			best = tok
		}
	}
	if best == "" {
		return ""
	}
	at := utf8.RuneCountInString(low[:strings.Index(low, best)])
	return string([]rune(text)[at : at+utf8.RuneCountInString(best)])
}

// ExcerptLocation 返回原文 rune 区间与摘要 UTF-16 区间，前端无需再次猜测匹配规则。
func ExcerptLocation(text, query string, before, after int) (snippet, match string, start, end, snippetStart, snippetEnd int) {
	match = Anchor(text, query)
	snippet = Excerpt(text, query, before, after)
	if match == "" {
		return snippet, match, -1, -1, -1, -1
	}
	at := strings.Index(text, match)
	start = utf8.RuneCountInString(text[:at])
	end = start + utf8.RuneCountInString(match)
	i := strings.Index(snippet, match)
	if i >= 0 {
		snippetStart = utf16Length(snippet[:i])
		snippetEnd = snippetStart + utf16Length(match)
	} else {
		snippetStart, snippetEnd = -1, -1
	}
	return
}

func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}

// foldRunes 逐 rune 小写，保持 rune 序号对齐，与 sessions 包同一做法。
func foldRunes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		} else if r == 0x3000 {
			r = ' '
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
