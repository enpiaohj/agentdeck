package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 真实端到端：从本地服务按发布形态下载、校验、解压。默认跳过，用
// AGENTDECK_UPDATE_E2E_URL 指向服务根地址（如 http://127.0.0.1:8731）打开。
//
// 这不是模拟：服务返回的是按发布脚本打出来的 ZIP 与 SHA256SUMS，exe 是真实
// 构建产物。断言只检查「下载并校验成功、解压出的 exe 可用」，不绑定具体版本号
// ——服务端版本会随发布变化，写死版本号会让这个测试在下次发版后失效。
func TestLiveDownloadAndInstall(t *testing.T) {
	url := os.Getenv("AGENTDECK_UPDATE_E2E_URL")
	if url == "" {
		t.Skip("未设置 AGENTDECK_UPDATE_E2E_URL，跳过真实下载验证")
	}
	// 用一个足够低的当前版本，保证服务端无论发布到哪个版本都会被判定为可用。
	client := Client{APIURL: url + "/releases/latest", Version: "0.0.0"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := client.Check(ctx, "0.0.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Available || result.Latest == "" {
		t.Fatalf("应当发现可用的新版本：%+v", result)
	}
	dir := t.TempDir()
	out, err := client.Download(ctx, result, dir, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	exe := filepath.Join(out, result.ExeAsset.Name)
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("解压后缺少 exe：%v", err)
	}
	t.Logf("下载并校验通过：%s（%d 字节）", exe, info.Size())
	if info.Size() < 1<<20 {
		t.Fatalf("exe 大小异常：%d", info.Size())
	}
}
