package sessions

// AgentDeck: 会话导出用的 Markdown 排版（独立于 magpie 移植的 markdown.go）。
//
// 与移植版相比，这里面向「导出后长期阅读与归档」：
//   - 顶部 YAML front matter 记录会话身份（session_id、agent、项目、时间、标签、
//     备注），文件名只是方便人看，重新导出时靠 session_id 认出旧文件；
//   - 正文用中文标签，每轮对话用分隔线隔开，用户轮次带序号；
//   - 轮次多时在开头给「对话概览」（每轮用户发言的首行），长会话不必从头翻；
//   - 工具调用折叠成一行摘要（工具名 + 关键参数），展开才看完整参数。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// ExportInfo 是导出时附带的、来自 AgentDeck 自己标注库的信息。
type ExportInfo struct {
	Tags       []string
	Note       string
	Favorite   bool
	ExportedAt time.Time
}

// overviewMin 是出现「对话概览」所需的最少用户轮次；更短的会话一眼就看完。
const overviewMin = 3

// overviewWidth 是概览里每条的最大字符数。
const overviewWidth = 70

// WriteExportMarkdown 把已读出的会话正文 t 写成带 front matter 的 Markdown。
// agent 是智能体名。只写 w，不读任何上游文件。
func WriteExportMarkdown(w io.Writer, s Session, agent string, t Transcript, info ExportInfo) error {
	// 先把正文写进缓冲：概览要等扫完所有轮次才知道，却要排在正文之前。
	var body bytes.Buffer
	bb := bufio.NewWriter(&body)
	var heads []string // 每个用户轮次的首行发言
	who, said, turns, headAt := "", false, 0, -1
	for _, p := range t.Parts {
		p.Text = strings.TrimSpace(exportClean(p.Text))
		p.Name = exportClean(p.Name)
		if p.Text == "" {
			continue
		}
		said = true
		// 工具结果归在调用它的那一轮之下
		if r := p.Role; r != "tool" && r != who {
			who = r
			headAt = -1
			bb.WriteString("\n---\n\n")
			if r == "assistant" {
				bb.WriteString("## 助手\n")
			} else {
				turns++
				heads = append(heads, "")
				headAt = len(heads) - 1
				fmt.Fprintf(bb, "## 用户 · 第 %d 轮\n", turns)
			}
		}
		if headAt >= 0 && p.Role == "user" && p.Kind == "text" && heads[headAt] == "" {
			heads[headAt] = truncateRunes(oneLine(p.Text), overviewWidth)
		}
		bb.WriteString("\n")
		switch p.Kind {
		case "thinking":
			folded(bb, "思考过程", p.Text, false)
		case "tool_use":
			lang := ""
			if strings.HasPrefix(p.Text, "{") || strings.HasPrefix(p.Text, "[") {
				lang = "json"
			}
			summary := "工具调用：" + oneLine(firstNonEmpty(p.Name, "tool"))
			if brief := toolBrief(p.Text); brief != "" {
				summary += " · " + brief
			}
			foldedCode(bb, summary, lang, p.Text)
		case "tool_result":
			label := "工具结果"
			if p.Name != "" {
				label += "：" + oneLine(p.Name)
			}
			folded(bb, label, p.Text, true)
		case "context":
			folded(bb, "上下文", p.Text, true)
		case "image":
			// an imported picture is already Markdown (![图片](…)); anything else is a placeholder
			if strings.HasPrefix(p.Text, "![") {
				bb.WriteString(p.Text + "\n")
			} else {
				bb.WriteString("*[图片]*\n")
			}
		default:
			bb.WriteString(p.Text + "\n")
		}
		if p.Cut > 0 {
			fmt.Fprintf(bb, "\n*（已省略后面的 %d 个字符。）*\n", p.Cut)
		}
	}
	if err := bb.Flush(); err != nil {
		return err
	}

	b := bufio.NewWriter(w)
	title := oneLine(exportClean(s.Title))
	if title == "" {
		title = "（无标题）"
	}
	writeFrontMatter(b, s, agent, title, turns, info)
	fmt.Fprintf(b, "# %s\n\n", title)
	if line := summaryLine(s, agent, turns); line != "" {
		fmt.Fprintf(b, "> %s\n", line)
	}
	if info.Note != "" {
		fmt.Fprintf(b, ">\n> 备注：%s\n", oneLine(info.Note))
	}
	if t.Cut {
		b.WriteString("\n*此会话未完整保留，以下是保留下来的部分。*\n")
	}
	if turns >= overviewMin {
		b.WriteString("\n## 对话概览\n\n")
		for i, h := range heads {
			if h == "" {
				h = "（无文字内容）"
			}
			fmt.Fprintf(b, "%d. %s\n", i+1, h)
		}
	}
	b.Write(body.Bytes())
	if !said {
		b.WriteString("\n*此会话中没有任何内容。*\n")
	}
	return b.Flush()
}

func writeFrontMatter(b *bufio.Writer, s Session, agent, title string, turns int, info ExportInfo) {
	b.WriteString("---\n")
	kv := func(k, v string) { fmt.Fprintf(b, "%s: %s\n", k, v) }
	kv("title", yamlString(title))
	b.WriteString(ExportIDLine(s.ID) + "\n")
	kv("agent", yamlString(agent))
	if s.Cwd != "" {
		kv("project", yamlString(s.Cwd))
	}
	if !s.Start.IsZero() {
		kv("started", s.Start.Local().Format(time.RFC3339))
	}
	if !s.Last.IsZero() {
		kv("last_active", s.Last.Local().Format(time.RFC3339))
	}
	var models []string
	for _, m := range s.Models {
		if m.Model != "" {
			models = append(models, m.Model)
		}
	}
	if len(models) > 0 {
		kv("models", yamlList(models))
	}
	if len(info.Tags) > 0 {
		kv("tags", yamlList(info.Tags))
	}
	if info.Favorite {
		kv("favorite", "true")
	}
	if info.Note != "" {
		kv("note", yamlString(oneLine(info.Note)))
	}
	kv("turns", fmt.Sprint(turns))
	kv("source", yamlString("AgentDeck"))
	if !info.ExportedAt.IsZero() {
		kv("exported_at", info.ExportedAt.Local().Format(time.RFC3339))
	}
	b.WriteString("---\n\n")
}

// ExportIDLine 是 front matter 里标识会话身份的那一行（不含换行）。重新导出时
// 靠它认出同一会话的旧文件。
func ExportIDLine(id string) string { return "session_id: " + yamlString(id) }

// summaryLine 是标题下面的一行：谁、什么时候、在哪个目录、多少轮。
func summaryLine(s Session, agent string, turns int) string {
	var parts []string
	if agent != "" {
		parts = append(parts, agent)
	}
	switch {
	case !s.Start.IsZero() && !s.Last.IsZero():
		parts = append(parts, s.Start.Local().Format("2006-01-02 15:04")+" → "+s.Last.Local().Format("2006-01-02 15:04"))
	case !s.Start.IsZero():
		parts = append(parts, s.Start.Local().Format("2006-01-02 15:04"))
	case !s.Last.IsZero():
		parts = append(parts, s.Last.Local().Format("2006-01-02 15:04"))
	}
	if s.Cwd != "" {
		parts = append(parts, "目录 `"+s.Cwd+"`")
	}
	if turns > 0 {
		parts = append(parts, fmt.Sprintf("%d 轮", turns))
	}
	return strings.Join(parts, " · ")
}

// foldedCode 是折叠块，内容放在围栏里（工具参数）。
func foldedCode(b *bufio.Writer, summary, lang, text string) {
	fmt.Fprintf(b, "<details>\n<summary>%s</summary>\n\n", htmlEscape(summary))
	fenced(b, lang, text)
	b.WriteString("\n</details>\n")
}

// toolBrief 从工具参数里挑一个最能说明「它在做什么」的值，做折叠行的摘要。
// 参数不是合法 JSON（比如被截断）或没有常见字段时返回空串。
func toolBrief(text string) string {
	var m map[string]any
	if json.Unmarshal([]byte(text), &m) != nil {
		return ""
	}
	for _, k := range []string{"command", "cmd", "file_path", "path", "pattern", "query", "url", "description"} {
		switch v := m[k].(type) {
		case string:
			if s := oneLine(v); s != "" {
				return truncateRunes(s, 80)
			}
		case []any:
			var words []string
			for _, e := range v {
				if s, ok := e.(string); ok {
					words = append(words, s)
				}
			}
			if s := oneLine(strings.Join(words, " ")); s != "" {
				return truncateRunes(s, 80)
			}
		}
	}
	return ""
}

// bulletGlyphs 是 PDF 文字提取常把项目符号带出来的私有区字符（Wingdings / Symbol 字体里
// 的圆点、方块、箭头、对勾），私有区在任何字体里都画不出来，映射成同义的标准符号。
var bulletGlyphs = map[rune]rune{
	0xF06C: '●', 0xF06E: '■', 0xF075: '◆', 0xF076: '❖', 0xF0A7: '▪',
	0xF0B7: '•', 0xF0D8: '➢', 0xF0FC: '✓', 0xF0E0: '→',
}

// exportClean 去掉写进 .md 会让阅读器出错的字符：NUL 等控制字符（很多编辑器见到 NUL 就把
// 整个文件当二进制或猜错编码，后半段全成乱码，PDF / 二进制工具的输出里常有），保留换行、
// 回车与制表符；并把常见的私有区项目符号换成标准符号。其余内容不动。
func exportClean(s string) string {
	clean := true
	for _, r := range s {
		if (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0x7f || (r >= 0x80 && r < 0xa0) || (r >= 0xE000 && r <= 0xF8FF) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 && r != '\n' && r != '\r' && r != '\t', r == 0x7f, r >= 0x80 && r < 0xa0:
			return -1
		case r >= 0xE000 && r <= 0xF8FF:
			if g, ok := bulletGlyphs[r]; ok {
				return g
			}
		}
		return r
	}, s)
}

// yamlString 用 JSON 字符串字面量表示：JSON 是 YAML 的子集，转义规则现成，
// 标题里的冒号、引号、换行都不会破坏 front matter。
func yamlString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimRight(buf.String(), "\n")
}

func yamlList(items []string) string {
	q := make([]string, len(items))
	for i, it := range items {
		q[i] = yamlString(it)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
