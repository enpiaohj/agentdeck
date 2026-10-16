package sessions

import "testing"

// TestTranscriptWorkBuddy: WorkBuddy's items read as the conversation they
// are — the words the user typed, and each reply's words, calls and their
// results — in the order its file has them.
func TestTranscriptWorkBuddy(t *testing.T) {
	inZone(t, 0)
	setupAgents(t)
	s := find(t, List(All), "workbuddy", "22222222-aaaa-4bbb-8ccc-000000000001")
	if !s.Transcript || !HasTranscript("workbuddy") {
		t.Fatal("a WorkBuddy session's conversation is not read")
	}
	tr, err := TranscriptOf(s)
	if err != nil {
		t.Fatal(err)
	}
	want := []Part{
		{Role: "user", Kind: "text", Text: "Draft the   launch notes"},
		{Role: "assistant", Kind: "text", Text: "Reading the notes.", Model: "hy3"},
		{Role: "assistant", Kind: "tool_use", Name: "Read", Text: "{}", Model: "hy3"},
		{Role: "assistant", Kind: "tool_use", Name: "Read", Text: "{}", Model: "hy3"},
		{Role: "tool", Kind: "tool_result", Name: "Read", Text: "notes"},
		{Role: "assistant", Kind: "text", Text: "Here they are.", Model: "hy3"},
		{Role: "assistant", Kind: "tool_use", Name: "Write", Text: "{}", Model: "hy3"},
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

// TestTranscriptWorkBuddyQuery: the words are what WorkBuddy hands over at the
// end of the reminder it wraps them in (<user_query>), and a message that is
// only its own text says nothing.
func TestTranscriptWorkBuddyQuery(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"<system-reminder>\n<user_info>OS: darwin</user_info>\n</system-reminder>\n<user_query>你是谁</user_query>", "你是谁"},
		{"<user_query>Fix the parser</user_query>", "Fix the parser"},
		{"<system-reminder data-role=\"user-context\"></system-reminder>", ""},
		{"@scene:4", ""},
		{"   ", ""},
		{"run the tests", "run the tests"},
	} {
		if got := wbSaid(c.in); got != c.want {
			t.Fatalf("wbSaid(%.30q)=%q, want %q", c.in, got, c.want)
		}
	}
}
