//go:build windows

package updateinstall

import (
	"os"
	"path/filepath"
	"testing"
)

// 真实产物端到端：用两个发布出来的真实 exe 走一遍「退出时落地 + 下次启动
// 善后」，确认字节真的换掉了。默认跳过，用 AGENTDECK_UPDATE_E2E_OLD / _NEW
// 指定两个 exe。
func TestRealArtifactsInstall(t *testing.T) {
	oldExe := os.Getenv("AGENTDECK_UPDATE_E2E_OLD")
	newExe := os.Getenv("AGENTDECK_UPDATE_E2E_NEW")
	if oldExe == "" || newExe == "" {
		t.Skip("未指定真实产物，跳过")
	}
	dir := t.TempDir()
	current := filepath.Join(dir, filepath.Base(oldExe))
	if err := copyFile(oldExe, current); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), filepath.Base(newExe))
	if err := copyFile(newExe, pkg); err != nil {
		t.Fatal(err)
	}

	if err := Probe(current); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	target := ResolveTarget(current, pkg)
	if filepath.Base(target) != filepath.Base(newExe) {
		t.Fatalf("落地路径 = %q，期望跟随 %q", target, filepath.Base(newExe))
	}
	if !Install(current, target, pkg) {
		t.Fatal("Install 失败")
	}
	// 落地后的文件必须与更新包逐字节一致：这是「用户真的换到了新版本」。
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(newExe)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("落地后大小 %d，期望 %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("落地内容在字节 %d 处不一致", i)
		}
	}
	t.Logf("落地完成：%s（%d 字节）", target, len(got))

	Cleanup(target, nil)
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatal("旧版本应当被删除")
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}
