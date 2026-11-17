package update

import (
	"context"
	"os"
	"testing"
	"time"
)

// 真实端到端：从真实发布接口检查、下载、校验。默认跳过，用
// AGENTDECK_UPDATE_E2E_URL 指向服务根地址打开：
//
//   - 本地镜像（如 http://127.0.0.1:8731）——它要按发布形态提供 /releases/latest
//     与资产地址；
//   - 真实的 https://api.github.com/repos/enpiaohj/agentdeck——这时下载的是
//     GitHub Releases 上的真实产物，走的就是用户点「下载并安装」那条路。
//
// 断言不绑定具体版本号：服务端版本会随发布变化，写死版本号会让这个测试在下次
// 发版后失效。
//
// 本机陷阱：`go test` 的子进程拿不到 shell 里的 HTTPS_PROXY（而 `go run` 拿得到），
// 于是它会直连 GitHub。本机实测直连只有约 55 KB/s 且常以 wsarecv 超时告终，
// 看起来像「下载坏了」，其实只是绕开了代理。用真实 GitHub 验证时按需在测试进程
// 内补上代理环境变量，别把结论建立在直连速率上。
func TestLiveDownloadAndInstall(t *testing.T) {
	url := os.Getenv("AGENTDECK_UPDATE_E2E_URL")
	if url == "" {
		t.Skip("未设置 AGENTDECK_UPDATE_E2E_URL，跳过真实下载验证")
	}
	// 用一个足够低的当前版本，保证服务端无论发布到哪个版本都会被判定为可用。
	client := Client{APIURL: url + "/releases/latest", Version: "0.0.0"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	start := time.Now()
	result, err := client.Check(ctx, "0.0.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Available || result.Latest == "" {
		t.Fatalf("应当发现可用的新版本：%+v", result)
	}
	t.Logf("检查用 %s；exe 资产=%s size=%d digest=%q，清单=%s",
		time.Since(start).Round(time.Millisecond), result.ExeAsset.URL, result.ExeAsset.Size, result.ExeAsset.Digest, result.SumsAsset.URL)
	dir := t.TempDir()
	var last int64
	exe, err := client.Download(ctx, result, dir, func(done, total int64) {
		if done-last >= 5<<20 {
			last = done
			t.Logf("已下载 %d / %d 字节（用时 %s）", done, total, time.Since(start).Round(time.Second))
		}
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("下载后缺少 exe：%v", err)
	}
	t.Logf("下载并校验通过：%s（%d 字节）", exe, info.Size())
	if info.Size() < 1<<20 {
		t.Fatalf("exe 大小异常：%d", info.Size())
	}
}
