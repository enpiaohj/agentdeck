package sessions

import (
	"encoding/json"
	"testing"
)

// The call this was reported from (messages.id = 15864, session
// 20260828_103350_0742e3ff, 「主模型切换完成通知」): the reply itself is a
// note, and the card the agent sent exists only here, as a Python literal.
func TestHermesCallCardTextFromExecuteCode(t *testing.T) {
	args := `{"code":"import json, subprocess\ncard = {\n  \"config\": {\"wide_screen_mode\": True},\n` +
		`  \"elements\": [\n    {\"tag\": \"markdown\", \"content\": \"**✅ QG 补好了**\"},\n` +
		`    {\"tag\": \"hr\"},\n    {\"tag\": \"markdown\", \"content\": \"最终状态快照\"}\n  ]\n}\n` +
		`subprocess.run([\"python\", \"card_send.py\", json.dumps(card, ensure_ascii=False)])\n"}`
	want := "**✅ QG 补好了**\n\n最终状态快照"
	if got := hermesCallCardText([]byte(args)); got != want {
		t.Fatalf("card text: %q, want %q", got, want)
	}
}

// The other shape Hermes sends cards in: a terminal call passing JSON through
// a heredoc (messages.id = 11154). Its shell quoting is not the card's, so the
// literal has to be read by the decoder rather than by counting quotes.
func TestHermesCallCardTextFromHeredoc(t *testing.T) {
	args := `{"command":"python card_send.py \"$(cat << 'PYEOF'\n{\n` +
		`  \"config\": { \"wide_screen_mode\": true },\n  \"elements\": [\n` +
		`    {\n      \"tag\": \"markdown\",\n      \"content\": \"**🏆 世界杯决赛结果**\"\n    },\n` +
		`    { \"tag\": \"hr\" }\n  ]\n}\nPYEOF\n)\""}`
	want := "**🏆 世界杯决赛结果**"
	if got := hermesCallCardText([]byte(args)); got != want {
		t.Fatalf("card text: %q, want %q", got, want)
	}
}

func TestHermesCallCardText(t *testing.T) {
	tests := []struct {
		name, args, want string
	}{
		{
			name: "python trailing comma",
			args: `{"code":"card = {\"config\": {}, \"elements\": [{\"tag\": \"markdown\", \"content\": \"甲\"},],}"}`,
			want: "甲",
		},
		{
			name: "wrapper object around the card",
			args: `{"code":"payload = {\"type\": \"card\", \"card\": {\"elements\": [{\"tag\": \"markdown\", \"content\": \"乙\"}]}}"}`,
			want: "乙",
		},
		{
			name: "unrecognized element keeps the call as it is",
			args: `{"code":"card = {\"config\": {}, \"elements\": [{\"tag\": \"mystery\"}]}"}`,
		},
		{
			name: "ordinary code with a dict",
			args: `{"code":"config = {\"timeout\": 30}\nprint(config)"}`,
		},
		{
			name: "a name that only looks like a keyword",
			args: `{"code":"TrueType = {\"config\": {\"font\": True}"}`,
		},
		{
			// The store's own cut (messages.id = 13379): the literal ends inside a
			// string, so nothing of the card is left to read.
			name: "arguments cut off by the store",
			args: `{"code":"card = {\"config\": {}, \"elements\": [{\"tag\": \"markdown\", \"content\": \"**Azure GPU 计费... [truncated]"}`,
		},
		{
			// A cut after a whole element leaves that element readable on its own.
			name: "cut after a whole element",
			args: `{"code":"card = {\"config\": {}, \"elements\": [{\"tag\": \"markdown\", \"content\": \"甲\"}]... [truncated]"}`,
			want: "甲",
		},
		{
			name: "no code at all",
			args: `{"path":"notes.md"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hermesCallCardText([]byte(tt.args)); got != tt.want {
				t.Fatalf("card text: %q, want %q", got, tt.want)
			}
		})
	}
}

// A card sent by a call is a part of the reply that follows the call, so that
// the body is read as text rather than as code inside it.
func TestHermesPartsAddsTheCardACallSent(t *testing.T) {
	m := hermesMessage{
		role:    "assistant",
		content: "卡片已经发出去了。",
		tools:   hermesToolsJSON(t, `card = {"elements": [{"tag": "markdown", "content": "正文"}]}`),
	}
	parts := hermesParts(m)
	if len(parts) != 3 {
		t.Fatalf("parts = %d, want text, tool_use and card: %#v", len(parts), parts)
	}
	if parts[1].Kind != "tool_use" || parts[2].Kind != "card" {
		t.Fatalf("kinds: %q, %q", parts[1].Kind, parts[2].Kind)
	}
	if parts[2].Text != "正文" || parts[2].Role != "assistant" {
		t.Fatalf("card part: %#v", parts[2])
	}
}

func TestHermesPartsKeepsCallsWithoutCards(t *testing.T) {
	m := hermesMessage{
		role:    "assistant",
		content: "先看看目录。",
		tools:   hermesToolsJSON(t, `ls -la`),
	}
	if parts := hermesParts(m); len(parts) != 2 {
		t.Fatalf("parts = %d, want text and tool_use: %#v", len(parts), parts)
	}
}

// hermesToolsJSON is the tool_calls column of an execute_code call whose code
// is given: the arguments are a string of JSON inside the call, as OpenAI's
// shape has them.
func hermesToolsJSON(t *testing.T, code string) string {
	t.Helper()
	args, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		t.Fatal(err)
	}
	tools, err := json.Marshal([]map[string]any{{
		"function": map[string]any{"name": "execute_code", "arguments": string(args)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(tools)
}
