package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// 导出文件以 UTF-8 BOM 开头；重新导出认得出带 BOM 的旧文件（更新原文件，不另存一份），
// 也认得出旧版本导出的、没有 BOM 的文件。
func TestExportMarkdownStartsWithBOMAndReexportReusesFile(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	const id = "11111111-2222-3333-4444-555555555555"
	a.setList([]sessions.Session{claudeFixture(t, t.TempDir(), id, "导出带编码标记")})
	dest := t.TempDir()
	svc := &SessionService{a: a}

	first, err := svc.ExportMarkdown("claude:"+id, dest)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(first)
	if !strings.HasPrefix(string(b), exportBOM+"---\n") {
		t.Fatalf("导出文件应以 BOM 加 front matter 开头: %q", string(b[:12]))
	}
	if strings.Count(string(b), exportBOM) != 1 {
		t.Fatal("BOM 只该出现在文件最前面")
	}
	again, err := svc.ExportMarkdown("claude:"+id, dest)
	if err != nil || again != first {
		t.Fatalf("重新导出应更新原文件: %q vs %q (%v)", again, first, err)
	}

	// 旧版本导出的文件没有 BOM：仍然认作同一会话的旧导出
	old := []byte(strings.TrimPrefix(string(b), exportBOM))
	if err := os.WriteFile(first, old, 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := svc.ExportMarkdown("claude:"+id, dest)
	if err != nil || third != first {
		t.Fatalf("没有 BOM 的旧导出也应被更新而不是另存: %q vs %q (%v)", third, first, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dest, "*.md")); len(left) != 1 {
		t.Fatalf("目录里应只有一个导出文件: %v", left)
	}
	nb, _ := os.ReadFile(third)
	if !strings.HasPrefix(string(nb), exportBOM) {
		t.Fatal("更新后的文件应带 BOM")
	}
}
