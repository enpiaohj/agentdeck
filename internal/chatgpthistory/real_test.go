package chatgpthistory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// TestRealExport runs the importer over a real ChatGPT export named by
// AGENTDECK_CHATGPT_EXPORT (skipped when unset). It prints counts only, never
// titles or text, and stores into a temp dir.
func TestRealExport(t *testing.T) {
	src := os.Getenv("AGENTDECK_CHATGPT_EXPORT")
	if src == "" {
		t.Skip("AGENTDECK_CHATGPT_EXPORT 未设置")
	}
	root := filepath.Join(t.TempDir(), "store")
	start := time.Now()
	pre, err := Preview(root, src, "real", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("preview: %d conversations, %d projects, %d images / %.0f MB (%s)", pre.Conversations, len(pre.Projects), pre.Images, float64(pre.ImageBytes)/1e6, time.Since(start).Round(time.Millisecond))
	start = time.Now()
	rep, err := Import(root, src, "real", Options{Images: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("import: %+v (%s)", rep, time.Since(start).Round(time.Millisecond))
	list, err := List(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	withProject, msgs, empty, leaked := 0, 0, 0, 0
	for _, s := range list {
		if s.Cwd != "" {
			withProject++
		}
		msgs += s.MessageCount
		if s.MessageCount == 0 {
			empty++
		}
		tr, err := sessions.TranscriptOf(s)
		if err != nil {
			t.Fatalf("读不出正文: %v", err)
		}
		for _, p := range tr.Parts {
			if strings.ContainsAny(p.Text, "\ue200\ue201\ue202") {
				leaked++
			}
		}
	}
	var stored int
	var storedBytes int64
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Base(filepath.Dir(p)) == assetsDirName {
			stored++
			if info, e := d.Info(); e == nil {
				storedBytes += info.Size()
			}
		}
		return nil
	})
	t.Logf("stored images %d, %.0f MB", stored, float64(storedBytes)/1e6)
	t.Logf("listed %d, in projects %d, messages %d, empty %d, parts still holding markers %d", len(list), withProject, msgs, empty, leaked)
	if leaked != 0 {
		t.Errorf("%d 段正文仍含引用标记", leaked)
	}
	again, err := Import(root, src, "real", Options{Images: true})
	if err != nil || !again.AlreadyImported || again.Conversations != rep.Conversations {
		t.Fatalf("重复导入: %+v %v", again, err)
	}
}
