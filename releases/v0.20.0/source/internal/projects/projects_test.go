package projects

import (
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func sess(agent, id, cwd string, last time.Time) sessions.Session {
	return sessions.Session{Agent: agent, ID: id, Cwd: cwd, Last: last}
}

// 三种路径表示（侦察结论 #4 的残余形态）归到同一项目：扩展长度前缀、
// 大小写、尾分隔符差异不拆组。
func TestGroupConvergesRepresentations(t *testing.T) {
	at := func(mins int) time.Time {
		return time.Date(2026, 10, 16, 10, 0, 0, 0, time.Local).Add(time.Duration(mins) * time.Minute)
	}
	list := []sessions.Session{
		sess("codex", "c1", `\\?\D:\Work\ExampleApp`, at(5)),
		sess("claude", "k1", `D:\work\exampleapp\`, at(10)),
		sess("zcode", "z1", `D:\Work\ExampleApp`, at(1)),
		sess("codex", "c2", `D:\Work\Notes`, at(3)),
		sess("claude", "k2", "", at(9)), // 无 cwd，不属于任何项目
	}
	ps := Group(list)
	if len(ps) != 2 {
		t.Fatalf("聚出 %d 个项目, want 2: %+v", len(ps), ps)
	}
	// 最新活动的组在前
	if ps[0].Path != `D:\work\exampleapp` && ps[0].Path != `D:\Work\ExampleApp` {
		t.Errorf("代表路径 = %q", ps[0].Path)
	}
	if ps[0].Name != "ExampleApp" || ps[0].Drive != "D" {
		t.Errorf("Name/Drive = %q/%q", ps[0].Name, ps[0].Drive)
	}
	if len(ps[0].Keys) != 3 {
		t.Fatalf("组内会话 = %v, want 3", ps[0].Keys)
	}
	// 组内按 Last 降序：k1(10) → c1(5) → z1(1)
	if ps[0].Keys[0] != "claude:k1" || ps[0].Keys[1] != "codex:c1" || ps[0].Keys[2] != "zcode:z1" {
		t.Fatalf("Keys 顺序 = %v", ps[0].Keys)
	}
	if !ps[0].Last.Equal(at(10)) {
		t.Errorf("组最新活动 = %v, want %v", ps[0].Last, at(10))
	}
	if len(ps[1].Keys) != 1 || ps[1].Keys[0] != "codex:c2" || ps[1].Name != "Notes" {
		t.Fatalf("第二组: %+v", ps[1])
	}
}

// 盘符根是自己的展示名。
func TestRootPathName(t *testing.T) {
	if got := Normalize(`\\?\D:\`); got != `D:\` {
		t.Errorf("Normalize = %q", got)
	}
	p := Group([]sessions.Session{sess("codex", "c1", `D:\`, time.Now())})
	if len(p) != 1 || p[0].Name != `D:\` || p[0].Drive != "D" {
		t.Fatalf("root: %+v", p)
	}
}

// Normalize 是比较键的来源：前缀与空白剥掉，Clean 收尾。
func TestNormalize(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`  D:\a\b  `, `D:\a\b`},
		{`\\?\D:\a\b`, `D:\a\b`},
		{`D:\a\b\`, `D:\a\b`},
		{``, ``},
		{`   `, ``},
	} {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
