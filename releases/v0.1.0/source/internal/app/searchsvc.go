package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/enpiaohj/agentdeck/internal/search"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// searchState 是全文索引的运行态。state: idle | building | ready | failed。
type searchState struct {
	mu   sync.Mutex
	path string

	state    string
	sessions int
	docs     int
	err      string
	building bool

	ix *search.Index
}

func newSearchState(path string) *searchState {
	return &searchState{path: path, state: "idle"}
}

func (st *searchState) status() IndexStatus {
	st.mu.Lock()
	defer st.mu.Unlock()
	return IndexStatus{State: st.state, Sessions: st.sessions, Docs: st.docs, Error: st.err}
}

func (st *searchState) close() {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.ix != nil {
		st.ix.Close()
		st.ix = nil
	}
}

// SearchService 是全文检索服务:启动后台重建 + 查询 + 未就绪降级。
type SearchService struct{ a *App }

// Rebuild 触发一次后台重建;构建中重复调用被忽略。索引纯派生,
// 每次 Rebuild 都是整库重建(与 Phase 1 的 Reset 语义一致)。
func (s *SearchService) Rebuild() {
	go s.a.buildIndex()
}

// Status 索引状态(状态条)。
func (s *SearchService) Status() IndexStatus {
	return s.a.search.status()
}

// Query 全文检索。索引就绪走 FTS(bm25 排序);未就绪降级为逐会话
// 子串扫描(sessions.SearchSession,字节预筛,单会话快),前端按
// Reply.Mode 展示「快速搜索」横幅。
func (s *SearchService) Query(q string, limit int) (*SearchReply, error) {
	if limit <= 0 {
		limit = 20
	}
	st := s.a.search
	st.mu.Lock()
	ready := st.state == "ready" && st.ix != nil
	var ix *search.Index
	if ready {
		ix = st.ix
	}
	st.mu.Unlock()

	if ready {
		return s.queryIndex(ix, q, limit)
	}
	return s.queryScan(q, limit)
}

// queryIndex 全文索引路径。
func (s *SearchService) queryIndex(ix *search.Index, q string, limit int) (*SearchReply, error) {
	rs, err := ix.Search(context.Background(), q, limit)
	if err != nil {
		return nil, err
	}
	total, err := ix.Count(context.Background(), q)
	if err != nil {
		total = len(rs) // 计数失败退回所见,不假装是全部
	}
	_, byKey, _ := s.a.snapshot()
	out := make([]SearchHitView, 0, len(rs))
	for _, r := range rs {
		key, _, _ := splitDocKey(r.Key)
		sess := byKey[key]
		out = append(out, SearchHitView{
			Key:     key,
			Part:    r.Key,
			Agent:   sess.Agent,
			Title:   sess.Title,
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
	list, _, _ := s.a.snapshot()
	out := make([]SearchHitView, 0, limit)
	for _, sess := range list {
		if len(out) >= limit {
			break
		}
		if !sessions.HasTranscript(sess.Agent) || sess.Path == "" {
			continue
		}
		h, found, err := sessions.SearchSession(context.Background(), sess, needle)
		if err != nil || !found {
			continue
		}
		out = append(out, SearchHitView{
			Key:   sess.Agent + ":" + sess.ID,
			Part:  sess.Agent + ":" + sess.ID,
			Agent: sess.Agent, Title: sess.Title, Last: lastString(sess.Last),
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

// buildIndex 后台整库重建:Open→Reset→逐会话 TranscriptOf→批量 Add。
// 进度经 EventIndexProgress 推送,完成/失败推 EventIndexReady。
func (a *App) buildIndex() {
	st := a.search
	st.mu.Lock()
	if st.building {
		st.mu.Unlock()
		return
	}
	st.building = true
	st.state = "building"
	st.err = ""
	st.mu.Unlock()
	defer func() {
		st.mu.Lock()
		st.building = false
		st.mu.Unlock()
	}()

	ix, err := search.Open(st.path)
	if err != nil {
		a.failIndex(err)
		return
	}
	if err := ix.Reset(); err != nil {
		ix.Close()
		a.failIndex(err)
		return
	}
	// 换上新库(关旧的)
	st.mu.Lock()
	if st.ix != nil && st.ix != ix {
		st.ix.Close()
	}
	st.ix = ix
	st.mu.Unlock()

	a.ensure()
	list, _, _ := a.snapshot()
	docs := make([]search.Doc, 0, 64)
	sessionsN, docsN := 0, 0
	lastEmit := time.Now()
	for _, s := range list {
		if !sessions.HasTranscript(s.Agent) || s.Path == "" {
			continue
		}
		tr, err := sessions.TranscriptOf(s)
		if err != nil {
			continue
		}
		key := s.Agent + ":" + s.ID
		for i, p := range tr.Parts {
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
		if a.cfg.Emit != nil && time.Since(lastEmit) >= 500*time.Millisecond {
			lastEmit = time.Now()
			a.pushIndex(EventIndexProgress, sessionsN, docsN)
		}
	}
	st.mu.Lock()
	st.state = "ready"
	st.sessions, st.docs = sessionsN, docsN
	st.err = ""
	st.mu.Unlock()
	a.pushIndex(EventIndexReady, sessionsN, docsN)
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
