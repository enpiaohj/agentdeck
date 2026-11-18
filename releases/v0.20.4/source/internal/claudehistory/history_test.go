package claudehistory

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const exportFixture = `[{"uuid":"same-cloud-id","name":"十月份项目进度","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-10-17T10:00:00Z","chat_messages":[{"sender":"human","text":"核对十月份的进度","created_at":"2026-10-16T10:00:00Z"},{"sender":"assistant","text":"旧文本不应重复","content":[{"type":"text","text":"这里是完整的回复"}],"created_at":"2026-10-17T10:00:00Z"}]}]`

func source(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "conversations.json")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportAccountsSearchableAndIdempotent(t *testing.T) {
	root := t.TempDir()
	p := source(t, exportFixture)
	for _, account := range []string{"工作账号", "个人账号"} {
		r, err := Import(root, p, account)
		if err != nil || r.Conversations != 1 {
			t.Fatalf("import: %+v %v", r, err)
		}
	}
	r, err := Import(root, p, "工作账号")
	if err != nil || !r.AlreadyImported {
		t.Fatalf("repeat import: %+v %v", r, err)
	}
	rows, err := List(root)
	if err != nil || len(rows) != 2 {
		t.Fatalf("account identities merged or duplicated: %+v %v", rows, err)
	}
	if rows[0].ID == rows[1].ID || !rows[0].Imported || rows[0].Account == "" || rows[0].Resume != "" || !rows[0].ReadOnly || rows[0].MessageCount != 2 {
		t.Fatalf("bad imported capabilities: %+v", rows)
	}
	tr, err := sessions.TranscriptOf(rows[0])
	if err != nil || len(tr.Parts) != 2 || tr.Parts[1].Text != "这里是完整的回复" {
		t.Fatalf("preview differs: %+v %v", tr, err)
	}
}

func TestZipImportAndMalformedBatchAreAtomic(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(t.TempDir(), "claude-export.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("export/conversations.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(exportFixture)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(root, p, "账号一"); err != nil {
		t.Fatal(err)
	}
	bad := source(t, exportFixture[:len(exportFixture)-1]+`, {"uuid":"bad"}]`)
	if _, err := Import(root, bad, "账号二"); err == nil {
		t.Fatal("unsupported record accepted")
	}
	rows, err := List(root)
	if err != nil || len(rows) != 1 {
		t.Fatalf("partial batch leaked: %d %v", len(rows), err)
	}
	if _, err := Import(root, p, " "); err == nil {
		t.Fatal("empty account accepted")
	}
}

func TestOlderExportCannotReplaceNewerConversation(t *testing.T) {
	root := t.TempDir()
	latest := source(t, exportFixture)
	if _, err := Import(root, latest, "Account"); err != nil {
		t.Fatal(err)
	}
	older := strings.ReplaceAll(exportFixture, "2026-10-17", "2026-09-17")
	older = strings.ReplaceAll(older, "2026-10-16", "2026-09-16")
	older = strings.ReplaceAll(older, "这里是完整的回复", "旧版回复")
	if _, err := Import(root, source(t, older), "account"); err != nil {
		t.Fatal(err)
	}
	rows, err := List(root)
	if err != nil || len(rows) != 1 {
		t.Fatalf("stable account identity failed: %d %v", len(rows), err)
	}
	tr, err := sessions.TranscriptOf(rows[0])
	if err != nil || tr.Parts[1].Text != "这里是完整的回复" {
		t.Fatalf("older export replaced newer content: %+v %v", tr, err)
	}
	after, err := os.ReadFile(latest)
	if err != nil || string(after) != exportFixture {
		t.Fatalf("source changed: %v", err)
	}
}

func TestAccountLabelRemainsConsistentAcrossUpdatedExports(t *testing.T) {
	root := t.TempDir()
	if _, err := Import(root, source(t, exportFixture), "Work"); err != nil {
		t.Fatal(err)
	}
	next := strings.ReplaceAll(exportFixture, "same-cloud-id", "another-cloud-id")
	report, err := Import(root, source(t, next), "work")
	if err != nil {
		t.Fatal(err)
	}
	if report.Account != "Work" {
		t.Fatalf("account label changed on updated export: %q", report.Account)
	}
	rows, err := List(root)
	if err != nil || len(rows) != 2 {
		t.Fatalf("account records: %d %v", len(rows), err)
	}
	for _, row := range rows {
		if row.Account != "Work" {
			t.Fatalf("one account split into multiple filter labels: %q", row.Account)
		}
	}
}
