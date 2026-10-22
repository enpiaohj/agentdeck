package app

import (
	"sort"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Only confirmed Codex child identities with a present, confirmed main are
// grouped. Unknown roles and missing roots remain independently accessible.
func sessionFamilyRoots(list []sessions.Session) map[string]string {
	byID := make(map[string]sessions.Session)
	for _, s := range list {
		if s.Agent == "codex" {
			byID[s.ID] = s
		}
	}
	roots := make(map[string]string)
	for _, s := range byID {
		if s.Role != "subagent" {
			continue
		}
		root, ok := byID[s.RootSession]
		if ok && root.Role == "main" && root.ID != s.ID {
			roots["codex:"+s.ID] = "codex:" + root.ID
		}
	}
	return roots
}

func applySessionFamilies(rows []SessionView, roots map[string]string) {
	indexes := make(map[string]int, len(rows))
	for i := range rows {
		indexes[rows[i].Key] = i
	}
	for i := range rows {
		key := roots[rows[i].Key]
		root, ok := indexes[key]
		if !ok {
			continue
		}
		rows[i].GroupKey = key
		childTime, _ := time.Parse(time.RFC3339, rows[i].Last)
		rootTime, _ := time.Parse(time.RFC3339, rows[root].Last)
		if childTime.After(rootTime) {
			rows[root].Last = rows[i].Last
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		first, _ := time.Parse(time.RFC3339, rows[i].Last)
		second, _ := time.Parse(time.RFC3339, rows[j].Last)
		if first.Equal(second) {
			return rows[i].GroupKey == "" && rows[j].GroupKey != ""
		}
		return first.After(second)
	})
}
