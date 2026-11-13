package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTranscriptZCode: ZCode keeps a message's id in the column of that name
// rather than in the document as OpenCode does, so a session's parts are only
// read when the row's id is read with it (zcDB.messages). The conversation is
// the same whichever reader asked: the listing's and the transcript's.
func TestTranscriptZCode(t *testing.T) {
	inZone(t, 0)
	setupAgents(t)
	s := find(t, List(All), "zcode", "ses_zcA")
	if !s.Transcript || !HasTranscript("zcode") {
		t.Fatal("a ZCode session's conversation is not read")
	}
	tr, err := TranscriptOf(s)
	if err != nil {
		t.Fatal(err)
	}
	want := []Part{
		{Role: "user", Kind: "text", Text: "Remember the house rules"},
		{Role: "user", Kind: "text", Text: "Port the   parser"},
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

// TestTranscriptDesktop: Desktop's Code and Cowork sessions are Claude Code's
// own files, in Desktop's home, and read as Claude Code's are.
func TestTranscriptDesktop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	lines := `{"type":"user","message":{"role":"user","content":"Open the report"}}` + "\n" +
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"Reading it first."},{"type":"text","text":"Opening it."},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a"}}]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"the report"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if !HasTranscript("claude-desktop") {
		t.Fatal("a Desktop session's conversation is not read")
	}
	tr, err := TranscriptOf(Session{Agent: "claude-desktop", ID: "sess", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	want := []Part{
		{Role: "user", Kind: "text", Text: "Open the report"},
		{Role: "assistant", Kind: "thinking", Text: "Reading it first.", Model: "claude-opus-5-5"},
		{Role: "assistant", Kind: "text", Text: "Opening it.", Model: "claude-opus-5-5"},
		{Role: "assistant", Kind: "tool_use", Name: "Read", Text: "{\n  \"file_path\": \"/a\"\n}", Model: "claude-opus-5-5"},
		{Role: "tool", Kind: "tool_result", Text: "the report"},
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
