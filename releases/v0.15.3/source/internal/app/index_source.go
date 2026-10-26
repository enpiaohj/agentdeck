package app

// 增量索引的「这个会话变了吗」判据。
//
// 会话层不缓存文件的 size/mtime（sessions.Session 没有这两个字段），所以
// 判据记在索引库里：doc_sources 表按会话标识存上次索引时的 size 与
// mtime，两者都没变就跳过重新解析。Codex / Claude 的历史会话都是只追加
// 的，这个判据足以覆盖「又写了几轮」。
//
// 刻意保守：任一侧取不到（文件不存在、记录缺失、保留副本没有上游文件）
// 都按「变了」处理。误判「没变」会让索引静默漏掉新内容，而漏内容不报错，
// 所以只允许反方向的误判。

import (
	"os"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// sourceStamp 是一个会话文件在一次索引时刻的状态。
type sourceStamp struct {
	size  int64
	mtime time.Time
}

// stampOf 取会话文件当前的状态；取不到时返回零值，调用方按「已变化」处理。
func stampOf(path string) sourceStamp {
	info, err := os.Stat(path)
	if err != nil {
		return sourceStamp{}
	}
	return sourceStamp{size: info.Size(), mtime: info.ModTime()}
}

// unchanged 回答「这个会话自上次索引后是否没有写入」。now 由调用方取好
// 传进来：调用方要先 stat 一遍才能决定跳过，再 stat 一次纯属重复。
// 保留副本没有上游文件，永远按已变化处理——它的正文在永久库里，由保留库
// 自己的版本机制保证变化可见。
func unchanged(sess sessions.Session, now, previous sourceStamp) bool {
	if sess.Retained || sess.Path == "" {
		return false
	}
	if now.size == 0 {
		return false
	}
	return now.size == previous.size && now.mtime.Equal(previous.mtime)
}
