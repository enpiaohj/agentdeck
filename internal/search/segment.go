// Package search 是 AgentDeck 的全文检索索引层：对会话正文建立
// 纯派生、可整库重建的 FTS5 索引，弥补 magpie 会话层只有逐文件子串
// 匹配（sessions.SearchSession）而没有全文索引的空白。
//
// 中文分词采用 bigram 预分词（设计评审报告的建议）：本机实测
// unicode61 与 trigram 两种 tokenizer 对 2 字中文词（「远程」「连接」
// 「实现」）全部 0 命中，而 2 字词正是中文查询最常见的形态。唯一
// 可靠的做法是索引侧与查询侧共用同一个分词函数，预先把 CJK 相邻
// 字符两两成词。该契约由 Segment 一处实现，Index.Add 与 Search
// 两侧都必须经过它，不可只在单侧调用。
//
// 本包只读派生数据，不触碰任何上游会话存储；入库正文由调用方提供。
package search

import (
	"strings"
	"unicode"
)

// Segment 是索引与查询共用的分词契约：把 text 切成词元流。
//
// CJK（汉字、假名、谚文）连续段做相邻二字 bigram，孤立的单一 CJK
// 字符自成词元；其余按字母/数字连续段成词。全部词元小写化，不含
// 空白与标点，以空格连接后即可交给 FTS5（unicode61）原样收词。
//
// bigram 保住的是相邻关系：「远程连接」切成 远程/程连/连接，查询
// 「远程连接」切出同样的词元序列，短语匹配即等价于子串匹配；代价
// 是跨词元的 1 字查询（「连」对「连接」）不命中，与 Lucene
// CJKAnalyzer 同一取舍。
func Segment(text string) []string {
	var toks []string
	var word, cjk []rune
	flushWord := func() {
		if len(word) > 0 {
			toks = append(toks, string(word))
			word = word[:0]
		}
	}
	flushCJK := func() {
		switch len(cjk) {
		case 0:
		case 1:
			toks = append(toks, string(cjk))
		default:
			for i := 0; i+1 < len(cjk); i++ {
				toks = append(toks, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range text {
		switch {
		case isCJK(r):
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word = append(word, unicode.ToLower(r))
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return toks
}

// isCJK says whether r is a character bigram segmentation is for: Han,
// Hiragana, Katakana or Hangul. CJK 标点（。、「）不算，它们和拉丁
// 标点一样是分隔符。与 sessions 包 rawNeedle 的 CJK 判定同一范围。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// join 是一个索引行要交给 FTS5 的形态：词元以空格相连。词元本身
// 不含空白（bigram 是相邻 CJK 字符，词是连续字母数字），unicode61
// 收到的每个词就是 Segment 给出的每个词元。
func join(toks []string) string {
	return strings.Join(toks, " ")
}
