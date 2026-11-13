package sessions

import "testing"

func TestHermesTextFlattensFeishuCard(t *testing.T) {
	// The row this was reported from (messages.id = 3331, session
	// 20260704_225217_1171924f): a reply Hermes stored as the card it was
	// delivered in. Its words are inside the element, so the JSON shell must
	// not be what the reader gets.
	raw := `{"tag": "div", "text": {"tag": "lark_md", "content": "我叫 **Hermes Agent**（赫尔墨斯代理），由 **Nous Research** 打造！🤖\n\n你可以叫我 Hermes，一个专注帮你干活的 AI 代理。"}}`
	want := "我叫 **Hermes Agent**（赫尔墨斯代理），由 **Nous Research** 打造！🤖\n\n你可以叫我 Hermes，一个专注帮你干活的 AI 代理。"
	if got := hermesText(raw); got != want {
		t.Fatalf("card text: %q", got)
	}
}

func TestHermesCardText(t *testing.T) {
	tests := []struct {
		name, raw, want string
		ok              bool
	}{
		{
			name: "envelope",
			raw: `{"schema":"2.0","config":{"wide_screen_mode":true},` +
				`"header":{"title":{"tag":"plain_text","content":"构建结果"}},` +
				`"body":{"elements":[` +
				`{"tag":"div","text":{"tag":"lark_md","content":"全部通过"}},` +
				`{"tag":"hr"},` +
				`{"tag":"column_set","columns":[{"tag":"column","elements":[` +
				`{"tag":"div","text":{"tag":"lark_md","content":"耗时 1 分"}}]}]}]}}`,
			want: "构建结果\n\n全部通过\n\n耗时 1 分",
			ok:   true,
		},
		{
			name: "v1 elements without a body",
			raw:  `{"config":{},"elements":[{"tag":"markdown","content":"第一行"},{"tag":"note","elements":[{"tag":"plain_text","content":"备注"}]}]}`,
			want: "第一行\n\n备注",
			ok:   true,
		},
		{
			name: "bare element array",
			raw:  `[{"tag":"div","text":{"tag":"lark_md","content":"甲"}},{"tag":"div","text":{"tag":"lark_md","content":"乙"}}]`,
			want: "甲\n\n乙",
			ok:   true,
		},
		{
			name: "mention and image",
			raw:  `{"tag":"div","fields":[{"tag":"at","user_id":"@_user_1","user_name":"张三"},{"tag":"img","img_key":"k","alt":{"tag":"plain_text","content":"截图"}}]}`,
			want: "@张三\n\n截图",
			ok:   true,
		},
		{
			name: "unknown element keeps the payload",
			raw:  `{"config":{},"elements":[{"tag":"div","text":{"tag":"lark_md","content":"x"}},{"tag":"mystery","payload":1}]}`,
			ok:   false,
		},
		{
			name: "unknown element tag on its own",
			raw:  `{"tag":"form","name":"f"}`,
			ok:   false,
		},
		{
			name: "text element without content",
			raw:  `{"tag":"div","text":{"tag":"lark_md"}}`,
			ok:   false,
		},
		{
			name: "ordinary object",
			raw:  `{"name":"item"}`,
			ok:   false,
		},
		{
			name: "ordinary array of objects",
			raw:  `[{"type":"text","text":"literal"}]`,
			ok:   false,
		},
		{
			name: "card-shaped but empty",
			raw:  `{"tag":"hr"}`,
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := hermesCardText(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tt.ok, got)
			}
			if ok && got != tt.want {
				t.Fatalf("text: %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHermesTextKeepsUnrecognizedCards(t *testing.T) {
	raw := `{"config":{},"elements":[{"tag":"mystery"}]}`
	if got := hermesText(raw); got != raw {
		t.Fatalf("unrecognized card became %q", got)
	}
}
