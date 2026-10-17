package app

import (
	"fmt"
	"sort"
	"strings"

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
