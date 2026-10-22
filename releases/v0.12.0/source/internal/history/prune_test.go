package history

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

func saveVersions(t *testing.T, s *Store, id string, n int) {
	t.Helper()
	sess := sessions.Session{Agent: "claude", ID: id, Title: id}
	for i := 1; i <= n; i++ {
		tr := sessions.Transcript{Parts: []sessions.Part{{Role: "user", Kind: "text", Text: fmt.Sprintf("%s 第 %d 版", id, i)}}}
		if err := s.Save(sess, tr); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPruneKeepsNewestAndCurrentVersions(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saveVersions(t, s, "busy", 6)
	saveVersions(t, s, "quiet", 2)

	if _, err := s.PrunePreview(0); err == nil {
		t.Fatal("keep < 1 必须被拒绝")
	}
	preview, err := s.PrunePreview(3)
	if err != nil || preview.Sessions != 1 || preview.Versions != 3 || preview.Bytes <= 0 {
		t.Fatalf("预览 = %+v %v", preview, err)
	}
	if st, _ := s.Stats(); st.Versions != 8 {
		t.Fatalf("预览不应改动数据: %+v", st)
	}
	done, err := s.Prune(3)
	if err != nil || done != preview {
		t.Fatalf("清理结果与预览不一致: %+v vs %+v (%v)", done, preview, err)
	}
	st, _ := s.Stats()
	if st.Versions != 5 || st.Sessions != 2 {
		t.Fatalf("清理后 = %+v", st)
	}
	// 当前版本仍可读，且是最新那一版。
	_, tr, found, err := s.Get("claude:busy")
	if err != nil || !found || tr.Parts[0].Text != "busy 第 6 版" {
		t.Fatalf("最新正文丢失: %+v %v %v", tr, found, err)
	}
	// 留下的是最新 3 版。
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM history_versions WHERE key = 'claude:busy'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("busy 剩余版本 = %d %v", n, err)
	}
	// 再清一次没有可清的。
	again, err := s.Prune(3)
	if err != nil || again.Versions != 0 {
		t.Fatalf("重复清理 = %+v %v", again, err)
	}
}

func TestPruneNeverRemovesCurrentVersionEvenWhenOldest(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saveVersions(t, s, "x", 4)
	// 让当前版本指向最旧的那一行：它即使排在 keep 之外也必须保留。
	if _, err := s.db.Exec(`UPDATE history_sessions SET hash = (SELECT hash FROM history_versions WHERE key='claude:x' ORDER BY saved_at, rowid LIMIT 1) WHERE key = 'claude:x'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prune(1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM history_versions v JOIN history_sessions s ON s.key=v.key AND s.hash=v.hash WHERE v.key='claude:x'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("当前版本被清掉了: %d %v", n, err)
	}
}
