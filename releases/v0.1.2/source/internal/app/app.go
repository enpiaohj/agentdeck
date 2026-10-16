// Package app 是 AgentDeck 桌面壳的服务层：把 sessions（会话）、search
// （全文索引）、marks（用户标注）、projects（项目聚合）装配成 Wails
// service 暴露给前端。
//
// 本包不 import wails——窗口、事件与剪贴板经 Config 的回调注入，整层
// 可以脱离窗口环境做单元测试。唯一的 wails 触点在
// cmd/agentdeck-desktop 的装配处。
package app

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Config 是装配参数。零值的回调意味着对应能力不可用（Emit 为 nil 则
// 不发事件、Clipboard 为 nil 则复制报错），服务层对 nil 一律显式报错
// 或静默降级，绝不 panic。
type Config struct {
	// MarksPath 标注库路径；空用 marks.DefaultPath()。
	MarksPath string
	// IndexPath 全文索引库路径；空用内存索引（构建结果随进程而去）。
	IndexPath string
	// Emit 前端事件推送；nil 时构建进度只落在 Status() 里。
	Emit Emitter
	// Clipboard 写剪贴板；nil 时复制动作报错。
	Clipboard func(text string) error
	// Runner 启动外部进程（恢复终端）；nil 时用 exec.Command.Start。
	Runner func(name string, args ...string) error
}

// App 是装配根：标注库句柄 + 会话列表缓存 + 索引状态。
type App struct {
	cfg   Config
	marks *marks.Store

	mu       sync.Mutex
	list     []sessions.Session // 装配缓存：sessions.List(All) 的快照
	byKey    map[string]sessions.Session
	marksAll map[string]marks.Mark // 标注快照，MarkService 写后同步刷新
	messages map[string]int        // agent:id → 消息数，后台补齐
	msgReady bool
	loaded   bool
	crashed  bool // 上次进程未正常退出(启动时检测残留 lock)

	warmOnce sync.Once
	search   *searchState
}

// New 打开标注库并返回装配根。索引不在此建——Rebuild 触发后台重建。
func New(cfg Config) (*App, error) {
	if cfg.MarksPath == "" {
		p, err := marks.DefaultPath()
		if err != nil {
			return nil, err
		}
		cfg.MarksPath = p
	}
	if cfg.IndexPath == "" {
		p, err := indexDefaultPath()
		if err != nil {
			return nil, err
		}
		cfg.IndexPath = p
	}
	m, err := marks.Open(cfg.MarksPath)
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, marks: m, search: newSearchState(cfg.IndexPath), crashed: checkCrash(cfg.MarksPath)}
	return a, nil
}

// indexDefaultPath 是索引库的默认落盘位置:%LOCALAPPDATA%\AgentDeck\
// index.db(缓存性质的用户数据,Local 而非 Roaming)。
func indexDefaultPath() (string, error) {
	base, err := os.UserCacheDir() // Windows 上即 %LOCALAPPDATA%
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "AgentDeck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.db"), nil
}

// Close 关闭标注库与索引库,并清除崩溃标记(正常退出)。
func (a *App) Close() {
	a.search.close()
	a.marks.Close()
	clearCrash(a.cfg.MarksPath)
}

// 服务构造器:cmd 装配用(字段未导出,强制经此创建)。
func (a *App) NewSessionService() *SessionService { return &SessionService{a: a} }
func (a *App) NewSearchService() *SearchService   { return &SearchService{a: a} }
func (a *App) NewMarkService() *MarkService       { return &MarkService{a: a} }
func (a *App) NewResumeService() *ResumeService   { return &ResumeService{a: a} }

// emit nil 安全的事件推送。
func (a *App) emit(event string, data any) {
	if a.cfg.Emit != nil {
		a.cfg.Emit(event, data)
	}
}

// ensure 装配列表缓存。首次调用承担 sessions 层的全量扫描（它自己的
// 缓存让这次扫描只发生一次），之后只是取快照。sessions.List 必须显式
// 传 sessions.All——List(0) 只是最新 200 条（陷阱 #1）。
func (a *App) ensure() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loaded {
		return
	}
	list := sessions.List(sessions.All)
	byKey := make(map[string]sessions.Session, len(list))
	for _, s := range list {
		byKey[s.Agent+":"+s.ID] = s
	}
	a.list = list
	a.byKey = byKey
	// 标注读不出来时按无标注降级，不阻塞列表
	ms, err := a.marks.All()
	if err != nil {
		ms = map[string]marks.Mark{}
	}
	a.marksAll = ms
	a.loaded = true
}

// setList 是测试注入口：绕过对真实机器的扫描，直接给定列表快照。
func (a *App) setList(list []sessions.Session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.list = list
	a.byKey = map[string]sessions.Session{}
	for _, s := range list {
		a.byKey[s.Agent+":"+s.ID] = s
	}
	a.marksAll = map[string]marks.Mark{}
	a.loaded = true
}

// warmMessages 后台补齐消息数：ListAgent 每 agent 全量走文件，慢。
// 补齐前列表的 Messages 显示 0（设计允许的降级态）。
func (a *App) warmMessages() {
	a.warmOnce.Do(func() {
		a.mu.Lock()
		if !a.loaded {
			a.mu.Unlock()
			return
		}
		agents := map[string]bool{}
		for _, s := range a.list {
			agents[s.Agent] = true
		}
		a.mu.Unlock()
		msgs := map[string]int{}
		for ag := range agents {
			for _, m := range sessions.ListAgent(ag) {
				msgs[m.Agent+":"+m.ID] = m.Messages
			}
		}
		a.mu.Lock()
		a.messages = msgs
		a.msgReady = true
		a.mu.Unlock()
		// 通知前端重拉列表:补齐前消息数一律显示 0
		a.emit(EventMessagesReady, nil)
	})
}

// snapshot 在锁内取列表与标注快照的浅拷贝引用：list 与 byKey 装配后
// 只读，marksAll 由 MarkService 写时换值，读侧拿到的引用始终完整。
func (a *App) snapshot() ([]sessions.Session, map[string]sessions.Session, map[string]marks.Mark) {
	a.ensure()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.list, a.byKey, a.marksAll
}

// refreshMark 把一个 key 的最新标注读进缓存；写路径在 MarkService。
func (a *App) refreshMark(key string) {
	m, err := a.marks.Get(key)
	if err != nil {
		return
	}
	a.mu.Lock()
	if a.marksAll == nil {
		a.marksAll = map[string]marks.Mark{}
	}
	a.marksAll[key] = m
	a.mu.Unlock()
}

// session 最后活动时间的字符串形态；零值给空串，前端按未知渲染。
func lastString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
