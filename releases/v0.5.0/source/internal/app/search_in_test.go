package app

import (
	"os"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestSearchInAllKeepsOriginalPartIndexes(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	sess := claudeFixture(t, t.TempDir(), "find-many", "unused")
	data := `{"type":"user","message":{"role":"user","content":"命中 first 命中"},"uuid":"u1"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"middle"}]},"uuid":"a1"}
{"type":"user","message":{"role":"user","content":"命中 last"},"uuid":"u2"}
`
	if err := os.WriteFile(sess.Path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	a.setList([]sessions.Session{sess})
	svc := a.NewSessionService()
	hits, err := svc.SearchInAll("claude:find-many", "命中")
	if err != nil || len(hits) != 2 {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
	for _, hit := range hits {
		key, index, ok := splitDocKey(hit.Part)
		if !ok || key != hit.Key {
			t.Fatalf("invalid part %+v", hit)
		}
		window, err := svc.TranscriptWindow(key, index, false)
		if err != nil || window.Parts[index-window.Start].Text == "middle" {
			t.Fatalf("wrong location %+v %v", window, err)
		}
	}
	first, err := svc.SearchIn("claude:find-many", "命中")
	if err != nil || first.Part != hits[0].Part {
		t.Fatalf("first=%+v %v", first, err)
	}
	empty, err := svc.SearchInAll("claude:find-many", " ")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty=%+v %v", empty, err)
	}
}
