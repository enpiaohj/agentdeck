// Command agentdeck 是 AgentDeck 的命令行入口。
//
// 当前处于 Phase 0：会话读取基线。本命令列出本机各 Agent 的历史会话，
// 用于验证会话层的发现、解析与恢复命令构造是否正确。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"syscall"
	"time"

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
		jsonOut = flag.Bool("json", false, "以 JSON 输出")
		limit   = flag.Int("limit", 20, "最多列出多少条（0 表示全部）")
		agent   = flag.String("agent", "", "只看某个 agent，如 claude、claude-desktop、codex")
		all     = flag.Bool("all", false, "列出全部会话（等同 -limit 0）")
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
