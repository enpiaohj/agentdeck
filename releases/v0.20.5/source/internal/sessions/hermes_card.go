package sessions

import (
	"encoding/json"
	"strings"
)

// hermesCardMaxBytes bounds card detection. A card of a chat message is a few
// hundred bytes; anything this large is a tool result or a document, and
// unmarshalling it into a tree on every transcript read would cost more than
// the detector can pay for. Abandoning the attempt leaves the raw text in
// place, which is what an unrecognized payload gets anyway.
const hermesCardMaxBytes = 1 << 16

// hermesCardText flattens a Feishu (Lark) message card into the text it shows.
//
// Hermes occasionally stores the card payload itself as a message's content,
// instead of the text the conversation carried: a card envelope
// ({"header":…, "body":{"elements":[…]}}, or the v1 shape with a top-level
// "elements") or a bare element such as
//
//	{"tag":"div","text":{"tag":"lark_md","content":"…"}}
//
// Flattening is all-or-nothing: ok is true only when the whole payload is a
// card whose every part is understood, and the caller then keeps the raw JSON
// otherwise. An element this does not know is exactly the case where dropping
// it would lose the only copy of what a reply said, so it stays visible as
// text — unreadable, but never missing.
func hermesCardText(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 2 || len(trimmed) > hermesCardMaxBytes {
		return "", false
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return "", false
	}
	var node any
	if json.Unmarshal([]byte(trimmed), &node) != nil {
		return "", false
	}
	blocks, ok := hermesCardBlocks(node)
	if !ok {
		return "", false
	}
	text := strings.TrimSpace(strings.Join(blocks, "\n\n"))
	if text == "" {
		return "", false
	}
	return text, true
}

// hermesCardBlocks reads a card, an element, or a run of elements.
func hermesCardBlocks(node any) ([]string, bool) {
	switch value := node.(type) {
	case []any:
		// A bare array is a card body without its envelope. Each item has to be
		// an element for this to be a card; an array of anything else is
		// ordinary JSON that happens to start with a bracket.
		if len(value) == 0 {
			return nil, false
		}
		var blocks []string
		for _, item := range value {
			more, ok := hermesCardBlocks(item)
			if !ok {
				return nil, false
			}
			blocks = append(blocks, more...)
		}
		return blocks, true
	case map[string]any:
		if tag, ok := value["tag"].(string); ok {
			return hermesCardElement(tag, value)
		}
		return hermesCardEnvelope(value)
	}
	return nil, false
}

// hermesCardEnvelope reads a card that wraps its elements. Nothing here names
// the envelope: an object is one when it has a card header or a body, or the
// config/schema keys every card carries.
func hermesCardEnvelope(node map[string]any) ([]string, bool) {
	var blocks []string
	recognized := false
	if header, ok := node["header"].(map[string]any); ok {
		recognized = true
		if title, ok := hermesCardTextObject(header["title"]); ok {
			blocks = hermesAppendText(blocks, title)
		}
	}
	elements := node["elements"]
	if body, ok := node["body"].(map[string]any); ok {
		recognized = true
		if elements == nil {
			elements = body["elements"]
		}
	}
	if elements != nil {
		recognized = true
		more, ok := hermesCardBlocks(elements)
		if !ok {
			return nil, false
		}
		blocks = append(blocks, more...)
	}
	for _, key := range []string{"config", "schema"} {
		if _, ok := node[key]; ok {
			recognized = true
		}
	}
	return blocks, recognized
}

// hermesCardElement reads one card element by its tag. An unknown tag is not
// read: see hermesCardText on why the whole payload is then left alone.
func hermesCardElement(tag string, node map[string]any) ([]string, bool) {
	switch tag {
	case "div":
		var blocks []string
		if text, ok := hermesCardTextObject(node["text"]); ok {
			blocks = hermesAppendText(blocks, text)
		} else if node["text"] != nil {
			return nil, false
		}
		// fields are a grid of short values, one per cell; each cell is a text
		// object, and the value ones (a mention, an image) are elements.
		if node["fields"] != nil {
			fields, ok := node["fields"].([]any)
			if !ok {
				return nil, false
			}
			more, ok := hermesCardBlocks(fields)
			if !ok {
				return nil, false
			}
			blocks = append(blocks, more...)
		}
		return blocks, true
	case "markdown", "lark_md", "plain_text", "text":
		text, ok := hermesCardContent(node)
		if !ok {
			return nil, false
		}
		return hermesAppendText(nil, text), true
	case "note":
		return hermesCardChildren(node, "elements")
	case "column_set":
		return hermesCardChildren(node, "columns")
	case "column":
		return hermesCardChildren(node, "elements")
	case "action":
		return hermesCardChildren(node, "actions")
	case "button":
		text, ok := hermesCardTextObject(node["text"])
		if !ok {
			return nil, false
		}
		return hermesAppendText(nil, text), true
	case "at":
		return hermesCardMention(node)
	case "img":
		if alt, ok := hermesCardTextObject(node["alt"]); ok && strings.TrimSpace(alt) != "" {
			return []string{alt}, true
		}
		return []string{"[图片]"}, true
	case "hr":
		// A rule carries no text of its own; an empty block keeps the card
		// recognized without inventing a line of it.
		return nil, true
	case "interactive":
		// A card nested as an element.
		return hermesCardEnvelope(node)
	}
	return nil, false
}

// hermesCardChildren reads a repeating group of child elements.
func hermesCardChildren(node map[string]any, key string) ([]string, bool) {
	children, ok := node[key].([]any)
	if !ok {
		return nil, false
	}
	return hermesCardBlocks(children)
}

func hermesCardMention(node map[string]any) ([]string, bool) {
	name, _ := node["user_name"].(string)
	if strings.TrimSpace(name) == "" {
		name, _ = node["user_id"].(string)
	}
	if strings.TrimSpace(name) == "" {
		name = "all"
	}
	return []string{"@" + name}, true
}

// hermesCardTextObject reads a text object: {"tag":"lark_md","content":"…"} or
// the plain string an older shape uses in its place.
func hermesCardTextObject(value any) (string, bool) {
	switch object := value.(type) {
	case string:
		return object, true
	case map[string]any:
		if content, ok := object["content"].(string); ok {
			return content, true
		}
		if text, ok := object["text"].(string); ok {
			return text, true
		}
	}
	return "", false
}

// hermesCardContent reads the content of a text element, which is always on the
// element itself.
func hermesCardContent(node map[string]any) (string, bool) {
	content, ok := node["content"].(string)
	return content, ok
}

func hermesAppendText(blocks []string, text string) []string {
	if text = strings.TrimSpace(text); text != "" {
		blocks = append(blocks, text)
	}
	return blocks
}
