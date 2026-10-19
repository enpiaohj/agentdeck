package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/search"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// searchState 是全文索引的运行态。state: idle | building | ready | failed。
type searchState struct {
	mu   sync.Mutex
	path string

	state      string
	sessions   int
	docs       int
	total      int
	err        string
	building   bool
	pending    bool // coalesce rebuild requests received during a build
	closed     bool
	generation uint64 // session snapshot represented by this index

	ix *search.Index
}

func newSearchState(path string) *searchState {
	return &searchState{path: path, state: "idle"}
}

func (st *searchState) status() IndexStatus {
	st.mu.Lock()
	defer st.mu.Unlock()
	return IndexStatus{State: st.state, Sessions: st.sessions, Docs: st.docs, Total: st.total, Error: st.err}
}

func (st *searchState) close() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.closed = true
	st.pending = false
	if st.ix != nil {
		st.ix.Close()
		st.ix = nil
	}
}

// SearchService 是全文检索服务:启动后台重建 + 查询 + 未就绪降级。
type SearchService struct{ a *App }

// Rebuild 触发一次后台更新;构建中重复调用合并为一次后续更新。
//
// 增量：索引库已存在且分词契约未变时，只 upsert 仍存在的会话、删掉已消失
// 的会话，不清库。整库 Reset 只在库不存在、schema 不符或调用方显式要求
// 全量时发生——28 万段 / 1.7 GB 的库每次启动都重写一遍，期间状态栏一直
// 停在 building，代价远大于增量。Add 的语义本就是按 Key 替换。
func (s *SearchService) Rebuild() {
	go s.a.buildIndex(false)
}

// RebuildFull 强制整库重建（设置里的「重建索引」）。分词契约或脱敏规则
// 变更后用它，日常启动与刷新走 Rebuild 的增量路径。
func (s *SearchService) RebuildFull() {
	go s.a.buildIndex(true)
}

// Status 索引状态(状态条)。
func (s *SearchService) Status() IndexStatus {
	status := s.a.search.status()
	s.a.mu.Lock()
	generation := s.a.generation
	s.a.mu.Unlock()
	s.a.search.mu.Lock()
	if status.State == "ready" && s.a.search.generation != generation {
		status.State = "idle"
	}
	s.a.search.mu.Unlock()
	return status
}

// Query 全文检索。索引就绪走 FTS(bm25 排序);未就绪降级为逐会话
// 子串扫描(sessions.SearchSession,字节预筛,单会话快),前端按
// Reply.Mode 展示「快速搜索」横幅。
func (s *SearchService) Query(q string, limit int) (*SearchReply, error) {
	if limit <= 0 {
		limit = 20
	}
	st := s.a.search
	s.a.ensure()
	// One immutable snapshot supplies both the generation check and metadata.
	// Lock order is search -> app; no app lock is held while acquiring search.
	st.mu.Lock()
	s.a.mu.Lock()
	generation, byKey, ms := s.a.generation, s.a.byKey, s.a.marksAll
	s.a.mu.Unlock()
	ready := st.state == "ready" && st.ix != nil && st.generation == generation
	var ix *search.Index
	if ready {
		ix = st.ix
		// Keep replacement/Close from invalidating a query's index handle.
		defer st.mu.Unlock()
		return s.queryIndex(ix, byKey, ms, q, limit)
	}
	st.mu.Unlock()
	return s.queryScan(q, limit)
}

// queryIndex 全文索引路径。
func (s *SearchService) queryIndex(ix *search.Index, byKey map[string]sessions.Session, ms map[string]marks.Mark, q string, limit int) (*SearchReply, error) {
	rs, err := ix.Search(context.Background(), q, limit)
	if err != nil {
		return nil, err
	}
	total, err := ix.Count(context.Background(), q)
	if err != nil {
		total = len(rs) // 计数失败退回所见,不假装是全部
	}
	out := make([]SearchHitView, 0, len(rs))
	for _, r := range rs {
		key, _, _ := splitDocKey(r.Key)
		sess, exists := byKey[key]
		if !exists {
			continue
		}
		out = append(out, SearchHitView{
			Key:     key,
			Part:    r.Key,
			Agent:   sess.Agent,
			Title:   displayTitle(sess, ms[sessionMarkKey(sess, key, ms)]),
			Last:    lastString(sess.Last),
			Match:   search.Anchor(r.Text, q),
			Snippet: search.Excerpt(r.Text, q, 36, 120),
		})
	}
	return &SearchReply{Mode: "index", Total: total, Hits: out}, nil
}

// queryScan 降级路径:逐会话子串扫描(每会话至多一条命中)。
func (s *SearchService) queryScan(q string, limit int) (*SearchReply, error) {
	needle := sessions.NeedleOf(q)
	if needle == "" {
		return &SearchReply{Mode: "scan", Hits: []SearchHitView{}}, nil
	}
	list, _, ms := s.a.snapshot()
	out := make([]SearchHitView, 0, limit)
	for _, sess := range list {
		if len(out) >= limit {
			break
		}
		if !sessions.HasTranscript(sess.Agent) || sess.Path == "" {
			continue
		}
		tr, _, _, err := s.a.readConversation(sess)
		if err != nil {
			continue
		}
		h, found := sessions.SearchRetainedTranscript(tr, needle)
		if !found {
			continue
		}
		out = append(out, SearchHitView{
			Key:   sess.Agent + ":" + sess.ID,
			Part:  sess.Agent + ":" + sess.ID,
			Agent: sess.Agent, Title: displayTitle(sess, ms[sessionMarkKey(sess, sess.Agent+":"+sess.ID, ms)]), Last: lastString(sess.Last),
			Match: h.Match, Snippet: h.Before + h.Match + h.After,
		})
	}
	return &SearchReply{Mode: "scan", Total: len(out), Hits: out}, nil
}

// splitDocKey 拆 doc key「agent:id#n」为会话 key 与序号。
func splitDocKey(docKey string) (key string, part int, ok bool) {
	i := strings.LastIndex(docKey, "#")
	if i < 0 {
		return docKey, 0, false
	}
	n, err := strconv.Atoi(docKey[i+1:])
	if err != nil {
		return docKey[:i], 0, false
	}
	return docKey[:i], n, true
}

// buildIndex 后台更新索引:Open→(full? Reset)→逐会话读正文→批量 Add。
// 增量时另需删掉已消失会话的 doc(上游删除、导入库清理)。
// 进度经 EventIndexProgress 推送,完成/失败推 EventIndexReady。
func (a *App) buildIndex(full bool) {
	st := a.search
	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		return
	}
	if st.building {
		st.pending = true
		st.mu.Unlock()
		return
	}
	st.building = true
	st.state = "building"
	st.err = ""
	st.mu.Unlock()
	a.pushIndex(EventIndexProgress, 0, 0)
	defer func() {
		st.mu.Lock()
		st.building = false
		pending := st.pending && !st.closed
		st.pending = false
		st.mu.Unlock()
		if pending {
			go a.buildIndex(full)
		}
	}()

	ix, err := search.Open(st.path)
	if err != nil {
		a.failIndex(err)
		return
	}
	// 只有全量重建才清库。增量路径下 Reset 会把 1.7 GB / 28 万段的库整个
	// 重写一遍，而 Add 的语义本就是按 Key 替换，没有必要。
	if full {
		if err := ix.Reset(); err != nil {
			ix.Close()
			a.failIndex(err)
			return
		}
	}
	// 换上新库(关旧的)
	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		ix.Close()
		return
	}
	if st.ix != nil && st.ix != ix {
		st.ix.Close()
	}
	st.ix = ix
	st.mu.Unlock()

	a.ensure()
	a.mu.Lock()
	list, generation := a.list, a.generation
	a.mu.Unlock()
	// 增量路径下先取库内已有的会话集合与它们的文件状态：读过的会话按
	// size/mtime 判是否变过，没变就跳过重新解析；消失的会话最后统一删 doc。
	// 取不到时按全量处理，不猜。
	stale, staleErr := ix.Keys()
	known := make(map[string]bool, len(stale))
	for _, k := range stale {
		known[k] = true
	}
	stamps := map[string]search.SourceStamp{}
	if recorded, err := ix.Stamps(); err == nil {
		for _, s := range recorded {
			stamps[s.Key] = s
		}
	}
	if staleErr != nil || full {
		known = nil // 不增量：每个会话都重读
		stamps = nil
	}
	// 先挑出这一趟真正要读的会话，再报分母。分母若按「全部可索引会话」算，
	// 增量下进度会停在 155 / 520 这种数字上直接跳到就绪——实际工作量才是
	// 用户需要看到的那个分母。live 顺带在这里建好，末尾据此删消失的会话。
	type pendingSession struct {
		sess sessions.Session
		key  string
		now  sourceStamp
	}
	live := make(map[string]bool, len(list))
	work := make([]pendingSession, 0, len(list))
	for _, s := range list {
		if !sessions.HasTranscript(s.Agent) || s.Path == "" {
			continue
		}
		key := s.Agent + ":" + s.ID
		live[key] = true
		now := stampOf(s.Path)
		// 增量跳过判据是会话文件的大小与修改时间：上游没动过就不重新解析。
		// 保留副本没有上游文件，unchanged 对它恒为 false，永远重读。
		if known != nil && known[key] && now.size > 0 {
			if previous, ok := stamps[key]; ok && unchanged(s, now, sourceStamp{size: previous.Size, mtime: time.Unix(0, previous.Mtime)}) {
				continue
			}
		}
		work = append(work, pendingSession{sess: s, key: key, now: now})
	}
	st.mu.Lock()
	st.total = len(work)
	st.mu.Unlock()
	a.setIndexTotal(len(work))
	docs := make([]search.Doc, 0, 64)
	sessionsN, docsN := 0, 0
	lastEmit := time.Now()
	var updated []search.SourceStamp
	for _, w := range work {
		s, key, now := w.sess, w.key, w.now
		st.mu.Lock()
		closed := st.closed
		st.mu.Unlock()
		if closed {
			return
		}
		// 注意别用 full 命名：外层那个 full 是「是否整库重建」，同名会遮住它。
		conv, _, _, err := a.readConversation(s)
		if err != nil {
			continue
		}
		for i, p := range conv.Parts {
			// 与 CLI/Phase 1 同一过滤:agent 每次都带的系统上下文与图片不收
			if p.Role == "system" || p.Kind == "context" || p.Kind == "image" {
				continue
			}
			docs = append(docs, search.Doc{Key: fmt.Sprintf("%s#%d", key, i), Text: p.Text})
		}
		if err := ix.Add(docs); err != nil {
			a.failIndex(err)
			return
		}
		docsN += len(docs)
		docs = docs[:0]
		sessionsN++
		if now.size > 0 {
			updated = append(updated, search.SourceStamp{Key: key, Size: now.size, Mtime: now.mtime.UnixNano()})
		}
		if a.cfg.Emit != nil && time.Since(lastEmit) >= 500*time.Millisecond {
			lastEmit = time.Now()
			a.pushIndex(EventIndexProgress, sessionsN, docsN)
		}
	}
	// 上游已删除、导入库已清理的会话：它的 doc 必须一起走，否则检索会命中
	// 一个点进去只有「原始记录不存在」的会话。
	if known != nil {
		var gone []string
		for _, k := range stale {
			if !live[k] {
				gone = append(gone, k)
			}
		}
		if len(gone) > 0 {
			if err := ix.Remove(gone); err != nil {
				a.failIndex(err)
				return
			}
		}
	}
	// 文件状态最后统一写：先 Remove 再 SaveStamps，SaveStamps 只给索引里
	// 仍有 doc 的会话留记录，反序会留下永不匹配的孤儿行。全量路径同样要写
	// ——Reset 把 doc_sources 一起清了，不写回去，下一次增量会因为「一条
	// 记录都没有」而重读全部会话，增量等于没建立。
	if err := ix.SaveStamps(updated); err != nil {
		a.failIndex(err)
		return
	}
	// 状态条要的是索引规模，不是这一趟的工作量：增量下这两者差得很远
	// （这趟只重读了变过的会话），直接报这一趟的计数会把 28 万段的库说成
	// 4 万段。统计失败不当作构建失败——正文已经写完，索引可用，退回这一趟
	// 的计数，只在数字上保守。
	indexSessions, indexDocs := sessionsN, docsN
	if total, totalDocs, err := ix.Totals(); err == nil {
		indexSessions, indexDocs = total, totalDocs
	}
	a.mu.Lock()
	currentGeneration := a.generation
	a.mu.Unlock()
	st.mu.Lock()
	st.state = "ready"
	if st.closed {
		st.mu.Unlock()
		return
	}
	if generation != currentGeneration {
		st.state = "idle"
		st.pending = true
	}
	st.generation = generation
	st.sessions, st.docs = indexSessions, indexDocs
	st.err = ""
	st.mu.Unlock()
	a.pushIndex(EventIndexReady, indexSessions, indexDocs)
}

func (a *App) failIndex(err error) {
	st := a.search
	st.mu.Lock()
	st.state = "failed"
	st.err = err.Error()
	st.mu.Unlock()
	a.pushIndex(EventIndexReady, st.status().Sessions, st.status().Docs)
}

func (a *App) pushIndex(event string, sessionsN, docsN int) {
	st := a.search.status()
	st.Sessions, st.Docs = sessionsN, docsN
	a.emit(event, st)
}

// setIndexTotal 单独推送一次分母：buildIndex 算完可索引会话数后调用，
// 不必等下一个 500ms 节流点，状态条就能显示「n / total」。
func (a *App) setIndexTotal(total int) {
	st := a.search.status()
	st.Total = total
	a.emit(EventIndexProgress, st)
}
