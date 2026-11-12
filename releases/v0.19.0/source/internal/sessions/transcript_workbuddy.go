package sessions

import (
	"cmp"
	"encoding/json"
	"strings"
)

// WorkBuddy's conversation, read whole from a session's own <id>.jsonl: the
// items the OpenAI Agents SDK writes, in the order they were written —
// message (a user's, its input_text blocks; a reply's, its output_text), and
// the reply's own reasoning (rawContent's reasoning_text), function_call
// (name and arguments) and function_call_result (name, output.text). A
// reply's items name the model they were answered by in providerData, as the
// summary reader reads it (workbuddyLine).
//
// The words WorkBuddy puts in beside the user's (a <system-reminder>, an
// @scene: reference) are left out, as the summary reader leaves them out
// (wbPrompt); a whole message that is one is not said at all.

// wbItem is a line of a WorkBuddy session: one item, in one of the shapes
// above. What is not an item the SDK writes (ai-title, custom-title,
// file-history-snapshot) has no type read here and says nothing.
type wbItem struct {
	Type         string          `json:"type"`
	Role         string          `json:"role"`
	Name         string          `json:"name"`
	Content      json.RawMessage `json:"content"`    // a message's blocks
	RawContent   json.RawMessage `json:"rawContent"` // a reasoning's blocks
	Arguments    json.RawMessage `json:"arguments"`  // a call's, as a string of JSON
	Output       *wbBlock        `json:"output"`     // a result's text
	ProviderData struct {
		Model        string `json:"model"`
		RequestModel string `json:"requestModelId"`
	} `json:"providerData"`
}

// wbBlock is one block of what an item carries: its kind and its words.
type wbBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// workBuddyTranscript tells add what a WorkBuddy session said, in the order
// of its file.
func workBuddyTranscript(path string, add func(bool, Part) bool) error {
	_, err := scanAt(path, 0, nil, func(b []byte, _, _ int64) bool {
		var it wbItem
		if json.Unmarshal(b, &it) != nil {
			return true
		}
		for _, p := range it.parts() {
			if !add(p.Role != "assistant", p) {
				return false
			}
		}
		return true
	})
	return err
}

// parts are what one item said: the model is the one its reply was answered
// by, said with a reply's own parts (#2253).
func (it wbItem) parts() []Part {
	model := cmp.Or(it.ProviderData.Model, it.ProviderData.RequestModel)
	say := func(parts []Part) []Part {
		for i := range parts {
			if parts[i].Role == "assistant" {
				parts[i].Model = model
			}
		}
		return parts
	}
	switch it.Type {
	case "message":
		if it.Role != "user" && it.Role != "assistant" {
			return nil
		}
		var parts []Part
		for _, b := range wbBlocks(it.Content) {
			switch {
			case it.Role == "user" && wbTyped(b.Type):
				if t := wbSaid(b.Text); t != "" {
					parts = append(parts, textPart("user", t))
				}
			case it.Role == "assistant" && (b.Type == "output_text" || b.Type == "text"):
				parts = append(parts, textPart("assistant", b.Text))
			}
		}
		return say(parts)
	case "reasoning":
		var parts []Part
		for _, b := range wbBlocks(it.RawContent) {
			if b.Type == "reasoning_text" {
				parts = append(parts, Part{Role: "assistant", Kind: "thinking", Text: b.Text})
			}
		}
		return say(parts)
	case "function_call":
		return say([]Part{{Role: "assistant", Kind: "tool_use", Name: it.Name, Text: kimiArgs(it.Arguments)}})
	case "function_call_result":
		if it.Output == nil {
			return nil
		}
		return []Part{{Role: "tool", Kind: "tool_result", Name: it.Name, Text: it.Output.Text}}
	}
	return nil
}

// wbTyped is whether a user message's block is one the user typed in: the
// SDK's input_text, and the plain text an older line carries instead.
func wbTyped(kind string) bool { return kind == "input_text" || kind == "text" || kind == "" }

// wbSaid is the words typed in a user message's block. WorkBuddy wraps them
// in what it puts in itself — a <system-reminder> of the workspace, the mood
// and the tools — and hands its own message over at the end of it, in a
// <user_query> tag; a block that carries none is either the words themselves
// or one of WorkBuddy's alone (@scene:…, a reminder with nothing asked), and
// says nothing either way.
func wbSaid(text string) string {
	if i := strings.Index(text, wbQueryOpen); i >= 0 {
		rest := text[i+len(wbQueryOpen):]
		if j := strings.Index(rest, wbQueryClose); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
		return strings.TrimSpace(rest)
	}
	t := strings.TrimSpace(text)
	if t == "" || strings.HasPrefix(t, "<") || wbScene.MatchString(t) {
		return ""
	}
	return t
}

// wbQueryOpen and wbQueryClose are what WorkBuddy wraps the user's own
// message in.
const (
	wbQueryOpen  = "<user_query>"
	wbQueryClose = "</user_query>"
)

// wbBlocks are the blocks of an item's content, which is a list of them, or
// the words alone.
func wbBlocks(raw json.RawMessage) []wbBlock {
	if len(raw) == 0 {
		return nil
	}
	var blocks []wbBlock
	if json.Unmarshal(raw, &blocks) == nil {
		return blocks
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []wbBlock{{Type: "text", Text: text}}
	}
	// a block that is an object with no list around it (older lines)
	var one wbBlock
	if json.Unmarshal(raw, &one) == nil && (one.Type != "" || one.Text != "") {
		return []wbBlock{one}
	}
	return nil
}
