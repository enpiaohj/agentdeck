package app

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// benchApp 装 n 条合成会话：约四分之一是 Codex 子会话，分布在 40 个项目里，
// 规模对齐本机实测（约 700 条）并放大到 3 倍看趋势。
func benchApp(b *testing.B, n int) *App {
	b.Helper()
	a, err := New(Config{
		MarksPath:    b.TempDir() + `\marks.db`,
		ProjectsPath: b.TempDir() + `\projects.db`,
		IndexPath:    ":memory:",
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(a.Close)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	list := make([]sessions.Session, 0, n)
	for i := 0; i < n; i++ {
		s := sessions.Session{
			Agent: []string{"codex", "claude", "claude-desktop", "hermes"}[i%4],
			ID:    fmt.Sprintf("%08d-0000-4000-8000-000000000000", i),
			Cwd:   fmt.Sprintf(`D:\Work\Project%02d`, i%40),
			Title: fmt.Sprintf("会话标题 %d", i),
			Last:  base.Add(-time.Duration(i) * time.Minute),
		}
		if s.Agent == "codex" {
			s.Role = "main"
			if i%8 == 0 && i > 0 { // 子会话挂到前一个主会话
				s.Role, s.RootSession = "subagent", list[i-4].ID
			}
		}
		list = append(list, s)
	}
	a.setList(list)
	return a
}

func BenchmarkSidebarRefreshAfterMark(b *testing.B) {
	for _, n := range []int{700, 2100} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			a := benchApp(b, n)
			svc, marks := a.NewSessionService(), a.NewMarkService()
			key := "claude:" + a.list[1].ID
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = marks.SetFavorite(key, i%2 == 0)
				svc.List(Filter{})
				_, _ = svc.Get(key)
				svc.Agents()
				svc.Projects()
				svc.Counts()
			}
		})
	}
}

func BenchmarkList(b *testing.B) {
	a := benchApp(b, 700)
	svc := a.NewSessionService()
	for i := 0; i < b.N; i++ {
		svc.List(Filter{})
	}
}

func BenchmarkProjects(b *testing.B) {
	a := benchApp(b, 700)
	svc := a.NewSessionService()
	for i := 0; i < b.N; i++ {
		svc.Projects()
	}
}

func BenchmarkListPayload(b *testing.B) {
	a := benchApp(b, 700)
	rows := a.NewSessionService().List(Filter{})
	data, _ := json.Marshal(rows)
	b.ReportMetric(float64(len(data))/1024, "KB/payload")
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(rows)
	}
}
