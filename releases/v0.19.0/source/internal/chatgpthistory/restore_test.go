package chatgpthistory

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func idOf(account, conv string) string {
	return "gpt_" + digest(digest(strings.ToLower(account))+":"+conv)
}

func transcriptExists(root, id string) bool {
	m, _ := filepath.Glob(filepath.Join(root, "*", "*", id+".jsonl"))
	return len(m) > 0
}

// A conversation deleted from AgentDeck is not written again: deleting it was meant to
// clear its private copy from the disk, and an export holds everything still in ChatGPT.
func TestImportDoesNotWriteDeletedConversations(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	c1 := chat("c1", "甲", "", 1_760_000_000)
	c2 := chat("c2", "乙", "", 1_760_100_000)
	deleted := map[string]bool{idOf("A", "c1"): true}
	zp := writeExport(t, dir, map[string][]tnode{"conversations-000.json": {c1, c2}})
	rep, err := Import(root, zp, "A", Options{Deleted: func(id string) bool { return deleted[id] }})
	if err != nil || rep.SkippedDeleted != 1 || rep.Conversations != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if transcriptExists(root, idOf("A", "c1")) {
		t.Fatal("被删除的会话不该再写回磁盘")
	}
	if !transcriptExists(root, idOf("A", "c2")) {
		t.Fatal("没删的会话照常写入")
	}
	// an export of nothing but deleted conversations is a no-op, not an error
	zp2 := writeExport(t, mk(t, dir, "only"), map[string][]tnode{"conversations-000.json": {c1}})
	rep, err = Import(root, zp2, "A", Options{Deleted: func(id string) bool { return deleted[id] }})
	if err != nil || rep.Conversations != 0 || rep.SkippedDeleted != 1 || batchesOnDisk(root) != 1 {
		t.Fatalf("全是已删除的会话: %+v %v batches=%d", rep, err, batchesOnDisk(root))
	}
}

func TestPreviewListsDeletedConversations(t *testing.T) {
	dir := t.TempDir()
	c1 := chat("c1", "甲 标题", "", 1_760_000_000)
	c2 := chat("c2", "乙", "", 1_760_100_000)
	zp := writeExport(t, dir, map[string][]tnode{"conversations-000.json": {c1, c2}})
	deleted := func(id string) bool { return id == idOf("A", "c1") }
	store := filepath.Join(dir, "store")
	res, err := Preview(store, zp, "A", deleted)
	if err != nil || len(res.Deleted) != 1 || res.Deleted[0].ID != idOf("A", "c1") || res.Deleted[0].Title != "甲 标题" || res.Deleted[0].Updated == "" {
		t.Fatalf("%+v %v", res.Deleted, err)
	}
	// ids depend on the account name: under another name nothing of this was deleted
	if res, _ = Preview(store, zp, "B", deleted); len(res.Deleted) != 0 {
		t.Fatalf("%+v", res.Deleted)
	}
	// no name typed yet, or no way to tell: nothing is listed
	if res, _ = Preview(store, zp, "", deleted); len(res.Deleted) != 0 {
		t.Fatal("没填账号名时无法判断")
	}
	if res, _ = Preview(store, zp, "A", nil); len(res.Deleted) != 0 {
		t.Fatal("没有删除记录")
	}
}

// Of several deleted conversations only the chosen come back, as a small batch of their
// own when the export was imported before.
func TestRestoreChosenDeletedConversations(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	c1 := chat("c1", "甲", "", 1_760_000_000)
	c2 := chat("c2", "乙", "", 1_760_100_000)
	c3 := chat("c3", "丙", "", 1_760_200_000)
	zp := writeExport(t, dir, map[string][]tnode{"conversations-000.json": {c1, c2, c3}})
	if _, err := Import(root, zp, "A", Options{}); err != nil {
		t.Fatal(err)
	}
	// 甲 and 乙 are deleted (their files go), 丙 stays
	gone := map[string]bool{idOf("A", "c1"): true, idOf("A", "c2"): true}
	for id := range gone {
		files, _ := TranscriptFiles(root, id)
		for _, f := range files {
			os.Remove(f)
		}
	}
	isDeleted := func(id string) bool { return gone[id] }
	if titles := titlesOf(t, root, isDeleted); len(titles) != 1 || titles[0] != "丙" {
		t.Fatalf("删除后: %v", titles)
	}
	// the same export again, restoring only 甲
	rep, err := Import(root, zp, "A", Options{Deleted: isDeleted, Restore: []string{idOf("A", "c1")}})
	if err != nil || rep.AlreadyImported || len(rep.Restored) != 1 || rep.Restored[0] != idOf("A", "c1") || rep.Conversations != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if !transcriptExists(root, idOf("A", "c1")) || transcriptExists(root, idOf("A", "c2")) {
		t.Fatal("只该写回勾选的那一个")
	}
	// the restored one shows once the block on it is lifted; the other stays out
	delete(gone, idOf("A", "c1"))
	list, err := List(root, isDeleted)
	if err != nil {
		t.Fatalf("旧批次里的残留条目不该算成问题: %v", err)
	}
	var titles []string
	for _, s := range list {
		titles = append(titles, s.Title)
	}
	sort.Strings(titles)
	if strings.Join(titles, ",") != "丙,甲" {
		t.Fatalf("恢复后: %v", titles)
	}
	// an id that is not in the export cannot be restored
	if _, err := Import(root, zp, "A", Options{Deleted: isDeleted, Restore: []string{idOf("A", "nope")}}); err == nil {
		t.Fatal("导出里没有的会话不能被“恢复”")
	}
	// ids that are not stored ids are ignored altogether
	if _, err := Import(root, zp, "A", Options{Restore: []string{"../x", "gpt_zz"}}); err != nil {
		t.Fatalf("格式不对的 ID 直接忽略: %v", err)
	}
}

// For an export not imported yet, the chosen ones come in with everything else that is not deleted.
func TestRestoreOnFirstImport(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	c1 := chat("c1", "甲", "", 1_760_000_000)
	c2 := chat("c2", "乙", "", 1_760_100_000)
	c3 := chat("c3", "丙", "", 1_760_200_000)
	zp := writeExport(t, dir, map[string][]tnode{"conversations-000.json": {c1, c2, c3}})
	gone := map[string]bool{idOf("A", "c1"): true, idOf("A", "c2"): true}
	rep, err := Import(root, zp, "A", Options{Deleted: func(id string) bool { return gone[id] }, Restore: []string{idOf("A", "c2")}})
	if err != nil || rep.Conversations != 2 || rep.SkippedDeleted != 1 || len(rep.Restored) != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if transcriptExists(root, idOf("A", "c1")) || !transcriptExists(root, idOf("A", "c2")) || !transcriptExists(root, idOf("A", "c3")) {
		t.Fatal("乙恢复、丙照常、甲仍不写入")
	}
}

// A missing file is no problem when another copy of the conversation shows.
func TestListIgnoresLeftoverEntryWhenAnotherCopyShows(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	c1 := chat("c1", "甲", "", 1_760_000_000)
	c2 := chat("c2", "乙", "", 1_760_100_000)
	if _, err := Import(root, writeExport(t, dir, map[string][]tnode{"conversations-000.json": {c1, c2}}), "A", Options{}); err != nil {
		t.Fatal(err)
	}
	files, _ := TranscriptFiles(root, idOf("A", "c1"))
	os.Remove(files[0])
	if _, err := List(root, nil); err == nil {
		t.Fatal("没有别的副本时缺文件要报告")
	}
	zp2 := writeExport(t, mk(t, dir, "again"), map[string][]tnode{"conversations-000.json": {updated(t, c1, "", 1_770_000_000)}})
	if _, err := Import(root, zp2, "A", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := List(root, nil); err != nil {
		t.Fatalf("有别的副本显示时，旧批次里缺的文件不再是问题: %v", err)
	}
}
