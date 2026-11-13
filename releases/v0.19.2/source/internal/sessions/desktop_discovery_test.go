package sessions

import (
	"path/filepath"
	"testing"
)

// AgentDeck must discover Code transcripts as well as older Cowork files,
// and use Desktop's explicit CLI IDs when it shares the CLI projects folder.
func TestDesktopCodeSessionDiscovery(t *testing.T) {
	d := setupCalls(t)
	writeLines(t, filepath.Join(d.desktop, "claude-code-sessions", "account", "org", "local_new", ".claude", "projects", "fixture", "new.jsonl"), `{"type":"user","timestamp":"2026-10-10T10:00:00Z","message":{"role":"user","content":"October Desktop Code"}}`)
	writeDesktopTitle(t, filepath.Join(d.desktop, "claude-code-sessions", "account", "org", "local_shared.json"), `{"sessionId":"local_shared","cliSessionId":"shared","priorCliSessionIds":["prior"],"title":"Desktop project"}`)
	for _, id := range []string{"shared", "prior", "cli"} {
		writeLines(t, filepath.Join(d.claude, "projects", "fixture", id+".jsonl"), `{"type":"user","timestamp":"2026-09-10T10:00:00Z","message":{"role":"user","content":"September conversation"}}`)
	}
	list := List(All)
	got := map[string]Session{}
	for _, s := range list {
		got[s.Agent+":"+s.ID] = s
	}
	for _, key := range []string{"claude-desktop:new", "claude-desktop:shared", "claude-desktop:prior", "claude:cli"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing %s (got %v)", key, got)
		}
	}
	if len(list) != 4 {
		t.Fatalf("duplicate or missing sessions: %d", len(list))
	}
	if got["claude-desktop:shared"].Title != "Desktop project" {
		t.Error("Desktop title was not applied")
	}
	if managed := ListAgent("claude-desktop"); len(managed) != 3 {
		t.Errorf("Desktop count disagrees: %d", len(managed))
	}
}
