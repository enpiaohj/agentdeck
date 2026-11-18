package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// newFileIndexTestApp 与 newTestApp 相同，但索引落在临时文件里。
//
// 增量判据（会话文件的 size/mtime）存在索引库内，而内存库每次 Open 都是一
// 个新的空库：用 ":memory:" 测增量，「未变化的会话被跳过」这条路径根本走
// 不到，删消失会话的路径也走不到，测试会假通过。状态条的分母同样依赖这条
// 路径。凡是要断言增量行为的用例都用它。
func newFileIndexTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	a, err := New(Config{
		MarksPath: filepath.Join(dir, "marks.db"),
		IndexPath: filepath.Join(dir, "index.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

// 增量索引：未变化的会话不重读，追加内容的会话重读，消失的会话删 doc。
// 这三个方向各自单独断言——任何一条反了都会静默给出错的检索结果。
// 分母（Total）是「这一趟要处理的会话数」，它直接暴露跳过有没有发生。
func TestIncrementalIndexSkipsUnchangedAndPicksUpChanges(t *testing.T) {
	dir := t.TempDir()
	keep := claudeFixture(t, dir, "keep", "未改动的正文")
	a := newFileIndexTestApp(t)
	a.setList([]sessions.Session{keep})

	a.buildIndex(false) // 首次：库是空的，等价全量
	if got, err := a.NewSearchService().Query("未改动", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("首次索引后应可检索: %+v %v", got, err)
	}
	if got := a.search.status(); got.Total != 1 {
		t.Fatalf("首次索引的分母应为 1: %+v", got)
	}

	// 同一个会话原样再来一次：走增量，文件没动过就跳过重读。
	a.buildIndex(false)
	if got := a.search.status(); got.Total != 0 {
		t.Fatalf("未变化的会话应被跳过（分母为 0）: %+v", got)
	}
	if got, err := a.NewSearchService().Query("未改动", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("增量后未改动的会话仍应可检索: %+v %v", got, err)
	}

	// 追加内容并让 mtime 明确前移：增量必须重新读到新正文。
	path := filepath.Join(dir, "keep.jsonl")
	line := `{"type":"user","message":{"role":"user","content":"未改动的正文"},"uuid":"u1"}`
	appended := line + "\n" + `{"type":"assistant","message":{"role":"assistant","content":"新增的可检索内容"},"uuid":"u2"}` + "\n"
	if err := os.WriteFile(path, []byte(appended), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	a.buildIndex(false)
	if got := a.search.status(); got.Total != 1 {
		t.Fatalf("改动过的会话应重新处理: %+v", got)
	}
	if got, err := a.NewSearchService().Query("新增的可检索内容", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("增量应索引到追加的内容: %+v %v", got, err)
	}
	if got, err := a.NewSearchService().Query("未改动", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("追加后旧内容仍应可检索: %+v %v", got, err)
	}
}

// 上游消失的会话：doc 必须一起删，否则检索命中一个点进去只有
// 「原始记录不存在」的会话。
func TestIncrementalIndexRemovesSessionsThatDisappeared(t *testing.T) {
	dir := t.TempDir()
	gone := claudeFixture(t, dir, "gone", "即将消失的正文")
	stay := claudeFixture(t, dir, "stay", "继续存在的正文")
	a := newFileIndexTestApp(t)
	a.setList([]sessions.Session{gone, stay})
	a.buildIndex(false)
	if got, err := a.NewSearchService().Query("即将消失", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("首次索引应收录两个会话: %+v %v", got, err)
	}

	a.setList([]sessions.Session{stay})
	a.buildIndex(false)
	// 只剩一个会话且它没变：分母为 0 说明删 doc 发生在增量路径里，而不是
	// 靠整库重来掩盖。
	if got := a.search.status(); got.Total != 0 || got.Sessions != 1 {
		t.Fatalf("消失的会话应被移除、保留的会话应被跳过: %+v", got)
	}
	got, err := a.NewSearchService().Query("即将消失", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 0 {
		t.Fatalf("消失的会话仍可检索: %+v", got.Hits)
	}
	if got, err := a.NewSearchService().Query("继续存在", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("保留的会话不应被牵连: %+v %v", got, err)
	}
}

// 保留副本没有上游文件，unchanged 对它恒为 false：每次增量都重读，
// 永久库里的新版本因此能进索引。
func TestIncrementalIndexAlwaysReindexesRetainedSessions(t *testing.T) {
	dir := t.TempDir()
	sess := claudeFixture(t, dir, "retained", "永久库正文")
	a := newFileIndexTestApp(t)
	a.setList([]sessions.Session{sess})
	// 先真实读一次，把正文写进永久库；再删掉上游文件，它就成了保留副本。
	if _, _, _, err := a.readConversation(sess); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sess.Path); err != nil {
		t.Fatal(err)
	}
	retained := sess
	retained.Retained = true
	a.setList([]sessions.Session{retained})
	a.buildIndex(false)
	if got, err := a.NewSearchService().Query("永久库正文", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("保留副本首次索引: %+v %v", got, err)
	}
	// 第二次增量：没有文件可比，必须重读而不是跳过（分母仍为 1）。
	a.buildIndex(false)
	if got := a.search.status(); got.Total != 1 {
		t.Fatalf("保留副本不应被跳过: %+v", got)
	}
	if got, err := a.NewSearchService().Query("永久库正文", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("保留副本增量后仍应可检索: %+v %v", got, err)
	}
}

// 状态条要报索引规模，不是这一趟的工作量：增量下这两者差得很远，报错了
// 会把 28 万段的库说成 4 万段。分母则相反，是这一趟真正要处理的会话数。
func TestIndexStatusReportsIndexTotalsNotPassWork(t *testing.T) {
	dir := t.TempDir()
	first := claudeFixture(t, dir, "totals-a", "第一段正文")
	second := claudeFixture(t, dir, "totals-b", "第二段正文")
	a := newFileIndexTestApp(t)
	a.setList([]sessions.Session{first, second})
	a.buildIndex(false)
	before := a.search.status()
	if before.Sessions != 2 || before.Docs == 0 {
		t.Fatalf("首次索引状态应覆盖两个会话: %+v", before)
	}

	// 只动第二个会话：这一趟只重读它。
	path := filepath.Join(dir, "totals-b.jsonl")
	line := `{"type":"user","message":{"role":"user","content":"第二段正文"},"uuid":"u1"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":"新增的可检索内容"},"uuid":"u2"}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	a.buildIndex(false)
	after := a.search.status()
	if after.Sessions != 2 {
		t.Fatalf("状态应报整个索引的会话数 2，而不是这一趟处理的 1: %+v", after)
	}
	if after.Docs <= before.Docs {
		t.Fatalf("增量后正文段数应增加: before=%+v after=%+v", before, after)
	}
	if after.Total != 1 {
		t.Fatalf("增量分母应为这一趟要处理的会话数 1: %+v", after)
	}
}

// 全量重建也要把文件状态写回去：Reset 把 doc_sources 一起清了，不写回去
// 下一次增量会因为「一条记录都没有」而重读全部会话，增量等于没建立。
func TestFullRebuildRecordsStampsSoNextIncrementalSkips(t *testing.T) {
	dir := t.TempDir()
	sess := claudeFixture(t, dir, "stamps", "全量重建后的正文")
	a := newFileIndexTestApp(t)
	a.setList([]sessions.Session{sess})
	a.buildIndex(true)
	stamps, err := a.search.ix.Stamps()
	if err != nil {
		t.Fatal(err)
	}
	if len(stamps) != 1 || stamps[0].Key != "claude:stamps" {
		t.Fatalf("全量重建后应有文件状态记录: %+v", stamps)
	}
	// 有记录在手，紧接着的增量这一趟无事可做。
	a.buildIndex(false)
	if got := a.search.status(); got.Total != 0 || got.Sessions != 1 {
		t.Fatalf("全量重建后的增量应跳过未变化的会话: %+v", got)
	}
}

// 全量重建仍然清库：设置里的「重建索引」要的就是这个语义。
func TestFullRebuildStillResetsTheIndex(t *testing.T) {
	dir := t.TempDir()
	first := claudeFixture(t, dir, "full", "全量重建前的正文")
	a := newFileIndexTestApp(t)
	a.setList([]sessions.Session{first})
	a.buildIndex(false)
	if got, err := a.NewSearchService().Query("全量重建前", 20); err != nil || len(got.Hits) != 1 {
		t.Fatalf("首次索引: %+v %v", got, err)
	}
	a.setList(nil)
	a.buildIndex(true)
	if got, err := a.NewSearchService().Query("全量重建前", 20); err != nil || len(got.Hits) != 0 {
		t.Fatalf("全量重建应清空库: %+v", got.Hits)
	}
}
