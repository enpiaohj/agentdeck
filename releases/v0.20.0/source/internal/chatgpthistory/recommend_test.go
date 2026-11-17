package chatgpthistory

import (
	"strings"
	"testing"
)

func TestRecommendName(t *testing.T) {
	for _, c := range []struct {
		name       string
		titles     []string
		latest     string
		files      []string
		want       string
		wantBasis  string
		wantSubstr string // for guesses where only the term matters
	}{
		{"反复出现的词", []string{"报销流程梳理", "差旅报销单据整理", "报销系统字段设计", "发票识别脚本"}, "发票识别脚本", nil, "报销", BasisTitles, ""},
		{"英文词", []string{"Kubernetes pod crash", "kubernetes ingress setup", "Fix kubernetes helm chart", "how to use docker"}, "", nil, "kubernetes", BasisTitles, ""},
		{"通用词不算，会话多了也不拿单个标题凑数", []string{"如何写周报", "如何查天气", "怎么做饭", "帮我翻译"}, "帮我翻译", nil, "", "", ""},
		{"保留原来的大小写", []string{"LDAP 账号同步", "ldap 证书", "OpenLDAP 备份", "LDAP 迁移"}, "", nil, "LDAP", BasisTitles, ""},
		{"通用动词不算", []string{"创建用户", "创建分组", "创建目录"}, "创建目录", nil, "", "", ""},
		{"通用名词不算", []string{"项目建议一", "项目建议二", "项目建议三"}, "项目建议三", nil, "", "", ""},
		{"只有两个会话时用最近的标题", []string{"甲乙丙丁戊", "己庚辛壬癸"}, "己庚辛壬癸", nil, "己庚辛壬癸", BasisLatest, ""},
		{"截图和时间戳文件名不算", []string{"某个对话"}, "某个对话", []string{"image(20261029-162755).png", "2026-11-06_112129.pdf", "屏幕截图 2026.png", "项目计划书.docx"}, "项目计划书", BasisFile, ""},
		{"大项目里分散的标题也能取到词", manyTitles(), "", nil, "备份方案", BasisTitles, ""},
		{"标题太少用文件名", []string{"某个对话"}, "某个对话", []string{"季度复盘模板.xlsx", "指标说明.pdf"}, "季度复盘模板", BasisFile, ""},
		{"没有文件用最近标题", []string{"一次性的话题"}, "一次性的话题", nil, "一次性的话题", BasisLatest, ""},
		{"最近标题限长", []string{"这是一个非常非常非常非常非常长的会话标题用来测试截断"}, "这是一个非常非常非常非常非常长的会话标题用来测试截断", nil, "", BasisLatest, "这是一个非常"},
		{"什么都没有", nil, "", nil, "", "", ""},
	} {
		got, basis := recommendName(c.titles, c.latest, c.files)
		if basis != c.wantBasis || (c.want != "" && got != c.want) || (c.wantSubstr != "" && !strings.HasPrefix(got, c.wantSubstr)) {
			t.Errorf("%s: got (%q, %q), want (%q%s, %q)", c.name, got, basis, c.want, c.wantSubstr, c.wantBasis)
		}
		if len([]rune(got)) > recommendMaxRunes+8 {
			t.Errorf("%s: 推荐名过长: %q", c.name, got)
		}
	}
}

// Fragments of longer words, one-character edges and the generic request words are no names.
func TestRecommendNameSkipsFragments(t *testing.T) {
	titles := []string{"关于合同审查的问题", "合同审查要点", "审查合同条款的方法", "合同模板的修改"}
	got, basis := recommendName(titles, "", nil)
	if basis != BasisTitles || !strings.Contains(got, "合同") {
		t.Fatalf("got (%q, %q)", got, basis)
	}
	for _, bad := range []string{"的", "关于", "问题", "方法"} {
		if strings.Contains(got, bad) {
			t.Errorf("推荐名 %q 含无意义的片段 %q", got, bad)
		}
	}
	// the longest of terms that always come together wins
	if got2, _ := recommendName([]string{"合同审查A", "合同审查B", "合同审查C"}, "", nil); got2 != "合同审查" {
		t.Errorf("got %q，应取总是一起出现的最长词", got2)
	}
}

func TestRecommendNameIsStable(t *testing.T) {
	titles := []string{"预算表公式", "预算表模板", "年度预算汇总", "预算审批流程"}
	first, _ := recommendName(titles, "", nil)
	for i := 0; i < 20; i++ {
		if again, _ := recommendName(titles, "", nil); again != first {
			t.Fatalf("推荐名不稳定: %q vs %q", first, again)
		}
	}
}

// A big project: 60 titles on many topics, "备份" in eight of them. The filler titles
// share no characters, so no word of theirs comes back.
func manyTitles() []string {
	var out []string
	for i := 0; i < 52; i++ {
		base := rune(0x4E00 + 8*i)
		out = append(out, string([]rune{base, base + 1, base + 2, base + 3}))
	}
	for i := 0; i < 8; i++ {
		out = append(out, "备份方案"+string(rune('一'+i)))
	}
	return out
}
