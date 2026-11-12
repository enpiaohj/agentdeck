package sessions

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// hermesTranscriptSchema is Hermes's messages table as the transcript reads
// it: what a reply reasoned in (reasoning_content), the tool a result row
// answers for (tool_name) and what the listing hides (display_kind,
// display_metadata).
const hermesTranscriptSchema = `
CREATE TABLE sessions (id TEXT PRIMARY KEY, title TEXT, cwd TEXT, started_at REAL, last_activity_at REAL);
CREATE TABLE messages (
 id INTEGER PRIMARY KEY, session_id TEXT, role TEXT, content TEXT,
 tool_calls TEXT, tool_call_id TEXT, tool_name TEXT, timestamp REAL,
 active INTEGER DEFAULT 1, display_kind TEXT, display_metadata TEXT,
 reasoning TEXT, reasoning_content TEXT
);`

// hermesTranscriptFixture writes a store with one session's messages.
func hermesTranscriptFixture(t *testing.T, sid string, rows ...[]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(hermesTranscriptSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions VALUES (?, 'A Hermes task', '/work/hermes', 1, 2)`, sid); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO messages (id, session_id, role, content, tool_calls, tool_call_id, tool_name, timestamp, active, display_kind, display_metadata, reasoning, reasoning_content) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append([]any{r[0], sid}, r[1:]...)...); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// TestTranscriptHermes: Hermes's rows read as the conversation they are — the
// prompt, what a reply reasoned and answered and the calls it made, and a
// call's result named by the tool it answers for.
func TestTranscriptHermes(t *testing.T) {
	sid := "20261016_090000_abc123"
	path := hermesTranscriptFixture(t, sid,
		[]any{1, "user", "\x00json:" + `[{"type":"text","text":"Switch the model"}]`, "[]", "", "", 10.0, 1, "", "", "", ""},
		[]any{2, "assistant", "\x00json:" + `[{"type":"text","text":"Looking at the config."}]`, `[{"function":{"name":"terminal","arguments":"{\"command\": \"hermes config get model\"}"}}]`, "", "", 11.0, 1, "", "", "thinking older", "Config first, then the model."},
		[]any{3, "tool", `{"output": "minimax-v1"}`, "[]", "call_1", "terminal", 12.0, 1, "", "", "", ""},
		// what the listing hides says nothing here either
		[]any{4, "assistant", `[{"type":"text","text":"Scaffolding"}]`, "[]", "", "", 13.0, 1, "", `{"model_only": true}`, "", ""},
		// a row a compaction replaced
		[]any{5, "assistant", `[{"type":"text","text":"Replaced"}]`, "[]", "", "", 14.0, 0, "", "", "", ""},
	)

	s := Session{Agent: "hermes", ID: hermesID(path, sid), Path: path + "#" + sid}
	if !HasTranscript(s.Agent) {
		t.Fatal("a Hermes session's conversation is not read")
	}
	tr, err := TranscriptOf(s)
	if err != nil {
		t.Fatal(err)
	}
	want := []Part{
		{Role: "user", Kind: "text", Text: "Switch the model"},
		{Role: "assistant", Kind: "thinking", Text: "Config first, then the model."},
		{Role: "assistant", Kind: "text", Text: "Looking at the config."},
		{Role: "assistant", Kind: "tool_use", Name: "terminal", Text: "{\n  \"command\": \"hermes config get model\"\n}"},
		{Role: "tool", Kind: "tool_result", Name: "terminal", Text: `{"output": "minimax-v1"}`},
	}
	if len(tr.Parts) != len(want) {
		t.Fatalf("parts=%d, want %d: %+v", len(tr.Parts), len(want), tr.Parts)
	}
	for i, p := range want {
		if tr.Parts[i] != p {
			t.Fatalf("part %d: %+v, want %+v", i, tr.Parts[i], p)
		}
	}
}

// TestTranscriptHermesBadPath: a path that names no store, and a store that
// is not there, are conversations that cannot be shown, not empty ones.
func TestTranscriptHermesBadPath(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "state.db") + "#sid"
	for _, path := range []string{"", "state.db", "state.db#", "#sid", gone} {
		s := Session{Agent: "hermes", ID: "h-1", Path: path}
		if _, err := TranscriptOf(s); err != ErrNoTranscript {
			t.Fatalf("path %q: error %v, want ErrNoTranscript", path, err)
		}
	}
}
