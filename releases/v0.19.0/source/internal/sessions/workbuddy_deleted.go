package sessions

import (
	"database/sql"
	"net/url"
	"path/filepath"
)

// AgentDeck: WorkBuddy deletes a session by setting deleted_at on its row in
// workbuddy.db and leaves the session's file in projects/. AgentDeck keeps
// listing it (an upstream deletion never takes a session away), flagged as
// deleted upstream so the list can say so.

// workbuddyDeleted are the ids workbuddy.db in dir marks deleted. A folder
// without the database, or one without the column, says nothing.
func workbuddyDeleted(dir string) map[string]bool {
	out := map[string]bool{}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, "workbuddy.db")), RawQuery: "mode=ro&_pragma=busy_timeout(2000)"}
	if len(u.Path) >= 2 && u.Path[1] == ':' {
		u.Path = "/" + u.Path
	}
	if !stat(&file{path: filepath.Join(dir, "workbuddy.db")}) {
		return out
	}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return out
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id FROM sessions WHERE deleted_at IS NOT NULL`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}

// markUpstreamDeleted flags the listed sessions their agent has deleted.
func markUpstreamDeleted(ss []Session) {
	deleted := map[string]map[string]bool{}
	for i := range ss {
		s := &ss[i]
		if !isWorkBuddy(s.Agent) || s.WSL != "" {
			continue
		}
		ids, ok := deleted[s.Agent]
		if !ok {
			ids = workbuddyDeleted(buddyDir(s.Agent))
			deleted[s.Agent] = ids
		}
		s.UpstreamDeleted = ids[s.ID]
	}
}
