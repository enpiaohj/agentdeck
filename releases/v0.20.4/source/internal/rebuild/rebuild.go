// Package rebuild 把 AgentDeck 永久库里保留的 Claude Code 会话，重建成 Claude Code 能
// `--resume` 的会话文件，或整理成一份“带上历史”的交接文档。
//
// 背景：Claude Code 只保留约 30 天（cleanupPeriodDays）的会话文件，超期的会被它自己删掉；
// AgentDeck 的永久库留下了解析后的内容（文字、思考、工具调用与结果的文本），但没有原始
// .jsonl 的字节，所以无法“还原”，只能“重建”：
//   - 用户与助手的文字原样保留；
//   - 工具调用与结果转写成文字（工具调用 id 的配对关系无法还原），较长的被截断；
//   - 思考内容不带回（它带签名，重建不出来）；
//   - 总量超过预算时保留开头与最近的部分，中间省略，免得恢复后上下文放不下。
//
// 本包只做纯计算（输入会话内容，输出字节），不碰磁盘；写文件由 app 层负责并受其安全约束。
package rebuild

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const (
	// DefaultBudget 是保留下来的文字总量上限（字符数）。恢复后这些内容要整个进上下文，
	// 中文约 1～2 个 token 一个字，12 万字符大致落在 20 万 token 窗口之内。
	DefaultBudget = 120_000
	maxMessage    = 20_000 // 单条消息的上限，超出保留首尾
	maxToolInput  = 600    // 工具调用的参数
	maxToolResult = 1_500  // 工具返回的结果
	maxHead       = 2_000  // 省略中间部分时，保留的第一条用户消息的长度
	maxDirName    = 200    // Claude Code 对更长的目录名会改用哈希，这里不支持
)

// Stats 描述一次重建的结果，预览与实际写入同形。
type Stats struct {
	Turns   int `json:"turns"`   // 原会话里用户发言的轮数
	Kept    int `json:"kept"`    // 写入的消息数（不含说明）
	Omitted int `json:"omitted"` // 因超出预算而省略的较早消息数
	Tools   int `json:"tools"`   // 转成文字的工具调用数
	Chars   int `json:"chars"`   // 写入的文字字符数
	Bytes   int `json:"bytes"`   // 会话文件的字节数
}

// Message 是整理后的一条发言。
type Message struct {
	Role  string // user 或 assistant
	Text  string
	Model string // 助手消息所属模型，可能为空
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidID 报告 id 是否是 Claude Code 会话 ID（小写 UUID）。写文件名前必须先过它。
func ValidID(id string) bool { return idPattern.MatchString(id) }

// ProjectDirName 是 Claude Code 给目录 cwd 起的项目文件夹名：每个非字母数字字符换成 "-"
// （中文、空格、点、下划线都是；JS 按 UTF-16 码元计，辅助平面字符占两个）。
func ProjectDirName(cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return "", errors.New("没有目录，无法确定项目文件夹")
	}
	var b strings.Builder
	for _, r := range cwd {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r > 0xFFFF:
			b.WriteString("--")
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() > maxDirName {
		return "", fmt.Errorf("目录路径太长（项目文件夹名超过 %d 个字符），Claude Code 会改用哈希命名，暂不支持", maxDirName)
	}
	return b.String(), nil
}

// Messages 把会话内容整理成交替的用户 / 助手发言：思考与上下文提示不带，工具调用与结果转写成
// 助手发言里的文字，超过 budget 时保留开头一条与最近的部分。
func Messages(tr sessions.Transcript, budget int) ([]Message, Stats) {
	if budget <= 0 {
		budget = DefaultBudget
	}
	var st Stats
	var msgs []Message
	add := func(role, model, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if n := len(msgs); n > 0 && msgs[n-1].Role == role {
			msgs[n-1].Text += "\n\n" + text
			if msgs[n-1].Model == "" {
				msgs[n-1].Model = model
			}
			return
		}
		msgs = append(msgs, Message{Role: role, Text: text, Model: model})
	}
	for _, p := range tr.Parts {
		text := strings.TrimSpace(clean(p.Text))
		note := ""
		if p.Cut > 0 {
			note = fmt.Sprintf("\n（已省略后面的 %d 个字符。）", p.Cut)
		}
		switch {
		case p.Kind == "thinking", p.Kind == "context":
			continue
		case p.Kind == "image":
			role := "assistant"
			if p.Role == "user" {
				role = "user"
			}
			add(role, p.Model, "[图片]")
		case p.Kind == "tool_use":
			st.Tools++
			add("assistant", p.Model, "【工具调用："+oneLine(clean(p.Name))+"】"+fenced(limit(text, maxToolInput)))
		case p.Kind == "tool_result":
			label := "【工具结果"
			if name := oneLine(clean(p.Name)); name != "" {
				label += "：" + name
			}
			add("assistant", p.Model, label+"】"+fenced(limit(text, maxToolResult)))
		case p.Role == "user":
			if text != "" {
				st.Turns++
			}
			add("user", "", text+note)
		default:
			add("assistant", p.Model, text+note)
		}
	}
	for i := range msgs {
		msgs[i].Text = limit(msgs[i].Text, maxMessage)
	}
	if len(msgs) == 0 {
		return nil, st
	}
	total := 0
	for _, m := range msgs {
		total += utf8.RuneCountInString(m.Text)
	}
	if total > budget {
		msgs, st.Omitted = trim(msgs, budget)
	}
	for _, m := range msgs {
		st.Chars += utf8.RuneCountInString(m.Text)
	}
	st.Kept = len(msgs)
	return msgs, st
}

// trim 保留第一条用户消息与结尾的若干条（从用户消息开始），中间换成一条助手的说明。
func trim(msgs []Message, budget int) ([]Message, int) {
	var head []Message
	used := 0
	if msgs[0].Role == "user" {
		h := msgs[0]
		h.Text = limit(h.Text, maxHead)
		head = []Message{h}
		used = utf8.RuneCountInString(h.Text)
	}
	start := len(msgs)
	for i := len(msgs) - 1; i >= len(head); i-- {
		n := utf8.RuneCountInString(msgs[i].Text)
		if used+n > budget && start < len(msgs) {
			break
		}
		used += n
		start = i
	}
	for start < len(msgs) && msgs[start].Role != "user" {
		start++ // 保留部分必须从用户发言开始
	}
	if start >= len(msgs) {
		// 预算内装下的都是助手发言：退一步，从最后一条用户发言开始保留
		start = len(msgs)
		for i := len(msgs) - 1; i >= len(head); i-- {
			if msgs[i].Role == "user" {
				start = i
				break
			}
		}
		if start == len(msgs) {
			return msgs, 0
		}
	}
	omitted := start - len(head)
	if omitted <= 0 {
		return msgs, 0
	}
	out := append([]Message{}, head...)
	if len(head) > 0 {
		out = append(out, Message{Role: "assistant", Text: fmt.Sprintf("（此处省略了较早的 %d 条消息，以控制上下文长度。）", omitted)})
	}
	return append(out, msgs[start:]...), omitted
}

// Build 生成 Claude Code 会话文件（.jsonl）的内容：一条说明、可选的标题行，随后是按 parentUuid
// 串起来的用户 / 助手消息。cwd 是要恢复到的目录（决定文件里的 cwd 字段）。同样的输入得到同样的
// 字节（UUID 由会话 ID 与序号派生），所以预览与写入的哈希可以比对。
func Build(s sessions.Session, tr sessions.Transcript, cwd string, now time.Time, budget int) ([]byte, Stats, error) {
	if !ValidID(s.ID) {
		return nil, Stats{}, fmt.Errorf("会话 ID 不是有效的 Claude 会话标识：%q", s.ID)
	}
	if strings.TrimSpace(cwd) == "" {
		return nil, Stats{}, errors.New("没有目录，无法重建会话")
	}
	msgs, st := Messages(tr, budget)
	if len(msgs) == 0 {
		return nil, st, errors.New("永久库里没有可重建的文字内容")
	}
	fallback := "<synthetic>"
	for _, m := range s.Models {
		if m.Model != "" {
			fallback = m.Model
			break
		}
	}
	note := "<system-reminder>\n这是一个由 AgentDeck 根据保留下来的聊天记录重建的会话：原会话文件已被 Claude Code 清理。" +
		"工具调用与结果已转写成文字（以“【工具调用】”“【工具结果】”标出），较长的内容已被截断"
	if st.Omitted > 0 {
		note += fmt.Sprintf("，较早的 %d 条消息已省略", st.Omitted)
	}
	note += "。请把它当作此前对话的文字记录来延续。\n</system-reminder>"

	start := s.Start
	if start.IsZero() {
		start = now.Add(-time.Duration(len(msgs)+1) * time.Second)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	var parent *string
	n := 0
	write := func(role, model, text string, meta bool) error {
		id := derivedUUID(s.ID, n)
		e := entry{ParentUUID: parent, UserType: "external", Cwd: cwd, SessionID: s.ID, Type: role, UUID: id,
			Timestamp: start.Add(time.Duration(n) * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"), IsMeta: meta}
		if role == "user" {
			e.Message = message{Role: "user", Content: text}
		} else {
			if model == "" {
				model = fallback
			}
			e.Message = message{Role: "assistant", Type: "message", Model: model, StopReason: "end_turn",
				Content: []block{{Type: "text", Text: text}}}
		}
		parent = &id
		n++
		return enc.Encode(e)
	}
	if err := write("user", "", note, true); err != nil {
		return nil, st, err
	}
	for _, m := range msgs {
		if err := write(m.Role, m.Model, m.Text, false); err != nil {
			return nil, st, err
		}
	}
	if title := oneLine(clean(s.Title)); title != "" {
		if err := enc.Encode(titleEntry{Type: "custom-title", CustomTitle: title, SessionID: s.ID}); err != nil {
			return nil, st, err
		}
	}
	st.Bytes = buf.Len()
	return buf.Bytes(), st, nil
}

// Handoff 把保留的会话整理成一份交接文档（Markdown），供“新开会话并带上历史”让 Claude 阅读。
func Handoff(s sessions.Session, tr sessions.Transcript, budget int) (string, Stats, error) {
	msgs, st := Messages(tr, budget)
	if len(msgs) == 0 {
		return "", st, errors.New("永久库里没有可整理的文字内容")
	}
	var b strings.Builder
	title := oneLine(clean(s.Title))
	if title == "" {
		title = "（无标题）"
	}
	fmt.Fprintf(&b, "# 此前的对话记录：%s\n\n", title)
	b.WriteString("> 这是 AgentDeck 根据保留下来的聊天记录整理的文字记录，原会话文件已被 Claude Code 清理，无法直接恢复。\n")
	b.WriteString("> 工具调用与结果已转写成文字，较长的内容已被截断")
	if st.Omitted > 0 {
		fmt.Fprintf(&b, "，较早的 %d 条消息已省略", st.Omitted)
	}
	b.WriteString("。请先通读，再按用户接下来的要求继续。\n")
	for _, m := range msgs {
		who := "用户"
		if m.Role == "assistant" {
			who = "助手"
		}
		fmt.Fprintf(&b, "\n---\n\n## %s\n\n%s\n", who, m.Text)
	}
	return b.String(), st, nil
}

type entry struct {
	ParentUUID  *string `json:"parentUuid"`
	IsSidechain bool    `json:"isSidechain"`
	IsMeta      bool    `json:"isMeta,omitempty"`
	UserType    string  `json:"userType"`
	Cwd         string  `json:"cwd"`
	SessionID   string  `json:"sessionId"`
	Type        string  `json:"type"`
	Message     message `json:"message"`
	UUID        string  `json:"uuid"`
	Timestamp   string  `json:"timestamp"`
}

type message struct {
	Role       string `json:"role"`
	Type       string `json:"type,omitempty"`
	Model      string `json:"model,omitempty"`
	Content    any    `json:"content"`
	StopReason string `json:"stop_reason,omitempty"`
}

type block struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type titleEntry struct {
	Type        string `json:"type"`
	CustomTitle string `json:"customTitle"`
	SessionID   string `json:"sessionId"`
}

// derivedUUID 由会话 ID 与序号派生一个 UUID（版本位 5 的样子），保证重建结果可复现。
func derivedUUID(sessionID string, n int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("agentdeck-rebuild/%s/%d", sessionID, n)))
	b := sum[:16]
	b[6] = b[6]&0x0f | 0x50
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// clean 去掉控制字符（保留换行、回车、制表符）：NUL 之类写进会话文件没有意义，只会带来麻烦。
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// limit 把 s 限制在 n 个字符内，超出时保留开头与结尾并注明省略了多少。
func limit(s string, n int) string {
	size := utf8.RuneCountInString(s)
	if size <= n {
		return s
	}
	r := []rune(s)
	head, tail := n*6/10, n-n*6/10
	return string(r[:head]) + fmt.Sprintf("\n…（已省略 %d 个字符）…\n", size-n) + string(r[len(r)-tail:])
}

// fenced 把文字放进长度足以包住其中反引号的代码围栏，前面换行；空文字不生成围栏。
func fenced(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	n, run := 3, 0
	for _, r := range text {
		if r == '`' {
			run++
			n = max(n, run+1)
		} else {
			run = 0
		}
	}
	f := strings.Repeat("`", n)
	return "\n" + f + "\n" + text + "\n" + f
}
