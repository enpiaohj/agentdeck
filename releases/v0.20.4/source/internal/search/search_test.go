package search

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// keys 提一次命中的 Key 集合，便于按集合断言。
func keys(rs []Result) map[string]bool {
	m := make(map[string]bool, len(rs))
	for _, r := range rs {
		m[r.Key] = true
	}
	return m
}

// hits 断言 query 恰好命中 want 里的 Key；misses 里的 Key 一个也不许中。
func hits(t *testing.T, ix *Index, query string, want, misses []string) {
	t.Helper()
	rs, err := ix.Search(context.Background(), query, 0)
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	got := keys(rs)
	for _, k := range want {
		if !got[k] {
			t.Errorf("query %q 没命中 %q（命中了 %v）", query, k, got)
		}
	}
	for _, k := range misses {
		if got[k] {
			t.Errorf("query %q 命中了不该命中的 %q", query, k)
		}
	}
}

// Segment 是分词契约：这几个形态是它的全部规则——CJK 连续段两两成
// bigram，孤字自成词元，字母数字连续段成词，全部小写，标点空白分隔。
func TestSegment(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"远程连接", []string{"远程", "程连", "连接"}},
		{"远程", []string{"远程"}},
		{"远", []string{"远"}},
		{"数据库", []string{"数据", "据库"}},
		{"Hello, 世界", []string{"hello", "世界"}},
		{"修复bug2024", []string{"修复", "bug2024"}}, // CJK 与拉丁字母的边界同标点，不成跨界 bigram
		{"在Auth.go里", []string{"在", "auth", "go", "里"}},
		{"ひらがなカタカナ", []string{"ひら", "らが", "がな", "なカ", "カタ", "タカ", "カナ"}},
		{"。、！？", nil},                         // CJK 标点是分隔符，不是 CJK 字符
		{"bug_2024", []string{"bug", "2024"}}, // 下划线同 unicode61，是分隔符
	} {
		got := Segment(c.in)
		if len(got) != len(c.want) {
			t.Errorf("Segment(%q) = %q, want %q", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("Segment(%q) = %q, want %q", c.in, got, c.want)
				break
			}
		}
	}
}

// F04 验收：「2 字中文词必须命中」。远程/连接/实现 这三个词在
// unicode61 与 trigram 下全部 0 命中（本机实测），是本次分词选型的
// 直接回归。
func TestTwoCharChineseHits(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	err := ix.Add([]Doc{
		{Key: "a#0", Text: "正在连接远程服务器"},
		{Key: "b#0", Text: "修复了实现里的一个 bug"},
	})
	if err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "远程", []string{"a#0"}, []string{"b#0"})
	hits(t, ix, "连接", []string{"a#0"}, nil)
	hits(t, ix, "实现", []string{"b#0"}, []string{"a#0"})
	hits(t, ix, "登录", nil, []string{"a#0", "b#0"}) // 没说过的词不中
}

// F04 验收：「CJK 文本中的 ASCII 词必须命中」。
func TestASCIIWithinCJK(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{{Key: "a#0", Text: "在Auth.go里修复登录bug"}}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "auth", []string{"a#0"}, nil)
	hits(t, ix, "Auth.go", []string{"a#0"}, nil) // 两个词都要在
	hits(t, ix, "登录 bug", []string{"a#0"}, nil)
	hits(t, ix, "login", nil, []string{"a#0"})
}

// bigram 短语保住相邻关系：连续的「远程连接」命中，「远程的连接」
// 不因两个词都在而被误中。
func TestPhraseAdjacency(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{
		{Key: "a#0", Text: "远程连接"},
		{Key: "b#0", Text: "远程的连接"},
	}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "远程连接", []string{"a#0"}, []string{"b#0"})
	// 拆开查，两篇都中
	hits(t, ix, "远程 连接", []string{"a#0", "b#0"}, nil)
}

// 同一 Key 再写入是替换：旧正文随之失效，不留下两份。
func TestReplaceSameKey(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{{Key: "a#0", Text: "远程连接"}}); err != nil {
		t.Fatal(err)
	}
	if err := ix.Add([]Doc{{Key: "a#0", Text: "本地缓存"}}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "本地缓存", []string{"a#0"}, nil)
	hits(t, ix, "远程", nil, []string{"a#0"})
}

// 相关度按 bm25 排序：词出现得越多的文档越靠前。
func TestRankOrdersByBM25(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{
		{Key: "once", Text: "远程连接出现过一次远程"},
		{Key: "many", Text: "远程 远程 远程服务器上的远程连接"},
	}); err != nil {
		t.Fatal(err)
	}
	rs, err := ix.Search(context.Background(), "远程", 0)
	if err != nil || len(rs) != 2 {
		t.Fatalf("Search: %v %v", rs, err)
	}
	if rs[0].Key != "many" {
		t.Errorf("bm25 把 %q 排在了 %q 前面", rs[0].Key, "many")
	}
	if rs[0].Rank > rs[1].Rank {
		t.Errorf("rank 应越小越相关: %v > %v", rs[0].Rank, rs[1].Rank)
	}
}

// 无词元的查询（空串、纯标点）返回空结果而非错误；取消的 ctx 随查询中断。
func TestEmptyAndCancelledQueries(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{{Key: "a#0", Text: "远程连接"}}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"", "   ", "。、！？", "!!!"} {
		rs, err := ix.Search(context.Background(), q, 0)
		if err != nil || len(rs) != 0 {
			t.Errorf("Search(%q) = %v, %v; want 空", q, rs, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ix.Search(ctx, "远程", 0); !errors.Is(err, context.Canceled) {
		t.Errorf("取消后的查询 err = %v; want context.Canceled", err)
	}
}

// 切不出词元的正文跳过不收。
func TestEmptyTextSkipped(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{{Key: "a#0", Text: "。、！？"}, {Key: "b#0", Text: "远程"}}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "远程", []string{"b#0"}, nil)
}

// Reset 之后旧命中消失，索引可整库重造。
func TestReset(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{{Key: "a#0", Text: "远程连接"}}); err != nil {
		t.Fatal(err)
	}
	if err := ix.Reset(); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "远程", nil, nil)
	if err := ix.Add([]Doc{{Key: "a#0", Text: "本地缓存"}}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "本地缓存", []string{"a#0"}, nil)
}

// 落盘的索引关掉再开，检索还在：索引库是持久的派生物，不是随进程
// 而去的内存态。
func TestPersistenceOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "search.db")
	ix, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.Add([]Doc{{Key: "a#0", Text: "修复远程连接超时"}}); err != nil {
		t.Fatal(err)
	}
	if err := ix.Close(); err != nil {
		t.Fatal(err)
	}
	ix, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	hits(t, ix, "远程连接", []string{"a#0"}, nil)
}

// 同一 Key 的正文变得无一词可中（脱敏、清空后只剩标点），替换语义
// 不含例外：旧行一起删，检索不再给出已失效的旧内容。
func TestReplaceWithZeroTokenText(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{{Key: "a#0", Text: "远程连接"}}); err != nil {
		t.Fatal(err)
	}
	if err := ix.Add([]Doc{{Key: "a#0", Text: "。、！？😀"}}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "远程", nil, []string{"a#0"})
}

// IME 打出的全角字母数字折到半角：全角正文用半角查询要命中，反之亦然
// （unicode61 只去变音符号，宽度折叠只能由 Segment 做）。
func TestFullWidthFoldedToASCII(t *testing.T) {
	if got := Segment("端口８０８０"); !slices.Equal(got, []string{"端口", "8080"}) {
		t.Errorf(`Segment("端口８０８０") = %q, want [端口 8080]`, got)
	}
	ix := mustOpen(t, "")
	defer ix.Close()
	if err := ix.Add([]Doc{
		{Key: "a#0", Text: "端口８０８０被占用"},
		{Key: "b#0", Text: "版本ＡＢＣ发布"},
	}); err != nil {
		t.Fatal(err)
	}
	hits(t, ix, "8080", []string{"a#0"}, nil)
	hits(t, ix, "abc", []string{"b#0"}, nil)
}

// Count 是不受 limit 截断的真实命中数：Search 取前 20 条时报数用它。
func TestCount(t *testing.T) {
	ix := mustOpen(t, "")
	defer ix.Close()
	var docs []Doc
	for i := range 25 {
		docs = append(docs, Doc{Key: strconv.Itoa(i) + "#0", Text: "远程连接"})
	}
	docs = append(docs, Doc{Key: "x#0", Text: "本地缓存"})
	if err := ix.Add(docs); err != nil {
		t.Fatal(err)
	}
	if n, err := ix.Count(context.Background(), "远程"); err != nil || n != 25 {
		t.Fatalf("Count(远程) = %d, %v; want 25", n, err)
	}
	if n, err := ix.Count(context.Background(), "本地"); err != nil || n != 1 {
		t.Fatalf("Count(本地) = %d, %v; want 1", n, err)
	}
	if rs, _ := ix.Search(context.Background(), "远程", 20); len(rs) != 20 {
		t.Fatalf("Search(limit 20) = %d 条; want 20", len(rs))
	}
}

func mustOpen(t *testing.T, path string) *Index {
	t.Helper()
	ix, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

// MatchOf 是查询侧的最终形态，只在此固定一次：每个词一个短语，词间
// AND。短语里的词元序列由同一个 Segment 给出。
func TestMatchOf(t *testing.T) {
	for _, c := range []struct {
		in, want string
	}{
		{"", ""},
		{"!!!", ""},
		{"远程", `"远程"`},
		{"远程连接", `"远程 程连 连接"`},
		{"修复 远程", `"修复" AND "远程"`},
		{"  Auth.go  ", `"auth go"`}, // 词内词元相连成短语，词间才有 AND
	} {
		if got := MatchOf(c.in); got != c.want {
			t.Errorf("MatchOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// 引号只是分隔符：查 "a" 就是查词 a
	if got := MatchOf(`"a"`); got != `"a"` {
		t.Errorf(`MatchOf(%q) = %q, want "a"`, `"a"`, got)
	}
}
