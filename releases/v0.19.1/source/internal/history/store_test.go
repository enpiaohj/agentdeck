package history

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func TestVersionsDeduplicateAndNeverExpire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library", "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sess := sessions.Session{Agent: "codex", ID: "owned", Title: "历史记录"}
	first := sessions.Transcript{Parts: []sessions.Part{{Role: "user", Kind: "text", Text: "第一版"}}}
	second := sessions.Transcript{Parts: []sessions.Part{{Role: "user", Kind: "text", Text: "后续内容"}}}
	for _, tr := range []sessions.Transcript{first, first, second, {Parts: []sessions.Part{}}} {
		if err := s.Save(sess, tr); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := s.Stats()
	if err != nil || stats.Sessions != 1 || stats.Versions != 2 || stats.Bytes == 0 {
		t.Fatalf("去重和版本保留: %+v %v", stats, err)
	}
	var original string
	if err := s.db.QueryRow(`SELECT transcript FROM history_versions ORDER BY saved_at LIMIT 1`).Scan(&original); err != nil || original == "" {
		t.Fatalf("旧版丢失: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, tr, found, err := s.Get("codex:owned")
	if err != nil || !found || tr.Parts[0].Text != "后续内容" {
		t.Fatalf("持久化最新正文: %+v %v", tr, err)
	}
	stats, _ = s.Stats()
	if stats.Versions != 2 {
		t.Fatal("重开数据库不应清理旧版")
	}
}

func TestConcurrentCaptureKeepsDistinctVersionsAndAccounts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sess := sessions.Session{Agent: "claude", ID: "work", Account: "工作"}
			if i%2 == 0 {
				sess.ID, sess.Account = "personal", "个人"
			}
			if err := s.Save(sess, sessions.Transcript{Parts: []sessions.Part{{Role: "user", Kind: "text", Text: "相同正文"}}}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	stats, err := s.Stats()
	if err != nil || stats.Sessions != 2 || stats.Versions != 2 {
		t.Fatalf("并发去重/账号隔离: %+v %v", stats, err)
	}
}
