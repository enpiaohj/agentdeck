// Package app 是 AgentDeck 桌面壳的服务层：把 sessions（会话）、search
// （全文索引）、marks（用户标注）、projects（项目聚合）装配成 Wails
// service 暴露给前端。
//
// 本包不 import wails——窗口、事件与剪贴板经 Config 的回调注入，整层
// 可以脱离窗口环境做单元测试。唯一的 wails 触点在
// cmd/agentdeck-desktop 的装配处。
package app

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/enpiaohj/agentdeck/internal/appconfig"
	"github.com/enpiaohj/agentdeck/internal/claudehistory"
	"github.com/enpiaohj/agentdeck/internal/history"
	"github.com/enpiaohj/agentdeck/internal/marks"
	"github.com/enpiaohj/agentdeck/internal/projects"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// Config 是装配参数。零值的回调意味着对应能力不可用（Emit 为 nil 则
// 不发事件、Clipboard 为 nil 则复制报错），服务层对 nil 一律显式报错
// 或静默降级，绝不 panic。
type Config struct {
	// MarksPath 标注库路径；空则由应用配置解析（默认位置或已迁移的数据根）。
	MarksPath string
	// IndexPath 全文索引库路径；空同 MarksPath，":memory:" 用独立内存库。
	IndexPath string
	// ImportDir stores AgentDeck-owned chat imports, separate from native agents.
	ImportDir string
	// LibraryPath is permanent history, independent from the rebuildable index.
	LibraryPath string
	// ProjectsPath 声明项目清单库路径；空用默认（与 marks 同目录）。
	// 测试传 :memory: 得独立内存库。
	ProjectsPath string
	// Emit 前端事件推送；nil 时构建进度只落在 Status() 里。
	Emit Emitter
	// Clipboard 写剪贴板；nil 时复制动作报错。
	Clipboard func(text string) error
	// Runner 启动外部进程（恢复终端）；nil 时用 exec.Command.Start。
	Runner func(name string, args ...string) error
	// TerminalShell 是打开终端时使用的 PowerShell 可执行文件：全文取值见
	// appconfig（powershell / auto / pwsh 或自定义绝对路径）；空用应用配置。
	TerminalShell string
	// Restart 重启应用（迁移数据位置后让新落点生效）；nil 时界面只提示手动重启。
	Restart func() error
	// StartAtLoginStatus 查询操作系统中的实际自启动注册状态。
	StartAtLoginStatus func() (bool, error)
	// SetStartAtLogin 显式注册或移除自启动；与查询回调同时存在才支持设置。
	SetStartAtLogin func(bool) error
	// TrayAvailable 表示当前桌面环境已装配可用托盘。
	TrayAvailable bool
	// UpdateAPIURL 覆盖更新检查的接口地址；空用 GitHub 的正式接口。
	// 测试指向本机服务，正式运行不需要设置。
	UpdateAPIURL string
}

// App 是装配根：标注库句柄 + 会话列表缓存 + 索引状态。
type App struct {
	dataMu        sync.Mutex // serializes owned-data backup, restore, import and mark mutations
	cfg           Config
	marks         *marks.Store
	declared      *projects.Store // 用户声明的项目（尚无会话的目录）
	history       *history.Store
	captureMu     sync.Mutex // serialize reads and commits, including index/preview callers
	libraryError  string
	captureErrors map[string]string
	closing       bool // guarded by captureMu
	// storageNote 说明本次运行的数据落点是否与配置一致（配置损坏或配置根
	// 不可用时的降级原因）；空表示无异常。界面在「设置 → 数据与诊断」展示。
	storageNote string
	// 应用配置与终端 shell 设置。settingsErr 非空时界面提示并拒绝回写。
	paths        appconfig.Paths
	settingsMu   sync.Mutex
	settings     appconfig.Config
	settingsErr  error
	readNative   func(sessions.Session) (sessions.Transcript, error)
	deletePlans  map[string]*deleteJob                      // guarded by dataMu
	deleteUndos  map[string]*deleteJob                      // guarded by dataMu
	deleted      map[string]bool                            // guarded by mu
	nativeDelete func(*sessions.NativeDeletePlan) error     // fault injection for rollback tests
	codexName    func(threadID, name string) error          // 写 Codex 原生名；测试替换成假的，不去起真进程
	probeShell   func(exe string) (string, error)           // 探测 shell 版本；测试替换成假的，不去起真进程
	copyFileFn   func(src, dest string) (copyRecord, error) // 迁移复制单个文件；测试替换成会失败的那一个

	mu          sync.Mutex
	list        []sessions.Session // 装配缓存：sessions.List(All) 的快照
	byKey       map[string]sessions.Session
	marksAll    map[string]marks.Mark // 标注快照，MarkService 写后同步刷新
	messages    map[string]int        // agent:id → 消息数，后台补齐
	msgReady    bool
	loaded      bool
	crashed     bool // 上次进程未正常退出(启动时检测残留 lock)
	importError string

	warmMu     sync.Mutex // serialize message scans across refreshed generations
	generation uint64
	search     *searchState
	// shellMu 保护终端 shell 的当前取值与版本探测缓存（见 terminalshell）。
	shellMu      sync.Mutex
	shellSetting string
	shellProbes  map[string]shellVersion
	// migrateMu 保护数据位置迁移的独占权（同一时间只允许一次迁移）。
	migrateMu sync.Mutex
	migrating bool
	// version 是从 exe 的 VERSIONINFO 读出的产品版本（构建时由
	// build/windows/info.json 写入）；空表示开发版本。
	version string
	// updateMu 保护在线更新的检查/下载状态（见 updatesvc.go）。
	updateMu sync.Mutex
	update   updateState
	// singleRelease 释放单实例互斥锁；nil 表示本进程没有取得所有权。
	singleRelease func()
}

// New 打开标注库并返回装配根。索引不在此建——Rebuild 触发后台重建。
func New(cfg Config) (*App, error) {
	// 数据落点：调用方显式给出的路径优先，缺省项由应用配置解析
	// （默认位置或已迁移的数据根，见 appconfig）。
	resolved, err := resolveDataPaths()
	if err != nil {
		return nil, err
	}
	paths := resolved.paths
	if cfg.MarksPath == "" {
		cfg.MarksPath = paths.MarksPath
	}
	if cfg.IndexPath == "" {
		cfg.IndexPath = paths.IndexPath
	}
	if cfg.TerminalShell == "" {
		cfg.TerminalShell = resolved.cfg.TerminalShell
	}
	if cfg.ImportDir == "" {
		base := filepath.Dir(cfg.IndexPath)
		if cfg.IndexPath == ":memory:" {
			base = filepath.Dir(cfg.MarksPath)
		}
		cfg.ImportDir = filepath.Join(base, "imports", "claude")
	}
	if cfg.LibraryPath == "" {
		base := filepath.Dir(cfg.IndexPath)
		if cfg.IndexPath == ":memory:" {
			base = filepath.Dir(cfg.MarksPath)
		}
		cfg.LibraryPath = filepath.Join(base, "library", "history.db")
	}
	// 声明项目与 marks 一样是用户数据（Roaming），跟随它的目录；
	// 测试把 MarksPath 设成 :memory: 时没有目录可用，落临时目录。
	if cfg.ProjectsPath == "" {
		base := filepath.Dir(cfg.MarksPath)
		if cfg.MarksPath == ":memory:" {
			base = os.TempDir()
		}
		cfg.ProjectsPath = filepath.Join(base, "projects.db")
	}
	m, err := marks.Open(cfg.MarksPath)
	if err != nil {
		return nil, err
	}
	d, err := projects.Open(cfg.ProjectsPath)
	if err != nil {
		m.Close()
		return nil, fmt.Errorf("打开声明项目库失败: %w", err)
	}
	h, err := history.Open(cfg.LibraryPath)
	if err != nil {
		m.Close()
		d.Close()
		return nil, fmt.Errorf("打开永久会话库失败: %w", err)
	}
	deleted, err := loadDeletedKeys(cfg.LibraryPath)
	if err != nil {
		m.Close()
		d.Close()
		h.Close()
		return nil, fmt.Errorf("读取删除恢复状态失败：%w", err)
	}
	a := &App{
		cfg:           cfg,
		marks:         m,
		declared:      d,
		history:       h,
		search:        newSearchState(cfg.IndexPath),
		crashed:       checkCrash(cfg.MarksPath),
		paths:         pathsFromConfig(cfg),
		settings:      resolved.cfg,
		settingsErr:   resolved.cfgErr,
		storageNote:   resolved.note,
		shellSetting:  appconfig.NormalizeShell(cfg.TerminalShell),
		captureErrors: map[string]string{},
		readNative:    sessions.TranscriptForRetention,
		deleted:       deleted,
		codexName:     setCodexThreadName,
		probeShell:    probeShellVersion,
		version:       exeVersion(),
	}
	return a, nil
}

// singleInstanceMutexName 是本应用唯一实例的命名互斥锁名。与 session.lock
// 文件标记不同，它是所有权而非存在性：已有实例时第二个进程直接退出，
// session.lock 于是只在真正的异常退出后残留，不再被多开误判成崩溃。
const singleInstanceMutexName = "AgentDeck-SingleInstance"

// ClaimSingleInstance 供入口调用：已有一个实例在运行时返回 true，调用方
// 应聚焦已有窗口并退出。取得的实例所有权随 Close 释放。
func (a *App) ClaimSingleInstance() bool {
	release, already := claimSingleInstance(singleInstanceMutexName)
	if already {
		return true
	}
	a.singleRelease = release
	return false
}

// Close 关闭标注库与索引库,并清除崩溃标记(正常退出)。
func (a *App) Close() {
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	a.search.close()
	// An in-flight native read owns captureMu until its transaction commits.
	// Wait for it before closing permanent storage, then reject later captures.
	a.captureMu.Lock()
	a.closing = true
	a.history.Close()
	a.captureMu.Unlock()
	a.marks.Close()
	a.declared.Close()
	clearCrash(a.cfg.MarksPath)
	if a.singleRelease != nil {
		a.singleRelease()
		a.singleRelease = nil
	}
}

// 服务构造器:cmd 装配用(字段未导出,强制经此创建)。
func (a *App) NewSessionService() *SessionService { return &SessionService{a: a} }
func (a *App) NewSearchService() *SearchService   { return &SearchService{a: a} }
func (a *App) NewMarkService() *MarkService       { return &MarkService{a: a} }
func (a *App) NewResumeService() *ResumeService   { return &ResumeService{a: a} }
func (a *App) NewProjectService() *ProjectService {
	return &ProjectService{a: a}
}
func (a *App) NewSettingsService() *SettingsService { return &SettingsService{a: a} }
func (a *App) NewUpdateService() *UpdateService     { return &UpdateService{a: a} }

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
	imported, importErr := claudehistory.List(a.cfg.ImportDir)
	a.importError = ""
	if importErr != nil {
		a.importError = importErr.Error()
	} else {
		list = append(list, imported...)
	}
	// Retained-only rows keep the original key and marks but cannot advertise
	// native resume: a local transcript is not an upstream executable session.
	saved, historyErr := a.history.List()
	if historyErr != nil {
		a.libraryError = historyErr.Error()
	}
	live := make(map[string]bool, len(list))
	for _, sess := range list {
		live[sess.Agent+":"+sess.ID] = true
	}
	for _, sess := range saved {
		if live[sess.Agent+":"+sess.ID] {
			continue
		}
		sess.Retained, sess.ReadOnly, sess.Resume = true, true, ""
		list = append(list, sess)
	}
	list = slices.DeleteFunc(list, func(sess sessions.Session) bool { return a.deleted[sess.Agent+":"+sess.ID] })
	sort.SliceStable(list, func(i, j int) bool { return list[i].Last.After(list[j].Last) })
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
	a.warmMu.Lock()
	defer a.warmMu.Unlock()
	a.mu.Lock()
	if !a.loaded || a.msgReady {
		a.mu.Unlock()
		return
	}
	generation := a.generation
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
	if generation != a.generation {
		a.mu.Unlock()
		return
	}
	a.messages = msgs
	a.msgReady = true
	a.mu.Unlock()
	// 通知前端重拉列表:补齐前消息数一律显示 0
	a.emit(EventMessagesReady, nil)
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
	next := maps.Clone(a.marksAll)
	next[key] = m
	a.marksAll = next
	a.mu.Unlock()
}

// sessionMarkKey preserves marks when an explicitly identified Desktop run
// previously appeared as CLI. Existing canonical marks always take priority.
func sessionMarkKey(sess sessions.Session, key string, ms map[string]marks.Mark) string {
	if _, exists := ms[key]; exists || sess.Agent != "claude-desktop" || sess.WSL != "" || sess.Path == "" {
		return key
	}
	rel, err := filepath.Rel(filepath.Join(sessions.ClaudeDir(), "projects"), sess.Path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return key
	}
	legacy := "claude:" + sess.ID
	if _, exists := ms[legacy]; exists {
		return legacy
	}
	return key
}

func (a *App) markKey(key string) string {
	_, byKey, ms := a.snapshot()
	return sessionMarkKey(byKey[key], key, ms)
}

// session 最后活动时间的字符串形态；零值给空串，前端按未知渲染。
func lastString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
