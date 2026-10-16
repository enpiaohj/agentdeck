// Command agentdeck 是 AgentDeck 的命令行入口。
//
// 当前处于 Phase 0/1：会话读取基线 + 中文全文检索。本命令列出本机各
// Agent 的历史会话；-search 对所选会话的正文建立 FTS5 索引并检索，
// 用于验证检索层的发现、分词与命中是否正确。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/enpiaohj/agentdeck/internal/search"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// useUTF8 把 Windows 控制台输出代码页切到 UTF-8。
// 本机区域为 zh-CN 而控制台代码页可能是 cp949 等，直接输出中文会乱码或报错。
func useUTF8() {
	k := syscall.NewLazyDLL("kernel32.dll")
	k.NewProc("SetConsoleOutputCP").Call(uintptr(65001))
}

func main() {
	useUTF8()

	var (
		jsonOut   = flag.Bool("json", false, "以 JSON 输出")
		limit     = flag.Int("limit", 20, "最多列出多少条（0 表示全部）")
		agent     = flag.String("agent", "", "只看某个 agent，如 claude、claude-desktop、codex")
		all       = flag.Bool("all", false, "列出全部会话（等同 -limit 0）")
		query     = flag.String("search", "", "对会话正文全文检索（范围随 -agent/-limit/-all），如 -search \"远程连接\"")
		indexPath = flag.String("index", "", "全文索引库路径；缺省用内存索引，会话多时建议落盘")
	)
	flag.Parse()

	n := *limit
	if *all {
		n = sessions.All
	} else if n == 0 {
		n = sessions.All
	}

	list := sessions.List(n)

	if *agent != "" {
		filtered := list[:0:0]
		for _, s := range list {
			if s.Agent == *agent {
				filtered = append(filtered, s)
			}
		}
		list = filtered
	}

	if *query != "" {
		runSearch(list, *query, *indexPath)
		return
	}

	if *jsonOut {
		out, err := json.MarshalIndent(list, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "序列化失败:", err)
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}

	printSummary(list)
}

// printSummary 打印会话概览：按 agent 的分布 + 会话明细。
func printSummary(list []sessions.Session) {
	if len(list) == 0 {
		fmt.Println("没有找到任何会话。")
		return
	}

	byAgent := map[string]int{}
	for _, s := range list {
		byAgent[s.Agent]++
	}
	agents := make([]string, 0, len(byAgent))
	for k := range byAgent {
		agents = append(agents, k)
	}
	sort.Slice(agents, func(i, j int) bool {
		if byAgent[agents[i]] != byAgent[agents[j]] {
			return byAgent[agents[i]] > byAgent[agents[j]]
		}
		return agents[i] < agents[j]
	})

	fmt.Printf("共 %d 条会话\n\n", len(list))
	fmt.Println("按 Agent 分布：")
	for _, a := range agents {
		fmt.Printf("  %-18s %4d\n", a, byAgent[a])
	}

	fmt.Println()
	fmt.Printf("%-16s %-20s %-17s %s\n", "AGENT", "标题", "最后活动", "恢复")
	fmt.Println(repeat("-", 100))
	for _, s := range list {
		title := s.Title
		if title == "" {
			title = "(无标题)"
		}
		title = truncate(title, 20)
		last := s.Last.Local().Format("2006-01-02 15:04")
		resume := "—"
		if s.Resume != "" {
			resume = "可恢复"
		}
		fmt.Printf("%-16s %-20s %-17s %s\n", s.Agent, title, last, resume)
	}
	_ = time.Now
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

// runSearch 对所选会话的正文建立 FTS5 索引（internal/search，bigram
// 预分词，索引侧与查询侧共用 Segment）并检索。索引是纯派生物：这里
// 每次现建，不保留增量状态。
func runSearch(list []sessions.Session, query, path string) {
	ix, err := search.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开索引失败:", err)
		os.Exit(1)
	}
	defer ix.Close()
	// 每次现建：打开既有落盘库也先整库重建。上一轮的行不在本轮范围里，
	// 残留只会给出已失效的命中与「(无标题)」，索引是纯派生物，落盘只是
	// 省内存，不省重建
	if err := ix.Reset(); err != nil {
		fmt.Fprintln(os.Stderr, "重建索引失败:", err)
		os.Exit(1)
	}

	titles := map[string]string{}
	var docs []search.Doc
	skipped := 0
	for _, s := range list {
		if !sessions.HasTranscript(s.Agent) || s.Path == "" {
			continue
		}
		tr, err := sessions.TranscriptOf(s)
		if err != nil {
			skipped++
			continue
		}
		for i, p := range tr.Parts {
			// 与 sessions 包检索同一取舍：Agent 每次请求都带的系统上下文
			// 与图片不是「说过的」内容，不收进索引
			if p.Role == "system" || p.Kind == "context" || p.Kind == "image" {
				continue
			}
			docs = append(docs, search.Doc{
				Key:  s.Agent + "/" + s.ID + "#" + strconv.Itoa(i),
				Text: p.Text,
			})
		}
		titles[s.Agent+"/"+s.ID] = s.Title
		if err := ix.Add(docs); err != nil {
			fmt.Fprintf(os.Stderr, "索引 %s/%s 失败: %v\n", s.Agent, s.ID, err)
			os.Exit(1)
		}
		docs = docs[:0]
	}
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "（%d 条会话的正文读不出来，已跳过）\n", skipped)
	}
	fmt.Fprintf(os.Stderr, "已索引 %d 个会话的正文\n\n", len(titles))

	rs, err := ix.Search(context.Background(), query, 20)
	if err != nil {
		fmt.Fprintln(os.Stderr, "检索失败:", err)
		os.Exit(1)
	}
	total, err := ix.Count(context.Background(), query)
	if err != nil {
		total = len(rs) // 计数失败退回所见条数，不假装是全部
	}
	if total == 0 {
		fmt.Println("没有命中。")
		return
	}
	fmt.Printf("「%s」命中 %d 处，按相关度显示前 %d 条：\n\n", query, total, len(rs))
	for _, r := range rs {
		title := ""
		if i := strings.LastIndex(r.Key, "#"); i >= 0 {
			title = titles[r.Key[:i]]
		}
		if title == "" {
			title = "(无标题)"
		}
		fmt.Printf("  %s\n  %s\n    (相关度 %.2f) %s\n\n", truncate(title, 24), r.Key, r.Rank, excerpt(r.Text, query))
	}
}

// excerpt 从原文上切命中词附近的一段（前 36 / 后 120 rune，与
// sessions 包的 Hit 同一尺度），越界侧以 … 示意截断。片段从原文上切，
// 不从分词后的串上切——后者与渲染的原文对不上。定位按 Segment 的
// 词元而非查询原文的字面子串：分词契约里标点只是分隔符，查询
// D:/Work 命中的原文可能写作 D:\Work，字面是找不到的；
// 取找得到的词元里最长的作锚，避开单字词元的噪声。
func excerpt(text, query string) string {
	low := fold(text)
	at, n := -1, 0
	for _, tok := range search.Segment(query) {
		if i := strings.Index(low, tok); i >= 0 {
			if n0 := utf8.RuneCountInString(tok); n0 > n {
				at, n = len([]rune(low[:i])), n0
			}
		}
	}
	runes := []rune(text)
	if at < 0 {
		if len(runes) > 120 {
			return string(runes[:120]) + "…"
		}
		return text
	}
	from, to := max(0, at-36), min(len(runes), at+n+120)
	out := string(runes[from:to])
	if from > 0 {
		out = "…" + strings.TrimLeft(out, " ")
	}
	if to < len(runes) {
		out = strings.TrimRight(out, " ") + "…"
	}
	return out
}

// fold 逐 rune 小写，保持 rune 序号对齐，与 sessions 包同一做法。
func fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
