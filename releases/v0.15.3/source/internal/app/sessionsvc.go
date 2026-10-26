package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/claudehistory"
	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/projects"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Filter 是列表过滤条件。归档语义：Archived=false 看未归档（默认视图），
// true 只看归档；归档只是标注过滤，源文件零改动。
type Filter struct {
	Agent string `json:"agent,omitempty"`
	// Agents 多选智能体（侧栏 Ctrl+单击叠加）；非空时只看这些，Agent 一并计入
	Agents    []string `json:"agents,omitempty"`
	Project   string   `json:"project,omitempty"` // 归一化路径
	Tag       string   `json:"tag,omitempty"`
	Favorites bool     `json:"favorites,omitempty"`
	Archived  bool     `json:"archived,omitempty"`
	Search    string   `json:"search,omitempty"` // 标题子串（大小写折叠）
}

// SessionService 是会话列表/详情服务。
type SessionService struct{ a *App }

// ImportClaudeHistory copies an official chat export into the owned local store.
// Account is a user label, not a login credential. No Claude source is changed.
func (s *SessionService) ImportClaudeHistory(path, account string) (ClaudeImportReport, error) {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	report, err := claudehistory.Import(s.a.cfg.ImportDir, path, account)
	if err != nil {
		return ClaudeImportReport{}, err
	}
	s.Refresh()
	s.a.mu.Lock()
	loadError := s.a.importError
	s.a.mu.Unlock()
	if loadError != "" {
		return ClaudeImportReport{}, fmt.Errorf("文件已保存，但读取导入库失败: %s", loadError)
	}
	return ClaudeImportReport{Conversations: report.Conversations, Account: report.Account, AlreadyImported: report.AlreadyImported, SkippedBlocks: report.SkippedBlocks}, nil
}

// Refresh rereads sources without touching transcripts or user marks.
// Old background message counts cannot overwrite the new generation.
func (s *SessionService) Refresh() {
	a := s.a
	a.mu.Lock()
	a.loaded = false
	a.generation++
	a.messages, a.msgReady = nil, false
	a.mu.Unlock()
	a.ensure()
	a.emit(EventSessionsChanged, "")
}

// List 按过滤条件返回会话视图（默认按最后活动降序，即 sessions 层顺序）。
func (s *SessionService) List(f Filter) []SessionView {
	list, _, ms := s.a.snapshot()
	// 异步补齐消息数:同步会阻塞首次 List 到全部 agent 的 ListAgent 走完
	go s.a.warmMessages()
	s.a.mu.Lock()
	msgs, msgReady := s.a.messages, s.a.msgReady
	s.a.mu.Unlock()

	searchKey := foldTitle(f.Search)
	names := s.declaredNames()
	agents := map[string]bool{}
	for _, ag := range append(slices.Clone(f.Agents), f.Agent) {
		if ag != "" {
			agents[ag] = true
		}
	}
	out := make([]SessionView, 0, 64)
	for _, sess := range list {
		key := sess.Agent + ":" + sess.ID
		m := ms[sessionMarkKey(sess, key, ms)]
		if len(agents) > 0 && !agents[sess.Agent] {
			continue
		}
		if f.Archived != m.Archived { // 归档视图只看归档，默认视图只看未归档
			continue
		}
		if f.Favorites && !m.Favorite {
			continue
		}
		if f.Tag != "" && !containsTag(m.Tags, f.Tag) {
			continue
		}
		if f.Project != "" && !strings.EqualFold(projects.Normalize(sess.Cwd), projects.Normalize(f.Project)) {
			continue
		}
		if searchKey != "" && !strings.Contains(foldTitle(displayTitle(sess, m)), searchKey) {
			continue
		}
		out = append(out, view(sess, key, m, msgs, msgReady, names))
	}
	applySessionFamilies(out, sessionFamilyRoots(list))
	return out
}

// Get 单个会话视图。
func (s *SessionService) Get(key string) (*SessionView, error) {
	_, byKey, ms := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", key)
	}
	s.a.mu.Lock()
	msgs, msgReady := s.a.messages, s.a.msgReady
	s.a.mu.Unlock()
	v := view(sess, key, ms[sessionMarkKey(sess, key, ms)], msgs, msgReady, s.declaredNames())
	return &v, nil
}

// Transcript 会话全文。
func (s *SessionService) Transcript(key string) (*TranscriptView, error) {
	_, byKey, _ := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", key)
	}
	if !sessions.HasTranscript(sess.Agent) || sess.Path == "" {
		return nil, sessions.ErrNoTranscript
	}
	full, retained, warning, err := s.a.readConversation(sess)
	if err != nil {
		return nil, err
	}
	tr := sessions.PreviewTranscript(full)
	tv := &TranscriptView{Key: key, Cut: tr.Cut, Retained: retained, StorageWarning: warning, Parts: make([]TranscriptPart, 0, len(tr.Parts))}
	for _, p := range tr.Parts {
		tv.Parts = append(tv.Parts, TranscriptPart{
			Role: p.Role, Kind: p.Kind, Name: p.Name, Text: p.Text, Cut: p.Cut, Model: p.Model,
		})
	}
	return tv, nil
}

// SearchIn 单会话内检索（命中片段），走 sessions 层子串匹配。
func (s *SessionService) SearchIn(key, needle string) (*SearchHitView, error) {
	hits, err := s.SearchInAll(key, needle)
	if err != nil || len(hits) == 0 {
		return nil, err
	}
	return &hits[0], nil
}

// SearchInAll 按原始段顺序返回所有命中段，定位编号与 TranscriptWindow 一致。
// 每段只返回一个摘要，避免长工具输出中的重复词占满导航。
func (s *SessionService) SearchInAll(key, needle string) ([]SearchHitView, error) {
	_, byKey, ms := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", key)
	}
	out := make([]SearchHitView, 0)
	needle = sessions.NeedleOf(needle)
	if needle == "" {
		return out, nil
	}
	tr, _, _, err := s.a.readConversation(sess)
	if err != nil {
		return nil, err
	}
	for i := range tr.Parts {
		part := tr
		part.Parts = tr.Parts[i : i+1]
		h, found := sessions.SearchRetainedTranscript(part, needle)
		if found {
			out = append(out, SearchHitView{
				Key: key, Part: fmt.Sprintf("%s#%d", key, i), Agent: sess.Agent,
				Title: displayTitle(sess, ms[sessionMarkKey(sess, key, ms)]), Last: lastString(sess.Last), Match: h.Match,
				Snippet: h.Before + h.Match + h.After,
				Start:   h.Start, End: h.End,
				SnippetStart: utf16Count(h.Before), SnippetEnd: utf16Count(h.Before + h.Match),
			})
		}
	}
	return out, nil
}

// Agents 侧栏 agent 清单。每个数字都是「点进去能看到几行」的数：子会话收在
// 主会话下不重复计，已归档的会话要走到「归档」里才看得到，因此不算在内。
func (s *SessionService) Agents() []AgentView {
	list, _, ms := s.a.snapshot()
	roots := sessionFamilyRoots(list)
	count := map[string]int{}
	for _, sess := range list {
		key := sess.Agent + ":" + sess.ID
		if roots[key] != "" {
			continue
		}
		if ms[sessionMarkKey(sess, key, ms)].Archived {
			continue
		}
		count[sess.Agent]++
	}
	out := make([]AgentView, 0, len(count))
	for ag, n := range count {
		out = append(out, AgentView{Agent: ag, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Agent < out[j].Agent
	})
	return out
}

// NavCounts 是侧栏三条入口上的数字：未归档的会话、其中的收藏、以及归档。
// 归档是一个维度而不是第二个集合，所以「全部会话」不含归档会话——点了
// 归档行才看到它们；三个数字各自对着点击后的列表。
type NavCounts struct {
	Sessions  int `json:"sessions"`
	Favorites int `json:"favorites"`
	Archived  int `json:"archived"`
}

// Counts 统计侧栏入口数量，口径与列表一致（子会话收在主会话下不重复计）。
func (s *SessionService) Counts() NavCounts {
	list, _, ms := s.a.snapshot()
	roots := sessionFamilyRoots(list)
	out := NavCounts{}
	for _, sess := range list {
		key := sess.Agent + ":" + sess.ID
		if roots[key] != "" {
			continue
		}
		m := ms[sessionMarkKey(sess, key, ms)]
		if m.Archived {
			out.Archived++
			continue
		}
		out.Sessions++
		if m.Favorite {
			out.Favorites++
		}
	}
	return out
}

// Projects 项目聚合视图：会话聚合出来的项目，与用户声明的项目（还没有
// 任何会话的目录）按路径合并。
//
// 同一路径两处都有时只出一行：用聚合出的统计数据（会话数、收藏、最后
// 活动）填上，Declared 标为真——声明只贡献「这个项目是用户特意建的」，
// 不该把真有会话的那个挤掉，也不该让它显示成 0 个会话。
// 声明的排在前面：它往往是用户刚建、最想立刻看到的那一个。
func (s *SessionService) Projects() []ProjectView {
	list, byKey, ms := s.a.snapshot()
	declared, err := s.a.declared.All()
	if err != nil {
		declared = nil // 读不出来不影响聚合结果，只是少了声明的那几个
	}
	aggregated := map[string]ProjectView{} // 小写归一化路径 → 聚合行
	order := make([]string, 0, len(list))  // 聚合顺序（Group 已按最后活动降序）
	for _, p := range projects.Group(list) {
		key := strings.ToLower(p.Path)
		if _, dup := aggregated[key]; dup {
			continue
		}
		fav, arch := 0, 0
		members := make([]sessions.Session, 0, len(p.Keys))
		for _, k := range p.Keys {
			members = append(members, byKey[k])
		}
		roots := sessionFamilyRoots(members)
		count := 0
		for _, k := range p.Keys {
			if roots[k] != "" {
				continue
			}
			m := ms[sessionMarkKey(byKey[k], k, ms)]
			// 与列表同口径：归档会话要看「归档」那一栏，不计入项目行的会话数，
			// 但单独记一个数，好让悬停提示把话说完。
			if m.Archived {
				arch++
				continue
			}
			count++
			if m.Favorite {
				fav++
			}
		}
		aggregated[key] = ProjectView{
			Path: p.Path, Name: p.Name, Drive: p.Drive,
			Count: count, Favorites: fav, Archived: arch, Last: lastString(p.Last),
		}
		order = append(order, key)
	}
	out := make([]ProjectView, 0, len(declared)+len(aggregated))
	seen := map[string]bool{}
	for _, d := range declared {
		key := strings.ToLower(projects.Normalize(d.Path))
		if seen[key] {
			continue
		}
		seen[key] = true
		row := ProjectView{
			Path: d.Path, Name: d.Name, Agent: d.Agent, Drive: projects.Drive(d.Path), Declared: true,
		}
		if agg, ok := aggregated[key]; ok {
			row.Count, row.Favorites, row.Archived, row.Last = agg.Count, agg.Favorites, agg.Archived, agg.Last
		}
		out = append(out, row)
	}
	// 聚合部分保持 Group 的顺序（按最后活动降序），不能遍历 map。
	for _, key := range order {
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, aggregated[key])
	}
	return out
}

// ScanProgress 会话文件扫描进度（首装/首次列表时轮询）。
func (s *SessionService) ScanProgress() ScanProgress {
	p := sessions.Indexing()
	return ScanProgress{Indexing: p.Indexing, Files: p.Files, Done: p.Done}
}

// ExportMarkdown 把会话导出为 Markdown 文件，写到 destDir 目录，返回
// 文件全路径。这是桌面端唯一一处写用户盘（导出目录由前端对话框传入）。
func (s *SessionService) ExportMarkdown(key, destDir string) (string, error) {
	_, byKey, ms := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return "", fmt.Errorf("会话不存在: %s", key)
	}
	dir := projects.Normalize(destDir)
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("导出目录必须是绝对路径: %s", destDir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("导出目录不存在: %s", dir)
	}
	name := fmt.Sprintf("%s-%s-%s.md", time.Now().Format("20060102-150405"), sess.Agent, safeFile(sess.ID))
	tr, _, _, err := s.a.readConversation(sess)
	if err != nil {
		return "", err
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	sess.Title = displayTitle(sess, ms[sessionMarkKey(sess, key, ms)])
	if err := sessions.WriteRetainedMarkdown(f, sess, sess.Agent, tr); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// ---- 内部 ----

func displayTitle(s sessions.Session, m marks.Mark) string {
	if m.Title != "" {
		return m.Title
	}
	return s.Title
}

// view 是 Session → SessionView 的唯一装配点。
func view(s sessions.Session, key string, m marks.Mark, msgs map[string]int, msgReady bool, names map[string]string) SessionView {
	source := "local"
	if s.Retained {
		source = "retained"
	} else if s.Gateway {
		source = "gateway"
	} else if s.Cached {
		source = "cache"
	} else if s.Imported {
		source = "import"
	} else if s.WSL != "" {
		source = "wsl"
	}
	return SessionView{
		Role: s.Role,
		ParentKey: func() string {
			if s.Agent == "codex" && s.Role == "subagent" && s.ParentSession != "" {
				return "codex:" + s.ParentSession
			}
			return ""
		}(),
		Key:           key,
		Agent:         s.Agent,
		Account:       s.Account,
		ID:            s.ID,
		Cwd:           s.Cwd,
		Project:       projectLabel(s.Cwd, names),
		Title:         displayTitle(s, m),
		OriginalTitle: s.Title,
		Last:          lastString(s.Last),
		Created:       lastString(s.Start),
		SourcePath:    s.Path,
		Messages: func() int {
			if s.Imported || s.Retained {
				return s.MessageCount
			}
			if msgReady {
				return msgs[key]
			}
			return 0 // 补齐前降级显示 0
		}(),
		Resume:          s.Resume,
		Transcript:      sessions.HasTranscript(s.Agent),
		Tokens:          s.Tokens.Input + s.Tokens.Output + s.Tokens.CacheRead + s.Tokens.CacheWrite,
		Cost:            s.Cost,
		Source:          source,
		ReadOnly:        s.ReadOnly,
		UpstreamDeleted: s.UpstreamDeleted,
		Favorite:        m.Favorite,
		Archived:        m.Archived,
		Tags:            m.Tags,
		HasNote:         m.Note != "",
		Note:            m.Note,
	}
}

// declaredNames 读用户声明的项目显示名称（小写归一化路径 → 名称）；读不出来
// 就当没有声明，会话仍按目录名显示。
func (s *SessionService) declaredNames() map[string]string {
	all, err := s.a.declared.All()
	if err != nil || len(all) == 0 {
		return nil
	}
	out := make(map[string]string, len(all))
	for _, d := range all {
		p := projects.Normalize(d.Path)
		if p == "" || strings.TrimSpace(d.Name) == "" {
			continue
		}
		out[strings.ToLower(p)] = d.Name
	}
	return out
}

// projectLabel 是会话所属项目的显示名：声明过就用用户在项目列表里设的名称，
// 否则用目录名。让列表、详情栏与项目区指向同一个名字，避免「改了项目名，
// 会话列表却还是旧名」。
func projectLabel(cwd string, names map[string]string) string {
	p := projects.Normalize(cwd)
	if p == "" {
		return ""
	}
	if name := names[strings.ToLower(p)]; name != "" {
		return name
	}
	return projects.DisplayName(p)
}

// containsTag 判断标签是否已在列表里。
func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// foldTitle 标题折叠比较键。
func foldTitle(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// safeFile 会话 id 的文件名安全化：只留白名单字符，其余转 _。
func safeFile(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
