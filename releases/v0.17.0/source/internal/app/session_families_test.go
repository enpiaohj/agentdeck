package app

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestSessionFamiliesKeepEighteenRecordsButCountFiveConversations(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	list := []sessions.Session{}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("main-%d", i)
		list = append(list, sessions.Session{Agent: "codex", ID: id, Role: "main", RootSession: id, Cwd: `D:\Demo`, Last: time.Unix(int64(i), 0)})
	}
	for i := 0; i < 13; i++ {
		parent := "main-0"
		if i == 12 {
			parent = "child-0"
		}
		list = append(list, sessions.Session{Agent: "codex", ID: fmt.Sprintf("child-%d", i), Role: "subagent", ParentSession: parent, RootSession: "main-0", Cwd: `D:\Demo`, Last: time.Unix(int64(100+i), 0)})
	}
	a.setList(list)
	svc := a.NewSessionService()
	rows := svc.List(Filter{})
	if len(rows) != 18 {
		t.Fatalf("Grouping must preserve all records, got %d", len(rows))
	}
	data, _ := json.Marshal(rows)
	var dto []map[string]any
	_ = json.Unmarshal(data, &dto)
	children := 0
	for _, row := range dto {
		if row["role"] == "subagent" {
			children++
			if row["group_key"] != "codex:main-0" {
				t.Fatalf("Child lacks confirmed root: %v", row)
			}
		}
		if row["key"] == "codex:main-0" && row["last"] != lastString(time.Unix(112, 0)) {
			t.Fatal("Main conversation must reflect its family's latest activity")
		}
	}
	if children != 13 {
		t.Fatalf("Missing child identities: %d", children)
	}
	if agents := svc.Agents(); len(agents) != 1 || agents[0].Count != 5 {
		t.Fatalf("Agent count must exclude grouped children: %+v", agents)
	}
	if projects := svc.Projects(); len(projects) != 1 || projects[0].Count != 5 {
		t.Fatalf("Project count must exclude grouped children: %+v", projects)
	}
	if err := a.NewMarkService().SetFavorite("codex:child-12", true); err != nil {
		t.Fatal(err)
	}
	favorites := svc.List(Filter{Favorites: true})
	if len(favorites) != 1 || favorites[0].Key != "codex:child-12" || favorites[0].GroupKey != "" {
		t.Fatal("A child must remain accessible when its main is filtered out")
	}
}

func TestSessionFamiliesDoNotMergeUnknownOrOrphanRecords(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.setList([]sessions.Session{
		{Agent: "codex", ID: "main", Role: "main", RootSession: "main"},
		{Agent: "codex", ID: "orphan", Role: "subagent", ParentSession: "missing"},
		{Agent: "codex", ID: "unknown", Role: "unknown", RootSession: "main"},
		{Agent: "claude", ID: "main"},
	})
	counts := map[string]int{}
	for _, agent := range a.NewSessionService().Agents() {
		counts[agent.Agent] = agent.Count
	}
	if counts["codex"] != 3 || counts["claude"] != 1 {
		t.Fatalf("Unknown/orphan identities must not be hidden: %v", counts)
	}
}

func TestSessionFamiliesActivityUsesActualTimeAndStartsWithMain(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	rootTime, _ := time.Parse(time.RFC3339, "2026-10-16T23:30:00-04:00")
	childTime, _ := time.Parse(time.RFC3339, "2026-10-17T12:00:00+09:00")
	a.setList([]sessions.Session{
		{Agent: "codex", ID: "child", Role: "subagent", ParentSession: "main", RootSession: "main", Last: childTime},
		{Agent: "codex", ID: "main", Role: "main", RootSession: "main", Last: rootTime},
	})
	rows := a.NewSessionService().List(Filter{})
	if rows[0].Key != "codex:main" || rows[0].Last != lastString(rootTime) {
		t.Fatalf("Older child must not replace the main's newer activity: %+v", rows)
	}
	childTime = rootTime.Add(time.Hour)
	a.setList([]sessions.Session{
		{Agent: "codex", ID: "child", Role: "subagent", RootSession: "main", Last: childTime},
		{Agent: "codex", ID: "main", Role: "main", RootSession: "main", Last: rootTime},
	})
	rows = a.NewSessionService().List(Filter{})
	if rows[0].Key != "codex:main" || rows[0].Last != lastString(childTime) {
		t.Fatalf("Startup must select the main even when its child was most recent: %+v", rows)
	}
}
