package app

// Emitter 把服务层的事件推给前端。internal/app 不 import wails——
// 装配处（cmd/agentdeck-desktop）注入 wails 的 EmitEvent 包装，测试里
// 注入收集器。data 一律是可序列化的 DTO。
type Emitter func(event string, data any)

// 事件名常量：前端经 wails EventsOn 订阅。
const (
	// EventIndexProgress 全文索引构建中，data 是 IndexStatus。
	EventIndexProgress = "index:progress"
	// EventIndexReady 全文索引构建完成（或失败），data 是 IndexStatus。
	EventIndexReady = "index:ready"
	// EventSessionsChanged 标注（收藏/归档/标签/备注）发生变化，
	// data 是变化的会话 key（string）。
	EventSessionsChanged = "sessions:changed"
)
