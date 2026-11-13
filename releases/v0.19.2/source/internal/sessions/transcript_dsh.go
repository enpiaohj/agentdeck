package sessions

import (
	"encoding/json"
	"strings"
)

// DeepSeek Harness's (dsh's) conversation, read whole from a session's own
// file in the order its events were written (dsh.go reads the same file for
// its summary, and dshOpen decompresses it: from format 4 on a session's file
// is session.v4.jsonl.zstd).
//
// A line says something of the conversation when it carries words: a
// "user/message" (the words typed in when its source's kind is "user"; what
// dsh puts in itself — a runtime context, a tool job's news, the skill
// catalog, a time context, an approval answer — is shown folded, as Claude's
// reminders are), an "assistant/message" (its blocks in their order: reasoning,
// the words it said, and a tool-call naming the tool and carrying its
// arguments as a string of JSON), and a "tool/result" (what a tool answered).
//
// What is passed over: the lines dsh writes about itself rather than the
// conversation — the system prompt ("system/message"), the messages spliced
// into the agent's inbox ("agent/inbox/spliced"), the commands it ran
// ("command/run", "command/done"), the jobs it registered and settled
// ("job/*"), the approvals it asked for and was given ("approval/*"), what it
// set up ("permission/preset", "sandbox/mode", "model/selection",
// "request/*"), the turn and step boundaries, the storage's own records
// ("session-log-*", "workspace/changes"), and the session's title. A reply's
// streamed deltas ("stream") are passed over too: the message's own blocks are
// the assembled form of them.
//
// A fork's file begins with the events it was seeded with, copied from the
// session it came from; they are read, as the conversation the fork goes on
// from (as Kimi's records are).
//
// A tool result's line names no tool of its own: it carries only the call's id
// (message.source.callId), so the calls already read are remembered by id and
// the name is taken from that.

// dshSaid is one event line: only the fields of it the conversation is read
// from.
type dshSaid struct {
	Type string `json:"type"`
	Data struct {
		Content json.RawMessage `json:"content"` // a user message's blocks
		Source  struct {
			Kind string `json:"kind"`
		} `json:"source"`
		Message struct {
			Content json.RawMessage `json:"content"`
			Source  struct {
				CallID string `json:"callId"`
				Model  string `json:"model"`
			} `json:"source"`
		} `json:"message"`
	} `json:"data"`
}

// dshBlock is one block of what a line carries: its kind and its words, and,
// of a call, the tool's name, the call's id and its arguments — a string of
// JSON, which kimiArgs lays out.
type dshBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// dshTranscript tells add what a dsh session said, in the order of its file.
func dshTranscript(path string, keep func([]byte) bool, add func(bool, Part) bool) error {
	calls := map[string]string{} // a call's id: its tool's name
	_, err := scanAt(path, 0, nil, func(b []byte, _, _ int64) bool {
		if keep != nil && !keep(b) {
			return true
		}
		var l dshSaid
		if json.Unmarshal(b, &l) != nil {
			return true
		}
		for _, p := range l.parts(calls) {
			if !add(p.Role != "assistant", p) {
				return false
			}
		}
		return true
	})
	return err
}

// parts are what one line said, in the order its blocks have it; a call's name
// is remembered by its id for the result that answers it.
func (l dshSaid) parts(calls map[string]string) []Part {
	switch l.Type {
	case "user/message":
		own := l.Data.Source.Kind == "user"
		var parts []Part
		for _, b := range dshBlocks(l.Data.Content) {
			if b.Type != "text" {
				continue
			}
			if own {
				parts = append(parts, textPart("user", b.Text))
			} else {
				parts = append(parts, Part{Role: "user", Kind: "context", Text: b.Text})
			}
		}
		return parts
	case "assistant/message":
		model := l.Data.Message.Source.Model
		var parts []Part
		for _, b := range dshBlocks(l.Data.Message.Content) {
			switch b.Type {
			case "text":
				parts = append(parts, textPart("assistant", b.Text))
			case "reasoning":
				parts = append(parts, Part{Role: "assistant", Kind: "thinking", Text: b.Text})
			case "tool-call":
				if b.ID != "" {
					calls[b.ID] = b.Name
				}
				parts = append(parts, Part{Role: "assistant", Kind: "tool_use", Name: b.Name, Text: kimiArgs(b.Arguments)})
			}
		}
		// the model the reply came back from, said with the reply's own parts
		for i := range parts {
			if parts[i].Role == "assistant" {
				parts[i].Model = model
			}
		}
		return parts
	case "tool/result":
		return []Part{{
			Role: "tool", Kind: "tool_result",
			Name: calls[l.Data.Message.Source.CallID],
			Text: dshResultText(dshBlocks(l.Data.Message.Content)),
		}}
	}
	return nil
}

// dshBlocks are the blocks of what a line carries, which is a list of them (a
// string of JSON, in the older formats that carry one) or the whole content of
// it as words.
func dshBlocks(raw json.RawMessage) []dshBlock {
	if len(raw) == 0 {
		return nil
	}
	var blocks []dshBlock
	if json.Unmarshal(raw, &blocks) == nil {
		return blocks
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []dshBlock{{Type: "text", Text: text}}
	}
	return nil
}

// dshResultText is what a tool answered: its text blocks in order, an image
// said as such.
func dshResultText(blocks []dshBlock) string {
	var out []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			out = append(out, b.Text)
		case "image":
			out = append(out, "[image]")
		}
	}
	return strings.Join(out, "\n")
}
