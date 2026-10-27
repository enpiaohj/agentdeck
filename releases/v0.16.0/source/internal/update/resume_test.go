package update

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// bigPayload 返回确定性的大载荷（约 300 KB），足以跨过 128 KiB 的进度节流。
func bigPayload() []byte {
	out := make([]byte, 300<<10)
	for i := range out {
		out[i] = byte(i*7 + i/251)
	}
	return out
}

func resumeResult(server *httptest.Server, name string, data []byte) Result {
	return Result{
		Latest: "1.1.0", Available: true,
		ExeAsset: Asset{Name: name, URL: server.URL + "/" + name, Size: int64(len(data)), Digest: digestOf(data)},
	}
}

const resumeName = AssetPrefix + "1.1.0" + AssetSuffix

// 上次中断留下的 .part：用 Range 从断点继续，最终文件完整且通过整体校验。
func TestDownloadResumesFromPartial(t *testing.T) {
	data := bigPayload()
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		http.ServeContent(w, r, resumeName, time.Time{}, bytes.NewReader(data))
	}))
	defer server.Close()

	dir := t.TempDir()
	const have = 100 << 10
	if err := os.WriteFile(filepath.Join(dir, resumeName+".part"), data[:have], 0o644); err != nil {
		t.Fatal(err)
	}
	var first int64 = -1
	out, err := Client{}.Download(context.Background(), resumeResult(server, resumeName, data), dir, func(done, total int64) {
		if first < 0 {
			first = done
		}
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := os.ReadFile(out)
	if !bytes.Equal(got, data) {
		t.Fatal("续传后的内容与原始载荷不一致")
	}
	if len(ranges) != 1 || ranges[0] != "bytes=102400-" {
		t.Fatalf("应当带 Range 续传，实际请求头：%q", ranges)
	}
	if first < have {
		t.Fatalf("进度应从断点开始，首次回调 done=%d", first)
	}
	if _, err = os.Stat(out + ".part"); !os.IsNotExist(err) {
		t.Fatal("完成后不应留下 .part")
	}
}

// 服务器不认 Range（返回 200 完整内容）：从头写，结果同样正确。
func TestDownloadRestartsWhenServerIgnoresRange(t *testing.T) {
	data := bigPayload()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer server.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, resumeName+".part"), data[:50<<10], 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Client{}.Download(context.Background(), resumeResult(server, resumeName, data), dir, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, data) {
		t.Fatal("内容不一致")
	}
}

// 416：.part 与服务器对不上，丢掉重来。
func TestDownloadRestartsOnRangeNotSatisfiable(t *testing.T) {
	data := bigPayload()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Write(data)
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, resumeName+".part"), data[:10<<10], 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Client{}.Download(context.Background(), resumeResult(server, resumeName, data), dir, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, data) {
		t.Fatal("内容不一致")
	}
}

// .part 被改过（长度对、内容错）：续传后整体校验必须失败，并且不留半成品。
func TestDownloadRejectsCorruptPartial(t *testing.T) {
	data := bigPayload()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, resumeName, time.Time{}, bytes.NewReader(data))
	}))
	defer server.Close()
	dir := t.TempDir()
	bad := bytes.Repeat([]byte{0xAA}, 80<<10)
	if err := os.WriteFile(filepath.Join(dir, resumeName+".part"), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Client{}.Download(context.Background(), resumeResult(server, resumeName, data), dir, nil)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("应当因整体校验失败而报错，实际：%v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("校验失败不应留下文件：%v", entries)
	}
}

// 网络中断后保留 .part，再次下载只补缺的部分。
func TestInterruptedDownloadKeepsPartAndResumes(t *testing.T) {
	data := bigPayload()
	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempt.Add(1) == 1 {
			w.Header().Set("Content-Length", "307200")
			w.Write(data[:150<<10])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		http.ServeContent(w, r, resumeName, time.Time{}, bytes.NewReader(data))
	}))
	defer server.Close()
	dir := t.TempDir()
	result := resumeResult(server, resumeName, data)

	if _, err := (Client{}).Download(context.Background(), result, dir, nil); err == nil {
		t.Fatal("连接被中途断开时应当报错")
	}
	part := filepath.Join(dir, resumeName+".part")
	info, err := os.Stat(part)
	if err != nil || info.Size() == 0 || info.Size() >= int64(len(data)) {
		t.Fatalf("中断后应保留不完整的 .part：%v %v", info, err)
	}
	out, err := Client{}.Download(context.Background(), result, dir, nil)
	if err != nil {
		t.Fatalf("续传: %v", err)
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, data) {
		t.Fatal("续传后的内容不一致")
	}
}

// 用户取消不留半成品。
func TestCanceledDownloadRemovesPart(t *testing.T) {
	data := bigPayload()
	var stuck atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "307200")
		w.Write(data[:200<<10])
		w.(http.Flusher).Flush()
		// 之后继续慢慢写：客户端关掉连接后，写入很快会失败，handler 随之退出。
		// 退出得快，说明取消真的断开了连接，而不是留一个后台下载。
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			if _, err := w.Write(data[200<<10 : 201<<10]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
		stuck.Store(true)
	}))
	defer server.Close()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := Client{}.Download(ctx, resumeResult(server, resumeName, data), dir, func(done, total int64) {
		if done >= 128<<10 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("取消后应当返回错误")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("取消后不应留下文件：%v", entries)
	}
	// 关服务器会等 handler 退出；若取消没有断开连接，上面的循环会跑满 3 秒。
	server.Close()
	if stuck.Load() {
		t.Fatal("取消后连接仍然开着，后台还在下载")
	}
}

// 同名安装包已在且校验通过：直接复用，不发任何请求。
func TestDownloadReusesVerifiedPackage(t *testing.T) {
	data := bigPayload()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	dir := t.TempDir()
	final := filepath.Join(dir, resumeName)
	if err := os.WriteFile(final, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var reported int64
	out, err := Client{}.Download(context.Background(), resumeResult(server, resumeName, data), dir, func(done, total int64) { reported = done })
	if err != nil || out != final {
		t.Fatalf("应当直接复用：%q %v", out, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("复用时不应有网络请求，实际 %d 次", hits.Load())
	}
	if reported != int64(len(data)) {
		t.Fatalf("复用时应报告完成进度，实际 %d", reported)
	}
}

// 同名但内容不对（被改过或上次没写完）：不复用，重新下载覆盖。
func TestDownloadDoesNotReuseCorruptPackage(t *testing.T) {
	data := bigPayload()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer server.Close()
	dir := t.TempDir()
	wrong := bytes.Repeat([]byte{1}, len(data))
	if err := os.WriteFile(filepath.Join(dir, resumeName), wrong, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Client{}.Download(context.Background(), resumeResult(server, resumeName, data), dir, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, data) {
		t.Fatal("应当用新下载的内容覆盖损坏的同名文件")
	}
}

// 其他版本遗留的安装包与 .part 会被清掉；目录里的其他文件（check.json）与别人的文件不动。
func TestDownloadRemovesStalePackagesOnly(t *testing.T) {
	data := bigPayload()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer server.Close()
	dir := t.TempDir()
	stale := []string{"AgentDeck-v1.0.0-win-x64.exe", "AgentDeck-v1.0.5-win-x64.exe.part"}
	keep := []string{"check.json", "notes.txt", "AgentDeck-v1.0.0-win-arm64.exe"}
	for _, name := range append(append([]string{}, stale...), keep...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (Client{}).Download(context.Background(), resumeResult(server, resumeName, data), dir, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s 应当被清掉", name)
		}
	}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s 不应被动：%v", name, err)
		}
	}
}

// 有新版本但没有可下载并校验的文件：不给下载，但要如实标出「更新却暂无文件」。
func TestCheckFlagsNewerReleaseWithoutAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(releaseJSON("v1.1.0", "notes.txt", serverURL(r)+"/notes.txt"))
	}))
	defer server.Close()

	result, err := Client{APIURL: server.URL}.Check(context.Background(), "0.9.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Available || !result.AssetMissing || result.Latest != "1.1.0" {
		t.Fatalf("应为「更新但暂无可下载文件」：%+v", result)
	}
	same, err := Client{APIURL: server.URL}.Check(context.Background(), "1.1.0")
	if err != nil || same.AssetMissing || same.Available {
		t.Fatalf("已是最新时不应标出暂无文件：%+v %v", same, err)
	}
}
