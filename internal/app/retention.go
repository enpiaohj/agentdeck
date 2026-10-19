package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// LibraryStatus reports permanent storage, not the rebuildable FTS cache.
type LibraryStatus struct {
	Path     string `json:"path"`
	Sessions int    `json:"sessions"`
	Versions int    `json:"versions"`
	Bytes    int64  `json:"bytes"`
	Error    string `json:"error"`
}

// LibraryCompact 是一次历史库压缩的结果：压缩前后占用、压了多少个版本、
// 用了多久。全部是字节与计数，前端自己格式化。
type LibraryCompact struct {
	Before   int64 `json:"before"`
	After    int64 `json:"after"`
	Versions int   `json:"versions"`
	Elapsed  int64 `json:"elapsed"`
}

// CompactLibrary 把旧版留下的明文正文压缩，并回收文件空间。只改存储
// 形态，不改内容：每一行写回前都解压比对过。可重复执行，没得可压时
// 返回零改动。进度经 EventStorageProgress 推给前端（stage=compact）。
func (s *SessionService) CompactLibrary() (LibraryCompact, error) {
	a := s.a
	// closing 由 captureMu 保护（见 App.Close）。只读一眼标志，不在整个
	// 压缩期间占着它——那会把所有会话读取一起挡住。
	a.captureMu.Lock()
	closing := a.closing
	a.captureMu.Unlock()
	if closing {
		return LibraryCompact{}, fmt.Errorf("程序正在关闭，未开始压缩")
	}
	start := time.Now()
	before, err := a.history.Stats()
	if err != nil {
		return LibraryCompact{}, fmt.Errorf("读取历史库规模失败: %w", err)
	}
	var last time.Time
	after, err := a.history.Compact(func(done, total int64) {
		// 每批只有 16 行，全推会把前端刷屏；400ms 一次足够看出在动。
		if now := time.Now(); now.Sub(last) >= 400*time.Millisecond || done >= total {
			last = now
			a.emit(EventStorageProgress, StorageProgress{
				Stage: "compact", Done: done, Total: total,
				Message: "正在压缩历史库", Elapsed: int64(time.Since(start).Seconds()),
			})
		}
	})
	if err != nil {
		return LibraryCompact{}, fmt.Errorf("压缩历史库失败: %w", err)
	}
	a.emit(EventStorageProgress, StorageProgress{
		Stage: "compact", Done: 1, Total: 1,
		Message: "历史库压缩完成", Elapsed: int64(time.Since(start).Seconds()),
	})
	return LibraryCompact{Before: before.Bytes, After: after.Bytes, Versions: after.Versions, Elapsed: int64(time.Since(start).Seconds())}, nil
}

func (s *SessionService) LibraryStatus() LibraryStatus {
	stats, err := s.a.history.Stats()
	s.a.mu.Lock()
	message := s.a.libraryError
	failed := []string{}
	for key, reason := range s.a.captureErrors {
		failed = append(failed, key+": "+reason)
	}
	s.a.mu.Unlock()
	sort.Strings(failed)
	if len(failed) > 0 {
		message = strings.TrimSpace(message + "\n" + strings.Join(failed, "\n"))
	}
	if err != nil {
		message = err.Error()
	}
	return LibraryStatus{Path: s.a.cfg.LibraryPath, Sessions: stats.Sessions, Versions: stats.Versions, Bytes: stats.Bytes, Error: message}
}

// readConversation is shared by preview, indexing, scanning and export. It
// commits parsed content before returning it. Failed/empty native reads never
// replace a saved revision. Only AgentDeck's own database is written.
func (a *App) readConversation(sess sessions.Session) (sessions.Transcript, bool, string, error) {
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	if a.closing {
		return sessions.Transcript{}, false, "", fmt.Errorf("程序正在关闭，未开始新的会话读取")
	}
	a.mu.Lock()
	deleted := a.deleted[sess.Agent+":"+sess.ID]
	a.mu.Unlock()
	if deleted {
		return sessions.Transcript{}, false, "", fmt.Errorf("此会话已被手动删除")
	}
	var tr sessions.Transcript
	var err error
	if !sess.Retained {
		tr, err = a.readNative(sess)
		if err == nil && len(tr.Parts) > 0 {
			sess.MessageCount = 0
			for _, p := range tr.Parts {
				if p.Kind == "text" && (p.Role == "user" || p.Role == "assistant") {
					sess.MessageCount++
				}
			}
			saveErr := a.history.Save(sess, tr)
			warning, key := "", sess.Agent+":"+sess.ID
			a.mu.Lock()
			if saveErr != nil {
				warning = fmt.Sprintf("会话未能持久保存: %v", saveErr)
				a.captureErrors[key] = warning
			} else {
				delete(a.captureErrors, key)
			}
			a.mu.Unlock()
			return tr, false, warning, nil
		}
	}
	_, saved, found, savedErr := a.history.Get(sess.Agent + ":" + sess.ID)
	if savedErr != nil {
		return tr, false, "", fmt.Errorf("读取永久会话库失败: %w", savedErr)
	}
	if found {
		return saved, true, "", nil
	}
	if err == nil && sess.Retained {
		err = fmt.Errorf("本地保留的正文不存在")
	}
	return tr, false, "", err
}
