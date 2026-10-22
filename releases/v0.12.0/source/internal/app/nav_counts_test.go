package app

import (
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// 侧栏三个数字必须与「点进去能看到几行」一致，口径与 agent / 项目清单相同：
// 子会话收在主会话下不重复计，已归档的会话要走到「归档」那一栏才看得到。
// 因此归档会话既不算会话也不算收藏，但项目行仍单独报一个归档数（悬停提示
// 用来说明「还有几条收在归档里」）。
func TestNavCountsMatchWhatEachEntryLists(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.setList([]sessions.Session{
		{Agent: "codex", ID: "m1", Role: "main", RootSession: "m1", Cwd: `D:\Demo`, Last: time.Unix(1, 0)},
		{Agent: "codex", ID: "c1", Role: "subagent", ParentSession: "m1", RootSession: "m1", Cwd: `D:\Demo`, Last: time.Unix(2, 0)},
		{Agent: "codex", ID: "m2", Role: "main", RootSession: "m2", Cwd: `D:\Demo`, Last: time.Unix(3, 0)},
		{Agent: "claude", ID: "a1", Role: "main", RootSession: "a1", Cwd: `D:\Demo`, Last: time.Unix(4, 0)},
		{Agent: "claude", ID: "a2", Role: "main", RootSession: "a2", Cwd: `D:\Demo`, Last: time.Unix(5, 0)},
	})
	ms := a.NewMarkService()
	if err := ms.SetFavorite("codex:m2", true); err != nil {
		t.Fatal(err)
	}
	// 被收藏的子会话仍能在「收藏」里点开（它不会跟着主会话被过滤掉），但不
	// 抬高数字：默认视图里它收在 m1 名下，不算一行。
	if err := ms.SetFavorite("codex:c1", true); err != nil {
		t.Fatal(err)
	}
	if err := ms.SetArchived("claude:a1", true); err != nil {
		t.Fatal(err)
	}
	// 归档且收藏的会话只出现在「归档」里，收藏数不该把它算进去。
	if err := ms.SetFavorite("claude:a2", true); err != nil {
		t.Fatal(err)
	}
	if err := ms.SetArchived("claude:a2", true); err != nil {
		t.Fatal(err)
	}

	svc := a.NewSessionService()
	if got := svc.Counts(); got != (NavCounts{Sessions: 2, Favorites: 1, Archived: 2}) {
		t.Fatalf("侧栏数字与列表口径不符: %+v", got)
	}
	if rows := svc.List(Filter{}); len(rows) != 3 || rootsOf(rows) != 2 {
		t.Fatalf("默认视图应收起子会话剩 2 行（已归档的 2 条不在其中）: %+v", rows)
	}
	if rows := svc.List(Filter{Archived: true}); len(rows) != 2 {
		t.Fatalf("「归档」应列出 2 行: %+v", rows)
	}
	// 「收藏」列表里 codex:c1 单独成行（它的主会话没被收藏、已被过滤掉），
	// 所以列表长度比数字多一行——数字数的是收起后的行，而「收藏」是刻意给
	// 子会话留的出口，不跟着主会话被过滤掉。
	if rows := svc.List(Filter{Favorites: true}); len(rows) != 2 {
		t.Fatalf("「收藏」应列出 m2 与单独成行的子会话 c1: %+v", rows)
	}

	agents := svc.Agents()
	if len(agents) != 1 || agents[0].Agent != "codex" || agents[0].Count != 2 {
		t.Fatalf("归档会话不应留在 agent 清单里: %+v", agents)
	}
	projects := svc.Projects()
	if len(projects) != 1 || projects[0].Count != 2 || projects[0].Favorites != 1 || projects[0].Archived != 2 {
		t.Fatalf("项目行要把归档数单独报出来: %+v", projects)
	}
}

// rootsOf 数出收起子会话后剩下的行：主会话在列表里时子会话挂到它名下。
func rootsOf(rows []SessionView) int {
	present := map[string]bool{}
	for _, r := range rows {
		present[r.Key] = true
	}
	n := 0
	for _, r := range rows {
		if r.GroupKey != "" && present[r.GroupKey] {
			continue
		}
		n++
	}
	return n
}
