package app

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const conversationPage = 80

// Keep source indexes contiguous across windows so FTS hits remain stable.
// Latest windows are filled backwards and clip long prose at its beginning,
// ensuring the actual last words are visible without rendering an entire log.
func conversationWindow(full sessions.Transcript, at int, before bool) *TranscriptView {
	out := &TranscriptView{Parts: []TranscriptPart{}, Total: len(full.Parts), Cut: full.Cut, Prompts: promptMarks(full.Parts)}
	if out.Total == 0 {
		return out
	}
	latest := at < 0
	start, end := max(0, min(at, out.Total-1)), out.Total
	if latest {
		start = max(0, end-conversationPage)
	} else if before {
		end = min(max(0, at), out.Total)
		start = max(0, end-conversationPage)
	} else {
		end = min(start+conversationPage, end)
	}
	size := 0
	appendPart := func(p sessions.Part) {
		view := TranscriptPart{Role: p.Role, Kind: p.Kind, Name: p.Name, Text: p.Text, Cut: p.Cut, Model: p.Model}
		if n := utf8.RuneCountInString(p.Text); n > 8000 {
			runes := []rune(p.Text)
			if latest && p.Kind == "text" && (p.Role == "user" || p.Role == "assistant") {
				view.Text = string(runes[n-8000:])
				view.OmittedBefore = n - 8000
			} else {
				view.Text = string(runes[:8000])
				view.Cut += n - 8000
			}
		}
		size += len(view.Text)
		out.Parts = append(out.Parts, view)
	}
	if latest || before {
		for i := end - 1; i >= start; i-- {
			if size >= 400000 {
				start = i + 1
				break
			}
			appendPart(full.Parts[i])
		}
		slices.Reverse(out.Parts)
	} else {
		for i := start; i < end; i++ {
			if size >= 400000 {
				end = i
				break
			}
			appendPart(full.Parts[i])
		}
	}
	out.Start, out.HasBefore, out.HasAfter = start, start > 0, end < out.Total
	return out
}

// promptMarkRunes 是提问摘要的长度上限：导航条悬停只需认出是哪一问。
const promptMarkRunes = 80

// promptMarks 列出用户的每一次提问：用户的正文段，空白段不算；摘要取第一行
// 非空文字，过长截断。
func promptMarks(parts []sessions.Part) []PromptMark {
	out := []PromptMark{}
	for i, p := range parts {
		if p.Role != "user" || p.Kind != "text" {
			continue
		}
		text := ""
		for _, line := range strings.Split(p.Text, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				text = line
				break
			}
		}
		if text == "" {
			continue
		}
		if r := []rune(text); len(r) > promptMarkRunes {
			text = string(r[:promptMarkRunes]) + "…"
		}
		out = append(out, PromptMark{Index: i, Text: text})
	}
	return out
}

// TranscriptWindow reads the latest window (-1), the beginning (0), or a
// window beginning at a source part index. before reads backwards ending just
// before at, so byte-limited pages remain adjacent. Saved history is unchanged.
func (s *SessionService) TranscriptWindow(key string, at int, before bool) (*TranscriptView, error) {
	_, byKey, _ := s.a.snapshot()
	sess, ok := byKey[key]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", key)
	}
	if at < -1 {
		return nil, fmt.Errorf("无效阅读位置: %d", at)
	}
	full, retained, warning, err := s.a.readConversation(sess)
	if err != nil {
		return nil, err
	}
	view := conversationWindow(full, at, before)
	view.Key, view.Retained, view.StorageWarning = key, retained, warning
	return view, nil
}
