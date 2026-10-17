package sessions

import (
	"context"
	"net/url"
	"strings"
)

// Hermes's conversation, read from its state.db: a session's rows of the
// messages table in their id order — a prompt (role user), what a reply said
// (role assistant: its words, what it reasoned in reasoning_content, and the
// calls it made in tool_calls) and what a call answered (role tool, named by
// tool_name). The rows the listing leaves out are left out here too, since
// eachMessage is the reader both use: a row hidden by display metadata, and a
// row a compaction replaced, say nothing either way.
//
// A session's Path names the store and the session (hermesFiles):
// <state.db>#<escaped session id>, which is read back here. Hermes keeps
// every profile's sessions in a store of its own, so the path, not the id
// alone, is what says which database to read.

// hermesTranscript tells add what a Hermes session said, in message order.
func hermesTranscript(s Session, add func(bool, Part) bool) error {
	path, sid, ok := hermesPathSession(s.Path)
	if !ok {
		return ErrNoTranscript
	}
	dbReadMu.Lock()
	defer dbReadMu.Unlock()
	defer closeDBs()
	db := openDB(path)
	if db == nil {
		return ErrNoTranscript
	}
	// A read transaction, as parseHermes takes: one snapshot while the
	// agent writes.
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	more := true
	err = newHermesDB(db).eachMessage(tx, sid, func(m hermesMessage) {
		if !more {
			return
		}
		for _, p := range hermesParts(m) {
			if !add(p.Role != "assistant", p) {
				more = false
				return
			}
		}
	})
	if err != nil {
		return err
	}
	return nil
}

// hermesPathSession is the store and the session id a session's Path names.
func hermesPathSession(path string) (string, string, bool) {
	db, escaped, ok := strings.Cut(path, "#")
	if !ok || db == "" || escaped == "" {
		return "", "", false
	}
	sid, err := url.QueryUnescape(escaped)
	if err != nil || sid == "" {
		return "", "", false
	}
	return db, sid, true
}

// hermesParts are what one message row said.
func hermesParts(m hermesMessage) []Part {
	switch m.role {
	case "user":
		if text := hermesText(m.content); strings.TrimSpace(text) != "" {
			return []Part{textPart("user", text)}
		}
	case "assistant":
		var parts []Part
		// what it reasoned is said before what it answered, as the reply
		// itself was written
		if t := strings.TrimSpace(m.thinking); t != "" {
			parts = append(parts, Part{Role: "assistant", Kind: "thinking", Text: t})
		}
		if text := hermesText(m.content); strings.TrimSpace(text) != "" {
			parts = append(parts, textPart("assistant", text))
		}
		for _, tool := range hermesTools(m.tools) {
			parts = append(parts, Part{Role: "assistant", Kind: "tool_use", Name: tool.Function.Name, Text: kimiArgs(tool.Function.Arguments)})
		}
		return parts
	case "tool":
		if text := hermesText(m.content); strings.TrimSpace(text) != "" {
			return []Part{{Role: "tool", Kind: "tool_result", Name: m.toolName, Text: text}}
		}
	}
	return nil
}
