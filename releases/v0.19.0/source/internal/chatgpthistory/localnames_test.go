package chatgpthistory

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeState(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, stateFile), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLocalProjectNames(t *testing.T) {
	dir := writeState(t, `{
	  "unrelated": {"token": "must never be read"},
	  "local-projects": {"g-p-111": {"id":"g-p-111","name":"旧列表里的名字"}, "not-a-project": {"name":"x"}},
	  "electron-persisted-atom-state": {
	    "something-else": {"g-p-999": "ignored"},
	    "chatgpt-sidebar-state-v1": {"acct-1": {"projects": [{"id":"g-p-222","name":"财务/资料: 2025"},{"id":"g-p-111","name":"v1 的名字"}]}},
	    "chatgpt-sidebar-state-v2": {"acct-1": {"projects": [{"id":"g-p-111","name":"最新的名字"},{"id":"g-p-333","name":"  "},{"id":"x-444","name":"非项目"}]}}
	  }}`)
	got := LocalProjectNames(dir)
	want := map[string]string{"g-p-111": "最新的名字", "g-p-222": "财务 资料 2025"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for id, name := range want {
		if got[id] != name {
			t.Errorf("%s = %q, want %q（新版本优先、名字按路径元素清理、空名与非项目编号忽略）", id, got[id], name)
		}
	}
}

func TestLocalProjectNamesNeverFails(t *testing.T) {
	for name, dir := range map[string]string{
		"empty home":     "",
		"no file":        t.TempDir(),
		"broken json":    writeState(t, "{broken"),
		"wrong shape":    writeState(t, `{"electron-persisted-atom-state": {"chatgpt-sidebar-state-v2": "text"}}`),
		"projects wrong": writeState(t, `{"electron-persisted-atom-state": {"chatgpt-sidebar-state-v2": {"a": {"projects": 5}}}}`),
	} {
		if got := LocalProjectNames(dir); got == nil || len(got) != 0 {
			t.Errorf("%s: %v", name, got)
		}
	}
	// the state file is only read, never changed
	dir := writeState(t, `{"local-projects":{"g-p-1":{"name":"a"}}}`)
	before, _ := os.ReadFile(filepath.Join(dir, stateFile))
	LocalProjectNames(dir)
	if after, _ := os.ReadFile(filepath.Join(dir, stateFile)); string(after) != string(before) {
		t.Fatal("状态文件被改动")
	}
}

func TestApplySuggestions(t *testing.T) {
	res := PreviewResult{Projects: []ProjectInfo{{ID: "g-p-1"}, {ID: "g-p-2"}}}
	ApplySuggestions(&res, map[string]string{"g-p-2": "有名字"})
	if res.Projects[0].Suggested != "" || res.Projects[1].Suggested != "有名字" {
		t.Fatalf("%+v", res.Projects)
	}
}

// library_files.json names the files uploaded to each project; the preview shows up to
// three (newest first), leaving out deleted and hidden ones and other accounts' non-projects.
func TestPreviewListsProjectFiles(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "e.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("conversations-000.json")
	json.NewEncoder(w).Encode([]tnode{chat("c1", "项目里", "g-p-aaaa", 1_760_000_000), chat("c2", "没有文件的项目", "g-p-bbbb", 1_760_100_000)})
	w, _ = zw.Create("library_files.json")
	del := "2026-01-01T00:00:00Z"
	no := false
	json.NewEncoder(w).Encode([]tnode{
		{"file_name": "旧的.pdf", "gizmo_id": "g-p-aaaa", "created_at": "2026-01-01", "deleted_at": nil},
		{"file_name": "已删除.pdf", "gizmo_id": "g-p-aaaa", "created_at": "2026-09-01", "deleted_at": del},
		{"file_name": "隐藏.pdf", "gizmo_id": "g-p-aaaa", "created_at": "2026-09-02", "deleted_at": nil, "is_visible": no},
		{"file_name": "预算.xlsx", "gizmo_id": "g-p-aaaa", "created_at": "2026-05-01", "deleted_at": nil},
		{"file_name": "预算.xlsx", "gizmo_id": "g-p-aaaa", "created_at": "2026-04-01", "deleted_at": nil}, // same name: once
		{"file_name": "说明\n文档.docx", "gizmo_id": "g-p-aaaa", "created_at": "2026-06-01", "deleted_at": nil},
		{"file_name": "第四个.txt", "gizmo_id": "g-p-aaaa", "created_at": "2025-12-01", "deleted_at": nil},
		{"file_name": "普通上传.png", "gizmo_id": nil, "created_at": "2026-07-01", "deleted_at": nil},
		{"file_name": "别人的.pdf", "gizmo_id": "g-x-zzzz", "created_at": "2026-07-01", "deleted_at": nil},
	})
	zw.Close()
	f.Close()
	res, err := Preview(filepath.Join(dir, "store"), zp, "A", nil)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ProjectInfo{}
	for _, p := range res.Projects {
		byID[p.ID] = p
	}
	if got := strings.Join(byID["g-p-aaaa"].Files, "|"); got != "说明 文档.docx|预算.xlsx|旧的.pdf" {
		t.Fatalf("项目文件应按时间由新到旧、去掉已删除与隐藏的、同名只算一次、最多三个: %q", got)
	}
	if len(byID["g-p-bbbb"].Files) != 0 {
		t.Fatalf("没有上传文件的项目: %v", byID["g-p-bbbb"].Files)
	}
	// an export without library_files.json (or with a broken one) still previews
	if got := (&exportSource{}).projectFiles(); len(got) != 0 {
		t.Fatal(got)
	}
}
