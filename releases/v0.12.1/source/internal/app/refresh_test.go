package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestRefreshDiscoversNewSessionsAndPreservesMarks(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	sessions.Reset()
	t.Cleanup(sessions.Reset)
	dir := filepath.Join(root, "projects", "fixture")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	claudeFixture(t, dir, "first", "First session")
	a, rec, _, _ := newTestApp(t)
	svc := a.NewSessionService()
	if got := svc.List(Filter{}); len(got) != 1 {
		t.Fatalf("initial list: %d", len(got))
	}
	if err := a.NewMarkService().SetFavorite("claude:first", true); err != nil {
		t.Fatal(err)
	}
	claudeFixture(t, dir, "second", "New session")
	svc.Refresh()
	rows := svc.List(Filter{})
	if len(rows) != 2 {
		t.Fatalf("refresh still serves stale cache: %d", len(rows))
	}
	if fav := svc.List(Filter{Favorites: true}); len(fav) != 1 || fav[0].ID != "first" {
		t.Fatalf("marks changed: %+v", fav)
	}
	if !rec.has(EventSessionsChanged) {
		t.Fatal("refresh did not notify views")
	}
}

func TestRefreshDoesNotSearchStaleIndex(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	dir := t.TempDir()
	first := claudeFixture(t, dir, "first", "Original searchable content")
	a.setList([]sessions.Session{first})
	a.buildIndex(true)
	search := a.NewSearchService()
	if got, err := search.Query("Original", 20); err != nil || got.Mode != "index" || len(got.Hits) != 1 {
		t.Fatalf("initial query: %+v %v", got, err)
	}
	// A refreshed generation must use current sources until its own index is ready.
	a.mu.Lock()
	a.generation++
	a.mu.Unlock()
	if got, err := search.Query("Original", 20); err != nil || got.Mode != "scan" {
		t.Fatalf("stale index used: %+v %v", got, err)
	}
}

func TestDesktopReclassificationKeepsExistingMarks(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	dir := filepath.Join(root, "projects", "fixture")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sess := claudeFixture(t, dir, "shared", "Desktop shared transcript")
	a, _, _, _ := newTestApp(t)
	a.setList([]sessions.Session{sess})
	mk := a.NewMarkService()
	if err := mk.SetFavorite("claude:shared", true); err != nil {
		t.Fatal(err)
	}
	if err := mk.SetArchived("claude:shared", true); err != nil {
		t.Fatal(err)
	}
	if err := mk.SetTags("claude:shared", []string{"keep"}); err != nil {
		t.Fatal(err)
	}
	if err := mk.SetNote("claude:shared", "Existing note"); err != nil {
		t.Fatal(err)
	}
	// Discovery now proves this shared source belongs to Desktop.
	sess.Agent = "claude-desktop"
	a.mu.Lock()
	a.list = []sessions.Session{sess}
	a.byKey = map[string]sessions.Session{"claude-desktop:shared": sess}
	a.mu.Unlock()
	svc := a.NewSessionService()
	rows := svc.List(Filter{Archived: true, Favorites: true})
	if len(rows) != 1 || rows[0].Note != "Existing note" || len(rows[0].Tags) != 1 {
		t.Fatalf("lost marks: %+v", rows)
	}
	if err := mk.SetNote("claude-desktop:shared", "Updated note"); err != nil {
		t.Fatal(err)
	}
	mark, err := mk.Get("claude-desktop:shared")
	if err != nil || !mark.Archived || !mark.Favorite || mark.Note != "Updated note" || len(mark.Tags) != 1 {
		t.Fatalf("lost marks after edit: %+v %v", mark, err)
	}
}

func TestRebuildRequestedDuringBuildIsNotLost(t *testing.T) {
	a, rec, _, _ := newTestApp(t)
	a.setList([]sessions.Session{claudeFixture(t, t.TempDir(), "first", "Queued rebuild content")})
	// Hold the snapshot barrier so a second request reliably arrives while
	// the first build is running, rather than relying on a large fixture.
	a.mu.Lock()
	var release sync.Once
	unlock := func() { release.Do(a.mu.Unlock) }
	t.Cleanup(unlock)
	go a.buildIndex(true)
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !check() {
			if time.Now().After(deadline) {
				t.Fatal("index rebuild timed out")
			}
			time.Sleep(time.Millisecond)
		}
	}
	wait(func() bool { return a.search.status().State == "building" })
	a.NewSearchService().Rebuild()
	wait(func() bool { a.search.mu.Lock(); defer a.search.mu.Unlock(); return a.search.pending })
	a.generation++
	unlock()
	wait(func() bool {
		rec.mu.Lock()
		ready := 0
		for _, event := range rec.events {
			if event == EventIndexReady {
				ready++
			}
		}
		rec.mu.Unlock()
		a.search.mu.Lock()
		defer a.search.mu.Unlock()
		return ready >= 2 && !a.search.building && a.search.state == "ready" && a.search.generation == 1
	})
	if got, err := a.NewSearchService().Query("Queued", 20); err != nil || got.Mode != "index" || len(got.Hits) != 1 {
		t.Fatalf("queued rebuild did not publish a searchable index: %+v %v", got, err)
	}
}

func TestSearchDoesNotReturnOrphanedIndexRows(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	a.setList([]sessions.Session{claudeFixture(t, t.TempDir(), "removed", "Orphaned indexed content")})
	a.buildIndex(true)
	a.setList(nil)
	got, err := a.NewSearchService().Query("Orphaned", 20)
	if err != nil || len(got.Hits) != 0 {
		t.Fatalf("search returned a row with no session: %+v %v", got, err)
	}
}
