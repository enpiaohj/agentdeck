package sessions

import (
	"bytes"
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/enpiaohj/agentdeck/internal/appdir"
)

// WorkBuddy (Tencent's desktop agent) keeps its sessions much as Claude
// Code does, under its folder's projects/: a session's <id>.jsonl in its
// project's folder, with <id>.meta.json beside it (the cwd, for the ones it
// brought over from its older history). Its lines are the OpenAI Agents
// SDK's items — "message", "reasoning", "function_call",
// "function_call_result" — and "ai-title" and "custom-title", each with its
// time (timestamp, in milliseconds), the session id and mostly the cwd. What
// a reply spent is in providerData.usage of one of its lines (the assistant
// message's, or its last function call's), named by the reply's
// providerData.messageId: input with what was read from the cache in it
// (inputTokensDetails[].cached_tokens), output with the reasoning. The
// sessions brought over from its older history have it as input_tokens and
// output_tokens instead, at times one line for the whole conversation.
// WorkBuddy is a desktop app: nothing resumes a session from a terminal.

// WorkBuddyDir is WorkBuddy's folder: $WORKBUDDY_CONFIG_DIR, else ~/.workbuddy.
func WorkBuddyDir() string {
	if d := strings.TrimSpace(appdir.Getenv("WORKBUDDY_CONFIG_DIR")); d != "" {
		return expandHome(d)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".workbuddy")
}

// workbuddyFiles are the sessions under WorkBuddy's projects/, however deep:
// a session's own <id>.jsonl, and a subagent's under <id>/subagents/.
func workbuddyFiles() []file { return buddyFiles("workbuddy", WorkBuddyDir()) }

// AgentDeck: WorkBuddy AI, WorkBuddy's international edition, is the same app
// under its own account and folder, ~/.workbuddy-ai, kept the same way; it is
// listed as its own agent, as qoder and qoder-cn are.

// WorkBuddyAIDir is WorkBuddy AI's folder: $WORKBUDDY_AI_CONFIG_DIR, else
// ~/.workbuddy-ai.
func WorkBuddyAIDir() string {
	if d := strings.TrimSpace(appdir.Getenv("WORKBUDDY_AI_CONFIG_DIR")); d != "" {
		return expandHome(d)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".workbuddy-ai")
}

func workbuddyAIFiles() []file { return buddyFiles("workbuddy-ai", WorkBuddyAIDir()) }

// isWorkBuddy is whether an agent's files are WorkBuddy's, either edition.
func isWorkBuddy(agent string) bool { return agent == "workbuddy" || agent == "workbuddy-ai" }

// buddyFiles are the sessions under projects/ in dir, as agent's: WorkBuddy's,
// or CodeBuddy Code's, which keeps them the same way.
func buddyFiles(agent, dir string) []file {
	var out []file
	walkLinked(filepath.Join(dir, "projects"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		f := file{agent: agent, key: agent + ":" + strings.TrimSuffix(d.Name(), ".jsonl"), path: p, main: true}
		if dir := filepath.Dir(p); filepath.Base(dir) == "subagents" {
			f.key, f.main = agent+":"+filepath.Base(filepath.Dir(dir)), false
		}
		if stat(&f) {
			out = append(out, f)
		}
		return nil
	})
	return out
}

// wbDetails are the details of a count WorkBuddy wrote down: a list of them
// (inputTokensDetails: [{cached_tokens}]), or one.
type wbDetails []map[string]int

func (d *wbDetails) UnmarshalJSON(b []byte) error {
	var list []map[string]int
	if json.Unmarshal(b, &list) == nil {
		*d = list
		return nil
	}
	var one map[string]int
	if json.Unmarshal(b, &one) == nil {
		*d = wbDetails{one}
	}
	return nil
}

func (d wbDetails) sum(key string) int {
	n := 0
	for _, m := range d {
		n += m[key]
	}
	return n
}

type wbLine struct {
	Type         string          `json:"type"`
	Role         string          `json:"role"`
	ID           string          `json:"id"`
	Cwd          string          `json:"cwd"`
	SessionID    string          `json:"sessionId"`
	AITitle      string          `json:"aiTitle"`
	CustomTitle  string          `json:"customTitle"`
	Content      json.RawMessage `json:"content"`
	ProviderData struct {
		MessageID    string `json:"messageId"`
		Model        string `json:"model"`
		RequestModel string `json:"requestModelId"`
		Usage        *struct {
			Input         int       `json:"inputTokens"`
			Output        int       `json:"outputTokens"`
			InputDetails  wbDetails `json:"inputTokensDetails"`
			OldInput      int       `json:"input_tokens"`
			OldOutput     int       `json:"output_tokens"`
			OldDetails    wbDetails `json:"input_tokens_details"`
			CacheWrite    int       `json:"cacheCreationTokens"`
			OldCacheWrite int       `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"providerData"`
}

var (
	wbUsage   = []byte(`"usage"`)
	wbTitle   = []byte(`-title"`)
	wbMessage = []byte(`"type":"message"`)
	wbCwd     = []byte(`"cwd":"`)
	wbScene   = regexp.MustCompile(`^@[\w-]+:\S*$`)
)

func workbuddyLine(s *state, b []byte, main bool) {
	at := numTS(b)
	s.saw(at, main)
	// AgentDeck: every message line of the main file is read, to count the
	// prompts and replies a day (the message count); before, only the first
	// prompt was, and WorkBuddy's sessions all said 0 messages.
	want := bytes.Contains(b, wbUsage) ||
		main && (bytes.Contains(b, wbTitle) || bytes.Contains(b, wbMessage)) ||
		s.Cwd == "" && bytes.Contains(b, wbCwd)
	if !want {
		return
	}
	var l wbLine
	if json.Unmarshal(b, &l) != nil {
		return
	}
	if s.Cwd == "" && l.Cwd != "" {
		s.Cwd = l.Cwd
	}
	if s.ID == "" && l.SessionID != "" {
		s.ID = l.SessionID
	}
	p := l.ProviderData
	if m := cmp.Or(p.Model, p.RequestModel); m != "" {
		s.Model = m // the model in use, for a usage that names none
	}
	switch l.Type {
	case "custom-title":
		// named by the user: before the one WorkBuddy made
		if main && l.CustomTitle != "" {
			s.Custom = title(l.CustomTitle)
		}
	case "ai-title":
		if main && l.AITitle != "" {
			s.Named = title(l.AITitle)
		}
	case "message":
		if main && l.Role == "user" {
			// a prompt is words the user typed, not what WorkBuddy put in alone
			if prompt := wbPrompt(l.Content); prompt != "" {
				if s.First == "" {
					s.First = prompt
				}
				s.day(dateOf(at)).Prompts++
			}
		}
		if main && l.Role == "assistant" && wbHasText(l.Content) {
			s.day(dateOf(at)).Replies++
		}
	}
	u := p.Usage
	if u == nil {
		return
	}
	in, out := cmp.Or(u.Input, u.OldInput), cmp.Or(u.Output, u.OldOutput)
	read := u.InputDetails.sum("cached_tokens") + u.OldDetails.sum("cached_tokens")
	write := cmp.Or(u.CacheWrite, u.OldCacheWrite)
	t := Tokens{Input: max(0, in-read-write), Output: out, CacheRead: read, CacheWrite: write}
	model := cmp.Or(p.Model, p.RequestModel, s.Model, "auto")
	id := p.MessageID
	if id != "" && id == s.Msg {
		// another line of the reply counted: its usage stands for the whole
		s.unuse(s.MsgDay, s.MsgModel, s.MsgUse)
	}
	date := dateOf(at)
	s.Msg, s.MsgModel, s.MsgUse, s.MsgDay = id, model, t, date
	s.use(date, model, t)
}

// wbHasText is whether an assistant message says something: an output_text
// block with words in it, not only a step between tool calls.
func wbHasText(content json.RawMessage) bool {
	var blocks []struct{ Type, Text string }
	if json.Unmarshal(content, &blocks) != nil {
		var text string
		return json.Unmarshal(content, &text) == nil && strings.TrimSpace(text) != ""
	}
	for _, b := range blocks {
		if b.Type == "output_text" && strings.TrimSpace(b.Text) != "" {
			return true
		}
	}
	return false
}

// wbPrompt is the words typed in a user message: its input_text blocks,
// less what WorkBuddy put in beside them (<system-reminder>…, @scene:…).
func wbPrompt(content json.RawMessage) string {
	var text string
	if json.Unmarshal(content, &text) == nil {
		if q := wbQuery(text); q != "" {
			return title(q)
		}
		if strings.HasPrefix(strings.TrimSpace(text), "<") {
			return ""
		}
		return title(text)
	}
	var blocks []struct{ Type, Text string }
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		t := strings.TrimSpace(b.Text)
		if b.Type != "input_text" && b.Type != "text" {
			continue
		}
		if q := wbQuery(t); q != "" {
			parts = append(parts, q)
		} else if t != "" && !strings.HasPrefix(t, "<") && !wbScene.MatchString(t) {
			parts = append(parts, t)
		}
	}
	return title(strings.Join(parts, " "))
}

// AgentDeck: newer WorkBuddy builds put the typed words in the same block as
// its <system-reminder>, wrapped in <user_query>; the block starts with "<"
// and was dropped whole, so the prompt went uncounted and untitled.
var wbUserQuery = regexp.MustCompile(`(?s)<user_query>(.*?)</user_query>`)

// wbQuery is the words in a block's <user_query> tags, "" without them.
func wbQuery(text string) string {
	var parts []string
	for _, m := range wbUserQuery.FindAllStringSubmatch(text, -1) {
		if q := strings.TrimSpace(m[1]); q != "" {
			parts = append(parts, q)
		}
	}
	return strings.Join(parts, " ")
}

// workbuddyMeta fills in a session's cwd from its meta file, for the lines
// that name none.
func workbuddyMeta(s *state, path string) {
	b, err := os.ReadFile(strings.TrimSuffix(path, ".jsonl") + ".meta.json")
	if err != nil {
		return
	}
	var m struct {
		Cwd string `json:"cwd"`
	}
	if json.Unmarshal(b, &m) == nil {
		s.Cwd = m.Cwd
	}
}
