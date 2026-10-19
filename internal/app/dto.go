package app

// 全部前端可见的 DTO。刻意与 sessions.Session 分开：Session 内嵌
// Tokens、ModelEffort 切片与 time.Time，对 bindings 生成器是坑；这里
// 全部显式扁平字段，Last 一类时间用 RFC3339 字符串，前端本地化渲染。

// SessionView 是列表与会话卡片的一行。Resume 非空 = 可原生命令恢复；
// 空 = 仅可查看历史。Source 区分本机/WSL/网关来源。
type SessionView struct {
	Role          string   `json:"role"`       // confirmed main | subagent | unknown
	ParentKey     string   `json:"parent_key"` // original parent, when known
	GroupKey      string   `json:"group_key"`  // present main in the current list
	Account       string   `json:"account"`    // imported cloud-chat account label
	Key           string   `json:"key"`        // agent:id
	Agent         string   `json:"agent"`
	ID            string   `json:"id"`
	Cwd           string   `json:"cwd"`
	Project       string   `json:"project"` // 归一化后的项目展示名
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title"`
	Last          string   `json:"last"`        // RFC3339
	Created       string   `json:"created"`     // RFC3339;零值空串
	SourcePath    string   `json:"source_path"` // 会话源文件路径(详情「源路径」,只读展示)
	Messages      int      `json:"messages"`
	Resume        string   `json:"resume"`
	Transcript    bool     `json:"transcript"`
	Tokens        int      `json:"tokens"`
	Cost          float64  `json:"cost"`
	Source        string   `json:"source"` // local | wsl | gateway
	ReadOnly      bool     `json:"read_only"`
	Favorite      bool     `json:"favorite"`
	Archived      bool     `json:"archived"`
	Tags          []string `json:"tags"`
	HasNote       bool     `json:"has_note"`
	Note          string   `json:"note"`
}

type ClaudeImportReport struct {
	Conversations   int    `json:"conversations"`
	Account         string `json:"account"`
	AlreadyImported bool   `json:"already_imported"`
	SkippedBlocks   int    `json:"skipped_blocks"`
}

// TranscriptView 是会话详情：全文按序。Cut 说明会话层按上限截断了。
type TranscriptView struct {
	Start          int              `json:"start"`
	Total          int              `json:"total"`
	HasBefore      bool             `json:"has_before"`
	HasAfter       bool             `json:"has_after"`
	Retained       bool             `json:"retained"`
	StorageWarning string           `json:"storage_warning"`
	Key            string           `json:"key"`
	Parts          []TranscriptPart `json:"parts"`
	Cut            bool             `json:"cut"`
}

// TranscriptPart 是一句：Role user/assistant/tool，Kind text/thinking/
// tool_use/tool_result/context/image。
type TranscriptPart struct {
	OmittedBefore int    `json:"omitted_before"`
	Role          string `json:"role"`
	Kind          string `json:"kind"`
	Name          string `json:"name,omitempty"`
	Text          string `json:"text"`
	Cut           int    `json:"cut,omitempty"`
	Model         string `json:"model,omitempty"`
}

// SearchReply 是一次全文检索：Mode 说明命中来自全文索引（index）还是
// 逐会话降级扫描（scan，索引构建中）。Total 是真实命中数，Hits 是按
// limit 截断后的展示列表。
type SearchReply struct {
	Mode  string          `json:"mode"`
	Total int             `json:"total"`
	Hits  []SearchHitView `json:"hits"`
}

// SearchHitView 是一条命中：Snippet 从原文切取（Excerpt），Match 是
// 高亮词（Anchor，与 Snippet 的锚同一规则）。
type SearchHitView struct {
	Key     string `json:"key"`  // 会话 key（agent:id），跳转用
	Part    string `json:"part"` // 完整 doc key（agent:id#n）
	Agent   string `json:"agent"`
	Title   string `json:"title"`
	Last    string `json:"last"` // RFC3339
	Match   string `json:"match"`
	Snippet string `json:"snippet"`
}

// ProjectView 是聚合出的项目一行。
type ProjectView struct {
	Agent     string `json:"agent"` // 保存的默认 CLI；空表示按本机会话回退
	Path      string `json:"path"`  // 归一化代表路径
	Name      string `json:"name"`
	Drive     string `json:"drive"`
	Count     int    `json:"count"`
	Favorites int    `json:"favorites"`
	Archived  int    `json:"archived"` // 已归档的会话；不计入 Count，供悬停提示说明
	Last      string `json:"last"`     // RFC3339
	Declared  bool   `json:"declared"`
}

// IndexStatus 是全文索引的状态条数据。State: idle | building | ready |
// failed。Sessions 是已索引会话数，Docs 是收录的正文块数。
// Total 是这批会话里待索引的总数（building 时给进度一个分母），
// 未开始或无法统计时为 0，此时前端只显示已索引数。
type IndexStatus struct {
	State    string `json:"state"`
	Sessions int    `json:"sessions"`
	Docs     int    `json:"docs"`
	Total    int    `json:"total"`
	Error    string `json:"error,omitempty"`
}

// AgentView 是侧栏的 agent 清单一行。
type AgentView struct {
	Agent string `json:"agent"`
	Count int    `json:"count"`
}

// ScanProgress 是会话文件扫描（sessions 层）的进度，首次列表装配期间
// 前端轮询它画进度条。
type ScanProgress struct {
	Indexing bool  `json:"indexing"`
	Files    int64 `json:"files"`
	Done     int64 `json:"done"`
}

// TerminalShellOption 是「终端 PowerShell」设置页的一个预置选项。
type TerminalShellOption struct {
	Value   string `json:"value"`   // powershell | auto | pwsh
	Exe     string `json:"exe"`     // 解析出的可执行文件；空 = 未找到
	Version string `json:"version"` // 自报版本；空 = 未探测到
	Error   string `json:"error"`   // 不可用原因；空 = 可用
}

// TerminalShellView 是终端 shell 设置的当前状态：当前取值、它解析出的
// 可执行文件与版本，以及三个预置选项的可用性。
type TerminalShellView struct {
	Setting string                `json:"setting"` // 当前取值；自定义时为绝对路径
	Exe     string                `json:"exe"`
	Version string                `json:"version"`
	Error   string                `json:"error"`
	Options []TerminalShellOption `json:"options"`
}

// StorageItem 是数据存储位置的一项占用。
type StorageItem struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	Files   int    `json:"files"`
	Present bool   `json:"present"`
	// Rebuild 为真表示可重建、迁移时不复制（全文索引）。
	Rebuild bool `json:"rebuild"`
	// Note 说明该项的用途或处理方式。
	Note string `json:"note"`
}

// StorageView 是「数据与诊断 → 数据存储位置」的完整状态。
type StorageView struct {
	// Root 是配置的数据根；空表示使用默认位置。
	Root         string `json:"root"`
	UsingDefault bool   `json:"using_default"`
	ConfigDir    string `json:"config_dir"`
	LocalDir     string `json:"local_dir"`
	// Leaf 是数据根的固定相对路径：迁移时取所选目录下的这个子目录。
	Leaf string `json:"leaf"`
	// DefaultConfigDir 与 DefaultLocalDir 是默认位置，仅在已迁移时有别于当前值。
	DefaultConfigDir string `json:"default_config_dir"`
	DefaultLocalDir  string `json:"default_local_dir"`
	// DefaultHasData 说默认位置当前是否已有 AgentDeck 数据（改回默认位置
	// 前要据此提示，不能静默造一个空库）。
	DefaultHasData bool          `json:"default_has_data"`
	Items          []StorageItem `json:"items"`
	TotalBytes     int64         `json:"total_bytes"`
	// FreeBytes 是当前数据根所在盘的剩余空间；无法获取时为 0。
	FreeBytes int64 `json:"free_bytes"`
	// PrevDirs 是上一次迁移前的数据目录；非空表示可以清理旧位置。
	PrevDirs []string `json:"prev_dirs"`
	// PrevInUse 为真表示旧位置仍是本次运行正在使用的目录（迁移后尚未重启），
	// 此时不能清理。
	PrevInUse bool          `json:"prev_in_use"`
	PrevItems []StorageItem `json:"prev_items"`
	PrevBytes int64         `json:"prev_bytes"`
	// Note 是启动时的降级说明（配置损坏或配置根不可用）。
	Note string `json:"note"`
	// Busy 表示正在迁移。
	Busy bool `json:"busy"`
	// Restartable 表示可以「立即重启」（装配处注入了重启回调）。
	Restartable bool `json:"restartable"`
}

// StorageProgress 是迁移进度，经 storage:progress 推送。
// Stage: scan | copy | verify | switch。
type StorageProgress struct {
	Stage     string `json:"stage"`
	Done      int64  `json:"done"`
	Total     int64  `json:"total"`
	Files     int    `json:"files"`
	DoneFiles int    `json:"done_files"`
	Message   string `json:"message"`
}

// MigrateReport 是一次数据位置迁移的结果。
type MigrateReport struct {
	Dest    string   `json:"dest"`
	Files   int      `json:"files"`
	Bytes   int64    `json:"bytes"`
	Skipped []string `json:"skipped"`
	Message string   `json:"message"`
}

// DestinationView 是补全后的迁移目标与它当前的状态：界面在用户确认之前就能
// 显示数据究竟会落在哪个目录、那里是不是已经有目录，而不是点下「开始迁移」
// 之后才被拒绝。
type DestinationView struct {
	// Picked 是用户选定的目录。
	Picked string `json:"picked"`
	// Dest 是补全后的数据根：固定取所选目录下的 AgentDeck\Data；所选目录
	// 本身就叫 AgentDeck 时只追加 Data。
	Dest string `json:"dest"`
	// Leaf 是固定的相对路径，Rule 是补全规则的一句话说明（含不重复追加的
	// 例外）。两者都由 Go 侧给出，界面不自己拼。
	Leaf string `json:"leaf"`
	Rule string `json:"rule"`
	// Exists 表示 Dest 目录已存在。
	Exists bool `json:"exists"`
	// Empty 表示 Dest 不存在或存在但为空（没有会被覆盖的内容）。
	Empty bool `json:"empty"`
	// Files 与 Bytes 是 Dest 已存在时里面的文件数与占用字节数。
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
	// Ready 表示可以直接迁移：校验通过，且目录不存在或为空。
	Ready bool `json:"ready"`
	// Reason 说明不可用的原因（Ready 为假时非空）。
	Reason string `json:"reason"`
}

// CleanupPlan 是「清理旧位置数据」的预览：将被逐个删除的文件清单。
// Token 绑定清单内容，执行前重新校验，清单变了就整批拒绝。
type CleanupPlan struct {
	Token string   `json:"token"`
	Dirs  []string `json:"dirs"`
	Files int      `json:"files"`
	Bytes int64    `json:"bytes"`
	// Other 是旧位置里不属于 AgentDeck 的条目，不会被删除，也不会阻止清理。
	Other []string `json:"other"`
}

// CleanupReport 是清理结果。
type CleanupReport struct {
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	Dirs    int    `json:"dirs"`
	Message string `json:"message"`
}
