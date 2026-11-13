package sessions

import (
	"encoding/json"
	"sort"
	"strings"
)

// A card the agent only ever sent through a tool call. Hermes does not always
// keep such a card as the message's content (the case hermesCardText covers):
// the row holds the agent's own note, and the card itself exists only inside
// the call that delivered it — an execute_code building the payload as a
// Python literal, or a terminal heredoc handing JSON to card_send.py. Read
// from the row alone, that body is visible only as code inside the call.

// hermesCardScanMaxBytes bounds the arguments a call is searched in. A call
// that carries a card also carries the script around it; past this size the
// arguments are a file the agent wrote, and walking it on every read would
// cost more than the card is worth.
const hermesCardScanMaxBytes = 1 << 20

// hermesCardCandidates bounds how many object literals one argument string is
// tried against. A card is found by its first key, so the literals before it
// are the few dicts and shell fragments a script starts with; a string with
// more than this is not a card's script.
const hermesCardCandidates = 64

// hermesCardWindowBytes bounds how much of an argument string one candidate is
// read out of: a card is no longer than hermesCardMaxBytes, and the window
// leaves room for the rest of a script's line around it.
const hermesCardWindowBytes = 4 * hermesCardMaxBytes

// hermesCardKeys are the keys a card opens with — the whole vocabulary of an
// envelope's head, plus the tag an element starts with.
var hermesCardKeys = []string{"config", "schema", "header", "body", "elements", "tag", "wide_screen_mode"}

// hermesCallCardText is the card a call's arguments carry, flattened as
// hermesCardText flattens one stored as content, or "" when there is none.
//
// Detection is by shape alone, so the same all-or-nothing rule applies: a
// literal the card reader does not fully understand leaves nothing behind and
// the arguments stay as they were. Hermes also truncates the arguments it
// keeps (the stored text ends in "...[truncated]"), which cuts the literal
// short; that payload is not read back, and the fallback keeps the code.
func hermesCallCardText(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var node any
	if json.Unmarshal(args, &node) != nil {
		return ""
	}
	for _, text := range hermesArgTexts(node) {
		if card := hermesCardLiteral(text); card != "" {
			return card
		}
	}
	return ""
}

// hermesArgTexts are the strings the arguments hold. A call's arguments are an
// object, or a string of JSON holding one (kimiArgs reads both shapes), so a
// string is read as JSON once more before it is read as text.
func hermesArgTexts(node any) []string {
	var texts []string
	if inner, ok := node.(string); ok {
		if len(inner) <= hermesCardScanMaxBytes {
			var nested any
			if json.Unmarshal([]byte(inner), &nested) == nil {
				texts = hermesArgStrings(nested, 0)
			}
		}
	}
	return append(texts, hermesArgStrings(node, 0)...)
}

// hermesArgStrings are the strings a call's arguments hold, in a stable order:
// the arguments are an object of named values, and reading the same row twice
// must find the same card.
func hermesArgStrings(node any, depth int) []string {
	if depth > 8 {
		return nil
	}
	switch value := node.(type) {
	case string:
		if len(value) > hermesCardScanMaxBytes {
			return nil
		}
		return []string{value}
	case []any:
		var out []string
		for _, item := range value {
			out = append(out, hermesArgStrings(item, depth+1)...)
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var out []string
		for _, key := range keys {
			out = append(out, hermesArgStrings(value[key], depth+1)...)
		}
		return out
	}
	return nil
}

// hermesCardLiteral is the flattened card of the first literal in source that
// reads as one, or "".
//
// A candidate is an object opening with a card's key (hermesCardKeys); the
// decoder, not a brace counter, says where it ends, so quotes and escapes are
// the only reading of the text that matters — the same literal is written
// both as JSON and as Python (True for true), and may sit inside a shell
// command whose quoting has nothing to do with its own.
func hermesCardLiteral(source string) string {
	tried := 0
	for i := 0; i < len(source) && tried < hermesCardCandidates; i++ {
		if source[i] != '{' || !hermesCardHead(source[i+1:]) {
			continue
		}
		tried++
		node, ok := hermesCardObject(source[i:])
		if !ok {
			continue
		}
		blocks, ok := hermesCardBlocks(node)
		if !ok {
			continue
		}
		if text := strings.TrimSpace(strings.Join(blocks, "\n\n")); text != "" {
			return text
		}
	}
	return ""
}

// hermesCardObject decodes the object a candidate opens, if it is no larger
// than a card may be. It is read as JSON first; the same literal written as
// Python is read after the rewrite hermesPythonJSON makes.
func hermesCardObject(rest string) (any, bool) {
	window := rest
	if len(window) > hermesCardWindowBytes {
		window = window[:hermesCardWindowBytes]
	}
	if node, parsed, ok := hermesDecodeObject(window); parsed {
		return node, ok
	}
	node, _, ok := hermesDecodeObject(hermesPythonJSON(window))
	return node, ok
}

// hermesDecodeObject decodes the first value text holds: parsed says it held
// one, ok whether that value is small enough to be a card.
func hermesDecodeObject(text string) (node any, parsed, ok bool) {
	dec := json.NewDecoder(strings.NewReader(text))
	if dec.Decode(&node) != nil {
		return nil, false, false
	}
	// A literal this long is a document, not a card.
	return node, true, dec.InputOffset() <= hermesCardMaxBytes
}

// hermesCardHead says whether what follows an opening brace is one of a card's
// keys, which is what makes the literal worth decoding.
func hermesCardHead(rest string) bool {
	rest = strings.TrimLeft(rest, " \t\r\n")
	if !strings.HasPrefix(rest, `"`) {
		return false
	}
	rest = rest[1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return false
	}
	key := rest[:end]
	for _, known := range hermesCardKeys {
		if key == known {
			return true
		}
	}
	return false
}

// hermesPythonJSON makes a Python literal readable as JSON: True, False and
// None spelled as JSON's true, false and null, and the comma Python allows
// before a closing brace or bracket dropped. Both rewrites happen outside
// double-quoted strings, so a payload already written as JSON passes through
// unchanged. A literal written with Python's single quotes is left as it is:
// its scalars are inside strings as far as this reading goes, and rewriting
// quote styles would risk the very text being read.
func hermesPythonJSON(literal string) string {
	out := make([]byte, 0, len(literal))
	quoted, escaped := false, false
	for i := 0; i < len(literal); i++ {
		c := literal[i]
		if quoted {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			out = append(out, c)
			continue
		}
		// A scalar is rewritten whole: a name such as TrueType is not a value.
		if i == 0 || !hermesIdentByte(literal[i-1]) {
			if rewritten, n := hermesPythonScalar(literal[i:]); n > 0 {
				out = append(out, rewritten...)
				i += n - 1
				continue
			}
		}
		// Python tolerates the comma before a closing brace or bracket that JSON
		// rejects; drop it, whitespace and all, so the same literal reads here.
		if c == ',' {
			j := i + 1
			for j < len(literal) && strings.IndexByte(" \t\r\n", literal[j]) >= 0 {
				j++
			}
			if j < len(literal) && (literal[j] == '}' || literal[j] == ']') {
				i = j - 1
				continue
			}
		}
		out = append(out, c)
	}
	return string(out)
}

// hermesPythonScalar is the JSON spelling of the Python scalar at the head of
// s, and its length.
func hermesPythonScalar(s string) (string, int) {
	for _, scalar := range []struct{ python, json string }{
		{"True", "true"},
		{"False", "false"},
		{"None", "null"},
	} {
		rest, ok := strings.CutPrefix(s, scalar.python)
		if !ok {
			continue
		}
		if rest != "" && hermesIdentByte(rest[0]) {
			return "", 0
		}
		return scalar.json, len(scalar.python)
	}
	return "", 0
}

func hermesIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
