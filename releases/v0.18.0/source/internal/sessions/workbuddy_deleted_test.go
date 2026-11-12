package sessions

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// AgentDeck: a session WorkBuddy deleted (deleted_at set in workbuddy.db,
// its file left in projects/) is still listed and readable, flagged deleted
// upstream; the others are not flagged, and the database is not written.
func TestWorkBuddyUpstreamDeleted(t *testing.T) {
	inZone(t, 0)
	home := setupAgents(t)
	dbPath := filepath.Join(home, ".workbuddy", "workbuddy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (id TEXT PRIMARY KEY, deleted_at INTEGER);
		INSERT INTO sessions VALUES ('` + wbID + `', 1789748562480), ('` + wbOldID + `', NULL)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	Reset()
	ss := List(0)
	gone := find(t, ss, "workbuddy", wbID)
	if !gone.UpstreamDeleted {
		t.Fatalf("deleted upstream, not flagged: %+v", gone)
	}
	if tr, err := TranscriptOf(gone); err != nil || len(tr.Parts) == 0 {
		t.Fatalf("a session deleted upstream must stay readable: %v", err)
	}
	if kept := find(t, ss, "workbuddy", wbOldID); kept.UpstreamDeleted {
		t.Fatalf("not deleted, flagged: %+v", kept)
	}
	for _, s := range ss {
		if s.Agent != "workbuddy" && s.UpstreamDeleted {
			t.Errorf("%s:%s flagged", s.Agent, s.ID)
		}
	}
	if after, _ := os.ReadFile(dbPath); string(after) != string(before) {
		t.Error("workbuddy.db was written")
	}

	// a folder without the database, or with another schema, flags nothing
	os.Remove(dbPath)
	Reset()
	if s := find(t, List(0), "workbuddy", wbID); s.UpstreamDeleted {
		t.Fatalf("no database, flagged: %+v", s)
	}
}
