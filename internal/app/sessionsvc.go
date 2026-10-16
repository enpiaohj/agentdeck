package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/projects"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Filter 是列表过滤条件。归档语义：Archived=false 看未归档（默认视图），
// true 只看归档；归档只是标注过滤，源文件零改动。
type Filter struct {
	Agent     string `json:"agent,omitempty"`
	Project   string `json:"project,omitempty"` // 归一化路径
	Tag       string `json:"tag,omitempty"`
	Favorites bool   `json:"favorites,omitempty"`
	Archived  bool   `json:"archived,omitempty"`
	Search    string `json:"search,omitempty"` // 标题子串（大小写折叠）
}

// SessionService 是会话列表/详情服务。
type SessionService struct{ a *App }

// List 按过滤条件返回会话视图（默认按最后活动降序，即 sessions 层顺序）。
func (s *SessionService) List(f Filter) []SessionView {
	list, _, ms := s.a.snapshot()
	s.a.warmMessages()
	s.a.mu.Lock()
	msgs, msgReady := s.a.messages, s.a.msgReady
	s.a.mu.Unlock()

	searchKey := foldTitle(f.Search)
	out := make([]SessionView, 0, 64)
	for _, sess := range list {
		key := sess.Agent + ":" + sess.ID
		m := ms[key]
		if f.Agent != "" && sess.Agent != f.Agent {
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
		if searchKey != "" && !strings.Contains(foldTitle(sess.Title), searchKey) {
			continue
		}
		out = append(out, view(sess, key, m, msgs, msgReady))
	}
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
	v := view(sess, key, ms[key], msgs, msgReady)
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
	tr, err := sessions.TranscriptOf(sess)
	if err != nil {
		return nil, err
	}
	tv := &TranscriptView{Key: key, Cut: tr.Cut, Parts: make([]TranscriptPart, 0, len(tr.Parts))}
	for _, p := range tr.Parts {
		tv.Parts = append(tv.Parts, TranscriptPart{
			Role: p.Role, Kind: p.Kind, Name: p.Name, Text: p.Text, Cut: p.Cut, Model: p.Model,
		})
	}
	return tv, nil
}

// SearchIn 单会话内检索（命中片段），走 sessions 层子串匹配。
func (s *SessionService) SearchIn(key, needle string) (*SearchHitView, error) {
	_, byKey, _ := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", key)
	}
	h, found, err := sessions.SearchSession(context.Background(), sess, sessions.NeedleOf(needle))
	if err != nil || !found {
		return nil, err
	}
	return &SearchHitView{
		Key: key, Agent: sess.Agent, Title: sess.Title, Last: lastString(sess.Last),
		Match: h.Match, Snippet: h.Before + h.Match + h.After,
	}, nil
}

// Agents 侧栏 agent 清单（计数含归档，前端按需展示）。
func (s *SessionService) Agents() []AgentView {
	list, _, _ := s.a.snapshot()
	count := map[string]int{}
	for _, sess := range list {
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

// Projects 项目聚合视图。
func (s *SessionService) Projects() []ProjectView {
	list, _, ms := s.a.snapshot()
	ps := projects.Group(list)
	out := make([]ProjectView, 0, len(ps))
	for _, p := range ps {
		fav := 0
		for _, k := range p.Keys {
			if ms[k].Favorite {
				fav++
			}
		}
		out = append(out, ProjectView{
			Path: p.Path, Name: p.Name, Drive: p.Drive,
			Count: len(p.Keys), Favorites: fav, Last: lastString(p.Last),
		})
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
	_, byKey, _ := s.a.snapshot()
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
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := sessions.WriteMarkdown(f, sess, sess.Title); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// ---- 内部 ----

// view 是 Session → SessionView 的唯一装配点。
func view(s sessions.Session, key string, m marks.Mark, msgs map[string]int, msgReady bool) SessionView {
	source := "local"
	if s.Gateway {
		source = "gateway"
	} else if s.WSL != "" {
		source = "wsl"
	}
	return SessionView{
		Key:     key,
		Agent:   s.Agent,
		ID:      s.ID,
		Cwd:     s.Cwd,
		Project: projects.DisplayName(projects.Normalize(s.Cwd)),
		Title:   s.Title,
		Last:    lastString(s.Last),
		Messages: func() int {
			if msgReady {
				return msgs[key]
			}
			return 0 // 补齐前降级显示 0
		}(),
		Resume:     s.Resume,
		Transcript: sessions.HasTranscript(s.Agent),
		Tokens:     s.Tokens.Input + s.Tokens.Output + s.Tokens.CacheRead + s.Tokens.CacheWrite,
		Cost:       s.Cost,
		Source:     source,
		ReadOnly:   s.ReadOnly,
		Favorite:   m.Favorite,
		Archived:   m.Archived,
		Tags:       m.Tags,
		HasNote:    m.Note != "",
		Note:       m.Note,
	}
}

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
