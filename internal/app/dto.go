package app

// 全部前端可见的 DTO。刻意与 sessions.Session 分开：Session 内嵌
// Tokens、ModelEffort 切片与 time.Time，对 bindings 生成器是坑；这里
// 全部显式扁平字段，Last 一类时间用 RFC3339 字符串，前端本地化渲染。

// SessionView 是列表与会话卡片的一行。Resume 非空 = 可原生命令恢复；
// 空 = 仅可查看历史。Source 区分本机/WSL/网关来源。
type SessionView struct {
	Key        string   `json:"key"` // agent:id
	Agent      string   `json:"agent"`
	ID         string   `json:"id"`
	Cwd        string   `json:"cwd"`
	Project    string   `json:"project"` // 归一化后的项目展示名
	Title      string   `json:"title"`
	Last       string   `json:"last"` // RFC3339
	Messages   int      `json:"messages"`
	Resume     string   `json:"resume"`
	Transcript bool     `json:"transcript"`
	Tokens     int      `json:"tokens"`
	Cost       float64  `json:"cost"`
	Source     string   `json:"source"` // local | wsl | gateway
	ReadOnly   bool     `json:"read_only"`
	Favorite   bool     `json:"favorite"`
	Archived   bool     `json:"archived"`
	Tags       []string `json:"tags"`
	HasNote    bool     `json:"has_note"`
	Note       string   `json:"note"`
}

// TranscriptView 是会话详情：全文按序。Cut 说明会话层按上限截断了。
type TranscriptView struct {
	Key   string           `json:"key"`
	Parts []TranscriptPart `json:"parts"`
	Cut   bool             `json:"cut"`
}

// TranscriptPart 是一句：Role user/assistant/tool，Kind text/thinking/
// tool_use/tool_result/context/image。
type TranscriptPart struct {
	Role  string `json:"role"`
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
	Text  string `json:"text"`
	Cut   int    `json:"cut,omitempty"`
	Model string `json:"model,omitempty"`
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
	Path      string `json:"path"` // 归一化代表路径
	Name      string `json:"name"`
	Drive     string `json:"drive"`
	Count     int    `json:"count"`
	Favorites int    `json:"favorites"`
	Last      string `json:"last"` // RFC3339
}

// IndexStatus 是全文索引的状态条数据。State: idle | building | ready |
// failed。Sessions 是已索引会话数，Docs 是收录的正文块数。
type IndexStatus struct {
	State    string `json:"state"`
	Sessions int    `json:"sessions"`
	Docs     int    `json:"docs"`
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
