package chatgpthistory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func batchesOnDisk(root string) int {
	m, _ := filepath.Glob(filepath.Join(root, "*", "*", "manifest.json"))
	return len(m)
}

func titlesOf(t *testing.T, root string, skip func(string) bool) []string {
	t.Helper()
	list, err := List(root, skip)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range list {
		out = append(out, s.Title)
	}
	sort.Strings(out)
	return out
}

// Three exports over time: 甲 gains messages, 丙 appears, 乙 is deleted in ChatGPT. Each
// import leaves only what still shows something; a conversation deleted upstream stays.
func TestReimportReclaimsSupersededBatches(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	c1 := chat("c1", "甲", "g-p-aaaa", 1_760_000_000)
	c2 := chat("c2", "乙", "", 1_760_100_000)
	c3 := chat("c3", "丙", "", 1_760_200_000)
	c1u := updated(t, c1, "", 1_770_000_000)
	exp := func(name string, convs ...tnode) string {
		return writeExport(t, mk(t, dir, name), map[string][]tnode{"conversations-000.json": convs})
	}

	rep, err := Import(root, exp("e1", c1, c2), "A", Options{})
	if err != nil || rep.Reclaimed != 0 || batchesOnDisk(root) != 1 {
		t.Fatalf("第一次导入: %+v %v batches=%d", rep, err, batchesOnDisk(root))
	}
	// E2 holds a newer 甲, the same 乙, a new 丙: E1 shows nothing any more
	rep, err = Import(root, exp("e2", c1u, c2, c3), "A", Options{})
	if err != nil || rep.Reclaimed != 1 || rep.ReclaimedBytes == 0 || batchesOnDisk(root) != 1 {
		t.Fatalf("第二次导入应回收被取代的第一批: %+v %v batches=%d", rep, err, batchesOnDisk(root))
	}
	if got := strings.Join(titlesOf(t, root, nil), ","); got != "丙,乙,甲" {
		t.Fatalf("会话 = %s", got)
	}
	list, _ := List(root, nil)
	for _, s := range list {
		if s.Title == "甲" && s.MessageCount != 6 {
			t.Fatalf("甲应是更新后的内容: %d 条", s.MessageCount)
		}
	}
	// E3: 乙 was deleted in ChatGPT. E2 still holds the only copy of 乙, so it stays.
	rep, err = Import(root, exp("e3", c1u, c3), "A", Options{})
	if err != nil || batchesOnDisk(root) != 2 {
		t.Fatalf("乙只有第二批还有，该批要保留: %+v %v batches=%d", rep, err, batchesOnDisk(root))
	}
	if got := strings.Join(titlesOf(t, root, nil), ","); got != "丙,乙,甲" {
		t.Fatalf("在 ChatGPT 里删掉的会话仍应留在本机: %s", got)
	}
	// an older export imported afterwards must not bring anything back, and leaves nothing behind
	before := batchesOnDisk(root)
	if _, err = Import(root, exp("e1b", c1), "A", Options{}); err != nil {
		t.Fatal(err)
	}
	if batchesOnDisk(root) != before {
		t.Fatalf("更旧的导出不显示任何东西，导入后应被回收: %d → %d", before, batchesOnDisk(root))
	}
	list, _ = List(root, nil)
	for _, s := range list {
		if s.Title == "甲" && s.MessageCount != 6 {
			t.Fatal("旧导出不能覆盖新内容")
		}
	}
}

// reclaim touches only batches it can read, only the files their manifest names, and
// removes a folder only when that leaves it empty.
func TestReclaimIsCareful(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	c1 := chat("c1", "甲", "", 1_760_000_000)
	exp := func(name string, convs ...tnode) string {
		return writeExport(t, mk(t, dir, name), map[string][]tnode{"conversations-000.json": convs})
	}
	if _, err := Import(root, exp("e1", c1), "A", Options{}); err != nil {
		t.Fatal(err)
	}
	accountRoot := filepath.Join(root, digest("a"))
	first, _ := filepath.Glob(filepath.Join(accountRoot, "*", "manifest.json"))
	oldBatch := filepath.Dir(first[0])
	// a file of the user's in that batch folder, and an unreadable second batch
	stray := filepath.Join(oldBatch, "notes.txt")
	os.WriteFile(stray, []byte("mine"), 0o600)
	broken := filepath.Join(accountRoot, "deadbeef")
	os.MkdirAll(broken, 0o700)
	os.WriteFile(filepath.Join(broken, "manifest.json"), []byte("{broken"), 0o600)

	c1u := updated(t, c1, "", 1_770_000_000)
	rep, err := Import(root, exp("e2", c1u), "A", Options{})
	if err != nil || rep.Reclaimed != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatal("批次文件夹里不属于导入的文件不能动")
	}
	if _, err := os.Stat(filepath.Join(oldBatch, "manifest.json")); err == nil {
		t.Fatal("被取代的批次的清单应被移除")
	}
	if _, err := os.Stat(filepath.Join(broken, "manifest.json")); err != nil {
		t.Fatal("读不了的批次不能碰")
	}
}

// Account name: a second spelling would store everything twice — the preview says so.
func TestPreviewFindsAccountsSharingConversations(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	c1 := chat("c1", "甲", "", 1_760_000_000)
	c2 := chat("c2", "乙", "", 1_760_100_000)
	c3 := chat("c3", "丙", "", 1_760_200_000)
	exp := func(name string, convs ...tnode) string {
		return writeExport(t, mk(t, dir, name), map[string][]tnode{"conversations-000.json": convs})
	}
	if _, err := Import(root, exp("e1", c1, c2, c3), "个人", Options{}); err != nil {
		t.Fatal(err)
	}
	// the same export previewed under whatever name: one stored account shares all three
	res, err := Preview(root, exp("p1", c1, c2, c3), "个人账号")
	if err != nil || len(res.Accounts) != 1 || res.Accounts[0].Name != "个人" || res.Accounts[0].Overlap != 3 || res.Accounts[0].Missing != 0 {
		t.Fatalf("%+v %v", res.Accounts, err)
	}
	// a newer export without 乙: two shared, one the export no longer holds
	res, _ = Preview(root, exp("p2", c1, c3), "个人")
	if len(res.Accounts) != 1 || res.Accounts[0].Overlap != 2 || res.Accounts[0].Missing != 1 {
		t.Fatalf("Missing 应是已导入却不在这份导出里的会话: %+v", res.Accounts)
	}
	// an export of something else shares nothing
	other := chat("zz", "别的", "", 1_750_000_000)
	if res, _ = Preview(root, exp("p3", other), "x"); len(res.Accounts) != 0 {
		t.Fatalf("没有重叠不该提示: %+v", res.Accounts)
	}
	// nothing stored yet
	if res, _ = Preview(filepath.Join(dir, "empty"), exp("p4", c1), "x"); len(res.Accounts) != 0 {
		t.Fatal("还没有导入过任何账号")
	}
}

// Every copy of a conversation, in every batch, and nothing else.
func TestTranscriptFilesFindsEveryCopy(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	c1 := chat("c1", "甲", "", 1_760_000_000)
	c2 := chat("c2", "乙", "", 1_760_100_000)
	c3 := chat("c3", "丙", "", 1_760_200_000)
	exp := func(name string, convs ...tnode) string {
		return writeExport(t, mk(t, dir, name), map[string][]tnode{"conversations-000.json": convs})
	}
	if _, err := Import(root, exp("e1", c1, c2), "A", Options{}); err != nil {
		t.Fatal(err)
	}
	// 乙 is only in the first batch, so both batches stay and 甲 has two copies
	if _, err := Import(root, exp("e2", updated(t, c1, "", 1_770_000_000), c3), "A", Options{}); err != nil {
		t.Fatal(err)
	}
	list, _ := List(root, nil)
	var id string
	for _, s := range list {
		if s.Title == "甲" {
			id = s.ID
		}
	}
	files, err := TranscriptFiles(root, id)
	if err != nil || len(files) != 2 {
		t.Fatalf("甲在两个批次里各有一份: %v %v", files, err)
	}
	for _, f := range files {
		if !strings.HasSuffix(f, id+".jsonl") {
			t.Fatalf("%s", f)
		}
	}
	for _, bad := range []string{"", "gpt_x", "../../etc/passwd", "gpt_" + strings.Repeat("g", 64), id + "/.."} {
		if got, err := TranscriptFiles(root, bad); err == nil || len(got) != 0 {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
	// a staging folder is not a batch
	stage := filepath.Join(root, digest("a"), ".stage-x")
	os.MkdirAll(stage, 0o700)
	os.WriteFile(filepath.Join(stage, id+".jsonl"), []byte("x"), 0o600)
	if files, _ = TranscriptFiles(root, id); len(files) != 2 {
		t.Fatalf("暂存目录里的文件不算副本: %v", files)
	}
}

// Conversations named in skip are left out silently, even with their files gone.
func TestListSkipsDeletedWithoutError(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	zp := writeExport(t, dir, fixtureShards())
	if _, err := Import(root, zp, "A", Options{}); err != nil {
		t.Fatal(err)
	}
	all, _ := List(root, nil)
	gone := all[0]
	os.Remove(gone.Path)
	if _, err := List(root, nil); err == nil {
		t.Fatal("没有声明删除时，缺文件应报告")
	}
	list, err := List(root, func(id string) bool { return id == gone.ID })
	if err != nil || len(list) != len(all)-1 {
		t.Fatalf("声明删除的会话不报错、不显示: %d %v", len(list), err)
	}
}

func cloneChat(t *testing.T, c tnode) tnode {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var out tnode
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// updated returns c with one more exchange at the end and a later update_time.
func updated(t *testing.T, c tnode, title string, bump float64) tnode {
	t.Helper()
	n := cloneChat(t, c)
	m := n["mapping"].(map[string]any)
	m["a2"].(map[string]any)["children"] = []any{"u3"}
	m["u3"] = msgNode("u3", "a2", []string{"a3"}, "user", "text", []any{"后来追加的问题"}, bump, nil)
	m["a3"] = msgNode("a3", "u3", nil, "assistant", "text", []any{"后来追加的回答"}, bump+1, nil)
	n["current_node"] = "a3"
	n["update_time"] = bump + 2
	if title != "" {
		n["title"] = title
	}
	return n
}
