package app

// 会话导出的文件名与落盘。格式：YYYY-MM-DD-会话标题-短ID.md
//   - 日期取会话开始日期（没有则取最后活跃日期），不是导出时刻，
//     所以同一会话重复导出得到同一个文件名，按名字排序即按时间排序；
//   - 标题是用户在 AgentDeck 里看到的名字，清理掉 Windows 非法字符；
//   - 短 ID 是会话 ID 前 8 位，标题会重复或为空，靠它保证唯一并可反查。

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const (
	exportTitleMax = 50 // 标题最多保留的字符数（按字符，不按字节）
	exportIDLen    = 8
	exportNoTitle  = "无标题"
	exportNoDate   = "未知日期"
	// exportHeadBytes 是识别「这是不是该会话的旧导出」时读取的文件头长度，
	// front matter 总在最前面。
	exportHeadBytes = 4096
)

// exportFileName 返回会话 sess 导出文件的名字（不含目录）。title 是显示用标题。
func exportFileName(sess sessions.Session, title string) string {
	day := exportNoDate
	switch {
	case !sess.Start.IsZero():
		day = sess.Start.Local().Format("2006-01-02")
	case !sess.Last.IsZero():
		day = sess.Last.Local().Format("2006-01-02")
	}
	return day + "-" + exportTitle(title) + "-" + exportShortID(sess.ID) + ".md"
}

// exportTitle 把标题清理成可做文件名的一段：非法字符与空白变成下划线，
// 首尾的点、空格、下划线、连字符去掉，截到 exportTitleMax 个字符。
func exportTitle(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case strings.ContainsRune(`\/:*?"<>|`, r), unicode.IsControl(r), unicode.IsSpace(r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	// 连续的下划线合成一个
	s := b.String()
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "._-")
	if utf8.RuneCountInString(s) > exportTitleMax {
		s = strings.Trim(string([]rune(s)[:exportTitleMax]), "._-")
	}
	if s == "" {
		return exportNoTitle
	}
	return s
}

func exportShortID(id string) string {
	s := safeFile(id)
	if len(s) > exportIDLen {
		s = s[:exportIDLen]
	}
	return s
}

// writeExportFile 把会话写到 dir 下并返回最终路径。
//
//   - 先找该会话以前的导出（文件名以同一个短 ID 结尾，且文件头 front matter
//     里的 session_id 与本会话一致）：会话改名后文件名会变，旧文件改名而不是
//     另存一份，避免同一个会话堆出多个文件。只改名认得出是自己写的文件。
//   - 目标名已被别的文件占用（不是该会话的导出）时加 _2、_3……，绝不覆盖。
//   - 先写临时文件再改名，写到一半失败不会留下半截的导出或毁掉旧导出。
func writeExportFile(dir, name, sessionID string, write func(io.Writer) error) (string, error) {
	target := filepath.Join(dir, name)
	if old := findPreviousExport(dir, sessionID); old != "" && !strings.EqualFold(old, target) {
		// 旧导出改到新名字；目标已存在则改走下面的占位判断，不在这里覆盖
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			if err := os.Rename(old, target); err != nil {
				target = old // 改不了名就原地更新，别另存一份
			}
		} else {
			target = old
		}
	}
	if !ownsExport(target, sessionID) {
		target = freeName(dir, name)
	}
	tmp, err := os.CreateTemp(dir, ".agentdeck-export-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if err := write(tmp); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return target, nil
}

// ownsExport 报告 path 可以被本会话的导出写入：文件不存在，或者它就是
// 本会话上一次的导出。
func ownsExport(path, sessionID string) bool {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return true
	}
	return isExportOf(path, sessionID)
}

// findPreviousExport 在 dir 里找会话 sessionID 以前导出的文件：只看以同一个
// 短 ID 结尾的 .md，再核对 front matter，不凭文件名猜。
func findPreviousExport(dir, sessionID string) string {
	suffix := "-" + exportShortID(sessionID) + ".md"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if isExportOf(p, sessionID) {
			return p
		}
	}
	return ""
}

// isExportOf 报告 path 是不是本程序导出的、属于 sessionID 的文件：文件头的
// front matter 里有一行 session_id。
func isExportOf(path, sessionID string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, exportHeadBytes)
	n, _ := io.ReadFull(f, head)
	text := string(head[:n])
	if !strings.HasPrefix(text, "---\n") {
		return false
	}
	return strings.Contains(text, "\n"+sessions.ExportIDLine(sessionID)+"\n")
}

// freeName 在 dir 里找一个没被占用的名字：name、name_2、name_3……
func freeName(dir, name string) string {
	stem := strings.TrimSuffix(name, ".md")
	for i := 2; ; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%s_%d.md", stem, i))
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
	}
}
