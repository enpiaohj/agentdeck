package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"0.9.0", "0.9.0"},
		{"v0.9.0", "0.9.0"},
		{"1.2", "1.2.0"},
		{"1", "1.0.0"},
		{"1.0.0-beta.1", "1.0.0-beta.1"},
		{" v1.0.1 ", "1.0.1"},
	}
	for _, c := range cases {
		got, err := ParseVersion(c.text)
		if err != nil || got.String() != c.want {
			t.Fatalf("ParseVersion(%q) = %q, %v，期望 %q", c.text, got.String(), err, c.want)
		}
	}
	for _, bad := range []string{"", "dev", "1.2.3.4", "1.x.0", "-1.0.0", "v"} {
		if _, err := ParseVersion(bad); err == nil {
			t.Fatalf("ParseVersion(%q) 应当报错", bad)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.9.9", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
		{"1.0.0-beta.1", "1.0.0", false},
		{"1.0.0", "1.0.0-beta.1", true},
		// 当前版本读不出来（开发构建）时不报「有新版本」。
		{"9.9.9", "dev", false},
		{"not-a-version", "0.9.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.candidate, c.current); got != c.want {
			t.Fatalf("IsNewer(%q, %q) = %v，期望 %v", c.candidate, c.current, got, c.want)
		}
	}
}

// releaseJSON 生成接口响应的最小 JSON。每个资产都带一个由名字推出的 digest：
// 真实发布里 GitHub 就是这么给的，检查路径要把它带回来。
func releaseJSON(tag string, assets ...string) []byte {
	type asset struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	}
	payload := struct {
		Tag     string  `json:"tag_name"`
		Name    string  `json:"name"`
		Body    string  `json:"body"`
		PageURL string  `json:"html_url"`
		Assets  []asset `json:"assets"`
	}{Tag: tag, Name: tag, Body: "## 变更\n- 修复了某处", PageURL: "https://example.test/release"}
	for i := 0; i+1 < len(assets); i += 2 {
		sum := sha256.Sum256([]byte(assets[i]))
		payload.Assets = append(payload.Assets, asset{
			Name: assets[i], URL: assets[i+1], Size: 1,
			Digest: "sha256:" + hex.EncodeToString(sum[:]),
		})
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func TestCheckFindsNewerRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "AgentDeck/0.9.0" {
			t.Errorf("User-Agent = %q", got)
		}
		name := AssetPrefix + "1.1.0" + AssetSuffix
		w.Write(releaseJSON("v1.1.0", name, serverURL(r)+"/"+name, SumsAsset, serverURL(r)+"/"+SumsAsset))
	}))
	defer server.Close()

	client := Client{APIURL: server.URL, Version: "0.9.0"}
	result, err := client.Check(context.Background(), "0.9.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Available || result.Latest != "1.1.0" || result.ExeAsset.Name == "" || result.SumsAsset.Name != SumsAsset {
		t.Fatalf("结果不符：%+v", result)
	}
	if result.Notes == "" || result.PageURL == "" {
		t.Fatalf("发布说明与页面地址应当带回来：%+v", result)
	}
	// digest 必须在检查这一步就带回来：下载校验优先用它，省掉一次清单请求。
	if result.ExeAsset.SHA256() == "" {
		t.Fatalf("检查结果应当带上资产的 digest：%+v", result.ExeAsset)
	}
}

// serverURL 返回测试服务自身的地址（请求已到达服务，Host 头即可）。
func serverURL(r *http.Request) string { return "http://" + r.Host }

func TestCheckOnLatestReleaseIsNotAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := AssetPrefix + "0.9.0" + AssetSuffix
		w.Write(releaseJSON("v0.9.0", name, serverURL(r)+"/"+name, SumsAsset, serverURL(r)+"/"+SumsAsset))
	}))
	defer server.Close()

	result, err := Client{APIURL: server.URL, Version: "0.9.0"}.Check(context.Background(), "0.9.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Available || result.Latest != "0.9.0" {
		t.Fatalf("同版本不应报告有新版本：%+v", result)
	}
}

// 发布缺少本平台分发文件时不能诱使用户点一个注定失败的「下载」。
func TestCheckWithoutAssetIsNotAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(releaseJSON("v1.1.0", SumsAsset, serverURL(r)+"/"+SumsAsset))
	}))
	defer server.Close()

	result, err := Client{APIURL: server.URL}.Check(context.Background(), "0.9.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Available || result.ExeAsset.Name != "" {
		t.Fatalf("缺少分发文件时不应可下载：%+v", result)
	}
}

func TestCheckErrorStatuses(t *testing.T) {
	for _, c := range []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrNotFound},
		{http.StatusForbidden, ErrRateLimited},
		{http.StatusTooManyRequests, ErrRateLimited},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
		}))
		_, err := Client{APIURL: server.URL}.Check(context.Background(), "0.9.0")
		server.Close()
		if !errors.Is(err, c.want) {
			t.Fatalf("HTTP %d 返回 %v，期望 %v", c.status, err, c.want)
		}
	}
}

// exeBytes 是测试用的「可执行文件」内容，只需是确定的字节。
func exeBytes() []byte { return []byte("MZ fake executable payload") }

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// 回归用例，守着一个曾经让应用内更新 100% 失败的缺陷。
//
// 发布里 exe 与 zip 是两个内容不同的资产。当时的代码下载 ExeAsset.URL（exe），
// 却按 exe 名字推出 zip 名、取 SHA256SUMS 里 zip 的哈希来比对，于是每次都在
// 下载完成之后被判「哈希不一致」并丢弃；单元测试之所以没发现，是因为夹具把
// ExeAsset.URL 指到了返回 ZIP 字节的地址。这里让 exe 的地址返回 exe 字节，
// 一旦比对对象又变回 ZIP，本用例立刻失败。
func TestDownloadVerifiesExeAsset(t *testing.T) {
	const version = "1.1.0"
	exe := exeBytes()
	name := AssetPrefix + version + AssetSuffix
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) != name {
			t.Errorf("下载路径 = %s，期望就是 exe 资产本身", r.URL.Path)
		}
		w.Write(exe)
	}))
	defer server.Close()

	result := Result{
		Current: "0.9.0", Latest: version, Available: true,
		ExeAsset: Asset{Name: name, URL: server.URL + "/" + name, Size: int64(len(exe)), Digest: digestOf(exe)},
	}
	dir := t.TempDir()
	var calls int
	out, err := Client{Version: "0.9.0"}.Download(context.Background(), result, dir, func(done, total int64) {
		calls++
		if done <= 0 || (total != 0 && total != int64(len(exe))) {
			t.Errorf("进度不合理：%d/%d", done, total)
		}
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if out != filepath.Join(dir, name) {
		t.Fatalf("落地路径 = %s", out)
	}
	got, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(got, exe) {
		t.Fatalf("下载内容不符：%q %v", got, err)
	}
	if calls == 0 {
		t.Fatal("应当报告过下载进度")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("目录里应当只有下载好的更新包：%v", entries)
	}
}

func TestDownloadRejectsHashMismatch(t *testing.T) {
	const version = "1.1.0"
	exe := exeBytes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(exe)
	}))
	defer server.Close()

	result := Result{
		Latest: version, Available: true,
		ExeAsset: Asset{
			Name: AssetPrefix + version + AssetSuffix, URL: server.URL + "/pkg",
			Size: int64(len(exe)), Digest: "sha256:" + strings.Repeat("0", 64),
		},
	}
	dir := t.TempDir()
	if _, err := (Client{}).Download(context.Background(), result, dir, nil); err == nil {
		t.Fatal("哈希不一致时应当失败")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("失败后不应留下文件：%v", entries)
	}
}

// 大小与发布记录不一致时同样丢弃：哈希相同但长度不同只可能是发布记录或传输坏了。
func TestDownloadRejectsSizeMismatch(t *testing.T) {
	const version = "1.1.0"
	exe := exeBytes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(exe)
	}))
	defer server.Close()

	result := Result{
		Latest: version, Available: true,
		ExeAsset: Asset{
			Name: AssetPrefix + version + AssetSuffix, URL: server.URL + "/pkg",
			Size: int64(len(exe)) + 1, Digest: digestOf(exe),
		},
	}
	if _, err := (Client{}).Download(context.Background(), result, t.TempDir(), nil); err == nil {
		t.Fatal("大小不一致时应当失败")
	}
}

// 发布早于 GitHub 提供 digest 字段时回退到 SHA256SUMS 里的同名行。
func TestDownloadFallsBackToSums(t *testing.T) {
	const version = "1.1.0"
	exe := exeBytes()
	name := AssetPrefix + version + AssetSuffix
	sum := sha256.Sum256(exe)
	sums := hex.EncodeToString(sum[:]) + " *" + name + "\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == SumsAsset {
			w.Write([]byte(sums))
			return
		}
		w.Write(exe)
	}))
	defer server.Close()

	result := Result{
		Latest: version, Available: true,
		ExeAsset:  Asset{Name: name, URL: server.URL + "/" + name, Size: int64(len(exe))},
		SumsAsset: Asset{Name: SumsAsset, URL: server.URL + "/" + SumsAsset},
	}
	out, err := (Client{}).Download(context.Background(), result, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got, err := os.ReadFile(out); err != nil || !bytes.Equal(got, exe) {
		t.Fatalf("下载内容不符：%q %v", got, err)
	}
}

// 既没有资产 digest、又没有可用的校验清单时拒绝下载：宁可让用户手动去发布页，
// 也不装一个没法确认来源的文件。
func TestDownloadRefusesWithoutChecksum(t *testing.T) {
	const version = "1.1.0"
	exe := exeBytes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(exe)
	}))
	defer server.Close()

	result := Result{
		Latest: version, Available: true,
		ExeAsset: Asset{Name: AssetPrefix + version + AssetSuffix, URL: server.URL + "/pkg", Size: int64(len(exe))},
	}
	dir := t.TempDir()
	_, err := (Client{}).Download(context.Background(), result, dir, nil)
	if err == nil {
		t.Fatal("没有任何校验依据时应当拒绝")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("拒绝下载时不该留下文件：%v", entries)
	}
}

// 24 小时缓存里恢复出来的结论可能没有资产地址（旧版本写下的 check.json）。
// 这时必须给出能照做的提示，而不是一句「发布缺少分发文件」。
func TestDownloadWithoutAssetURL(t *testing.T) {
	result := Result{Latest: "1.1.0", Available: true}
	_, err := (Client{}).Download(context.Background(), result, t.TempDir(), nil)
	if err == nil {
		t.Fatal("缺少下载地址时应当失败")
	}
	if !strings.Contains(err.Error(), "重新检查") {
		t.Fatalf("错误应当指出下一步：%v", err)
	}
}

// 停滞看门狗：连接还在、但长时间没有任何数据到达时必须中断。
// 这一条同时守住「下载不能被任何总时限砍断」——本用例的 context 没有上限。
func TestDownloadAbortsWhenStalled(t *testing.T) {
	saved := stallTimeout
	stallTimeout = 200 * time.Millisecond
	defer func() { stallTimeout = saved }()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		<-release // 之后一直不再发数据，模拟链路卡住
	}))
	defer server.Close()
	// LIFO：先放开处理器，再关服务（Close 会等未完成的请求）。
	defer close(release)

	result := Result{
		Latest: "1.1.0", Available: true,
		ExeAsset: Asset{
			Name: "AgentDeck-v1.1.0-win-x64.exe", URL: server.URL + "/pkg",
			Size: 1048576, Digest: digestOf([]byte("x")),
		},
	}
	start := time.Now()
	_, err := (Client{}).Download(context.Background(), result, t.TempDir(), nil)
	if err == nil {
		t.Fatal("停滞时应当中断")
	}
	if !strings.Contains(err.Error(), "停滞") {
		t.Fatalf("错误应当说明是停滞：%v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("停滞判定太慢：%s", elapsed)
	}
}

// 下载用的客户端不能带整次请求的总超时：20 MB 的包在慢链路上会远超任何合理的
// 总时限，用查询用的 10 秒客户端会让下载每次都在同一秒数被砍断（曾如此）。
func TestClientTimeouts(t *testing.T) {
	client := Client{}
	if got := client.downloadClient().Timeout; got != 0 {
		t.Fatalf("下载客户端不应设总超时，实际 %s", got)
	}
	if got := client.checkClient().Timeout; got != checkTimeout {
		t.Fatalf("查询客户端超时 = %s，期望 %s", got, checkTimeout)
	}
	// 注入的客户端优先，测试才能指向本机服务。
	custom := &http.Client{Timeout: time.Second}
	injected := Client{HTTP: custom}
	if injected.downloadClient() != custom || injected.checkClient() != custom {
		t.Fatal("应当优先使用注入的客户端")
	}
}

func TestAssetSHA256(t *testing.T) {
	valid := strings.Repeat("a", 64)
	for _, c := range []struct {
		digest, want string
	}{
		{"sha256:" + valid, valid},
		{"SHA256:" + strings.ToUpper(valid), valid},
		{"sha256: " + valid, valid},
		{"", ""},
		{"sha256:", ""},
		{"sha256:" + valid + "0", ""},
		{"sha256:" + strings.Repeat("z", 64), ""},
		{"md5:" + valid, ""},
		{valid, ""},
	} {
		if got := (Asset{Digest: c.digest}).SHA256(); got != c.want {
			t.Fatalf("SHA256(%q) = %q，期望 %q", c.digest, got, c.want)
		}
	}
}

// 资产信息必须随检查结果落盘：否则「检查过之后当天再打开应用」会拿到一个
// 按钮可点、点下去必然失败的结论（曾如此）。
func TestStateAssetsRoundTrip(t *testing.T) {
	state := State{
		CheckedAt: time.Now().Truncate(time.Second), Current: "0.9.0", Latest: "1.1.0", Available: true,
		ExeName: "AgentDeck-v1.1.0-win-x64.exe", ExeURL: "https://example.test/x.exe",
		ExeSize: 42, ExeDigest: "sha256:" + strings.Repeat("a", 64), SumsURL: "https://example.test/SHA256SUMS",
	}
	path := filepath.Join(t.TempDir(), "check.json")
	if err := SaveState(path, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	exe, sums, ok := LoadState(path).Assets()
	if !ok {
		t.Fatal("资产应当随状态一起读回来")
	}
	if exe.URL != state.ExeURL || exe.Size != state.ExeSize || exe.SHA256() != strings.TrimPrefix(state.ExeDigest, "sha256:") || sums.URL != state.SumsURL {
		t.Fatalf("读回的资产不符：%+v %+v", exe, sums)
	}
	if _, _, ok := (State{}).Assets(); ok {
		t.Fatal("没有地址时应当报告不可用")
	}
	// 旧版本写下的 check.json 没有这些字段：读回来是「不可下载」，而不是装作可以。
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(legacy, []byte(`{"checked_at":"2026-01-01T00:00:00Z","latest":"1.1.0","available":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := LoadState(legacy).Assets(); ok {
		t.Fatal("旧状态没有下载地址，应报告不可用")
	}
}

func TestStateDue(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	if !StateDue(State{}, 24*time.Hour, now) {
		t.Fatal("从未检查过应当视为到期")
	}
	fresh := State{CheckedAt: now.Add(-time.Hour)}
	if StateDue(fresh, 24*time.Hour, now) {
		t.Fatal("一小时内检查过不该再查")
	}
	old := State{CheckedAt: now.Add(-25 * time.Hour)}
	if !StateDue(old, 24*time.Hour, now) {
		t.Fatal("超过一天应当到期")
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates", "check.json")
	want := State{CheckedAt: time.Now().Truncate(time.Second), Current: "0.9.0", Latest: "1.0.0", Available: true}
	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got := LoadState(path)
	if !got.CheckedAt.Equal(want.CheckedAt) || got.Latest != want.Latest || !got.Available {
		t.Fatalf("读回的状态不符：%+v", got)
	}
	if state := LoadState(filepath.Join(t.TempDir(), "missing.json")); !state.CheckedAt.IsZero() {
		t.Fatal("文件不存在时应当返回零值")
	}
	broken := filepath.Join(t.TempDir(), "broken.json")
	os.WriteFile(broken, []byte("{not json"), 0o644)
	if state := LoadState(broken); !state.CheckedAt.IsZero() {
		t.Fatal("内容损坏时应当返回零值")
	}
}
