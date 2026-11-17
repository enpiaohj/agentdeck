package chatgpthistory

// A suggested name for a Project, worked out from nothing but what the export
// itself holds (the titles of the project's conversations and the names of the
// files uploaded to it). It is a guess meant to be checked: the import page
// offers it as a button and never saves it on its own.

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Where a recommended name came from; the import page says so next to it.
const (
	BasisTitles = "titles" // a word that keeps coming back in the conversation titles
	BasisFile   = "file"   // the name of a file uploaded to the project
	BasisLatest = "latest" // the title of the latest conversation
)

const (
	recommendMaxRunes = 20 // a recommended name stays short
	termMin, termMax  = 2, 4
)

// latinWord matches a word of a title written in letters: three or more.
var latinWord = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+#.]{2,}`)

// Words that say what kind of request a title is, not what the project is about.
var stopLatin = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "how": true, "what": true, "why": true, "can": true, "you": true,
	"are": true, "this": true, "that": true, "from": true, "into": true, "about": true, "your": true, "using": true,
	"use": true, "help": true, "need": true, "new": true, "chat": true, "gpt": true, "ask": true, "get": true, "make": true,
}

var stopCJK = map[string]bool{
	"如何": true, "怎么": true, "怎样": true, "什么": true, "为什么": true, "帮我": true, "一下": true, "请问": true,
	"关于": true, "以及": true, "一个": true, "这个": true, "那个": true, "可以": true, "能否": true, "是否": true,
	"有没有": true, "问题": true, "方法": true, "介绍": true, "解释": true, "说明": true, "需要": true, "怎么办": true,
	"新对话": true, "未命名": true, "对话": true, "聊天": true, "回答": true, "一些": true, "还是": true, "没有": true,
	"如果": true, "我的": true, "你的": true, "他们": true, "我们": true, "自己": true, "进行": true, "使用": true,
	"创建": true, "修改": true, "添加": true, "删除": true, "更新": true, "查询": true, "检查": true, "查看": true,
	"设置": true, "生成": true, "编写": true, "实现": true, "处理": true, "解决": true, "优化": true, "分析": true,
	"总结": true, "翻译": true, "整理": true, "制作": true, "帮忙": true, "继续": true, "测试": true, "示例": true,
	"建议": true, "项目": true, "内容": true, "相关": true, "功能": true, "信息": true, "工具": true, "资料": true,
}

// stopWords is stopCJK longest first, so "为什么" is cut before "什么".
var stopWords = func() []string {
	var out []string
	for w := range stopCJK {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}()

// Characters a term must not start or end with: it would be a fragment of a longer word.
const edgeCJK = "的了是在和与及对把被有为个吗呢吧啊也都就还又很不"

func isHan(r rune) bool { return unicode.Is(unicode.Han, r) }

// termsOf lists the distinct candidate terms of one title. Latin words are
// keyed in lower case; shown, so the original spelling (LDAP, ESXi) is kept in
// the forms map: key → spellings seen.
func termsOf(title string, forms map[string]map[string]int) map[string]bool {
	out := map[string]bool{}
	for _, w := range latinWord.FindAllString(title, -1) {
		w = strings.TrimRight(w, ".")
		key := strings.ToLower(w)
		if len(key) >= 3 && !stopLatin[key] {
			out[key] = true
			if forms != nil {
				if forms[key] == nil {
					forms[key] = map[string]int{}
				}
				forms[key][w]++
			}
		}
	}
	// Generic words are cut out first (each becomes a break), so a term can neither be
	// one of them nor straddle two of them ("项目建议一" must not yield "目建").
	for _, w := range stopWords {
		title = strings.ReplaceAll(title, w, " ")
	}
	var run []rune
	flush := func() {
		for n := termMin; n <= termMax; n++ {
			for i := 0; i+n <= len(run); i++ {
				t := run[i : i+n]
				if strings.ContainsRune(edgeCJK, t[0]) || strings.ContainsRune(edgeCJK, t[len(t)-1]) || genericOnly(string(t)) {
					continue
				}
				out[string(t)] = true
			}
		}
		run = run[:0]
	}
	for _, r := range title {
		if isHan(r) {
			run = append(run, r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// recommendName suggests a name for a project from its conversation titles
// (any order), the title of its latest conversation, and the names of its
// uploaded files (newest first). It returns "" when there is nothing to go on.
func recommendName(titles []string, latest string, files []string) (name, basis string) {
	if n := len(titles); n >= 2 {
		df := map[string]int{}
		forms := map[string]map[string]int{}
		for _, t := range titles {
			for term := range termsOf(t, forms) {
				df[term]++
			}
		}
		// in a quarter of the titles, never in fewer than two — and for a big project,
		// whose titles are all over the place, six are enough
		need := max(2, min((n+3)/4, 6))
		type scored struct {
			term  string
			df    int
			score float64
		}
		var cands []scored
		for term, c := range df {
			if c < need {
				continue
			}
			// keep only the longest of terms that always appear together
			longer := false
			for other, oc := range df {
				if len(other) > len(term) && oc == c && strings.Contains(other, term) {
					longer = true
					break
				}
			}
			if longer {
				continue
			}
			l := len([]rune(term))
			cands = append(cands, scored{term, c, float64(c) * (1 + 0.15*float64(l-termMin))})
		}
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].score != cands[j].score {
				return cands[i].score > cands[j].score
			}
			return cands[i].term < cands[j].term
		})
		if len(cands) > 0 {
			name = spelled(cands[0].term, forms)
			// a second word that is nearly as common and not a part of the first
			for _, c := range cands[1:] {
				if c.score >= cands[0].score*0.8 && !strings.Contains(cands[0].term, c.term) && !strings.Contains(c.term, cands[0].term) {
					name += "、" + spelled(c.term, forms)
					break
				}
			}
			return name, BasisTitles
		}
	}
	for _, f := range files {
		stem := cleanName(strings.TrimSuffix(f, pathExt(f)))
		if meaningfulStem(stem) {
			return truncateRunes(stem, recommendMaxRunes), BasisFile
		}
	}
	// One conversation's title stands for the project only when there are one or two
	// of them; with more, a single title is a poor name, and no guess is better.
	if s := cleanName(latest); s != "" && len(titles) <= 2 {
		return strings.TrimSuffix(truncateRunes(s, recommendMaxRunes), "…"), BasisLatest
	}
	return "", ""
}

// genericOnly says whether a term is nothing but generic words: taking them out
// leaves fewer than two characters ("项目建议" is "项目" + "建议").
func genericOnly(term string) bool {
	rest := term
	for w := range stopCJK {
		rest = strings.ReplaceAll(rest, w, "")
	}
	return len([]rune(rest)) < 2
}

// spelled is the most common spelling seen of a Latin term (its lower-case key
// when it is not Latin).
func spelled(term string, forms map[string]map[string]int) string {
	best, bestN := term, 0
	var spellings []string
	for s := range forms[term] {
		spellings = append(spellings, s)
	}
	sort.Strings(spellings)
	for _, s := range spellings {
		if n := forms[term][s]; n > bestN {
			best, bestN = s, n
		}
	}
	return best
}

// genericStem matches file names that say nothing about a project: pictures,
// screenshots, "untitled", camera and download names.
var genericStem = regexp.MustCompile(`(?i)^(image|img|screenshot|screen shot|photo|pic|picture|file|document|doc|untitled|download|scan|录屏|截图|屏幕截图|图片|照片|未命名|新建)`)

// meaningfulStem says whether a file's name (without extension) can stand as a
// project name: not a generic or timestamp-like name, and mostly letters.
func meaningfulStem(stem string) bool {
	r := []rune(stem)
	if len(r) < 2 || genericStem.MatchString(stem) {
		return false
	}
	letters := 0
	for _, c := range r {
		if unicode.IsLetter(c) {
			letters++
		}
	}
	return letters*10 >= len(r)*6 // at least 60% letters; "2026-11-06_112129" is not a name
}

// pathExt is the extension of a file name (".pdf"), "" when it has none.
func pathExt(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 && len(name)-i <= 6 && !strings.ContainsAny(name[i:], " /\\") {
		return name[i:]
	}
	return ""
}
