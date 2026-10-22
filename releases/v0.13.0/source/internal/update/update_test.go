package update

import (
	"archive/zip"
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

// releaseJSON 生成接口响应的最小 JSON。
func releaseJSON(tag string, assets ...string) []byte {
	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	}
	payload := struct {
		Tag     string  `json:"tag_name"`
		Name    string  `json:"name"`
		Body    string  `json:"body"`
		PageURL string  `json:"html_url"`
		Assets  []asset `json:"assets"`
	}{Tag: tag, Name: tag, Body: "## 变更\n- 修复了某处", PageURL: "https://example.test/release"}
	for i := 0; i+1 < len(assets); i += 2 {
		payload.Assets = append(payload.Assets, asset{Name: assets[i], URL: assets[i+1], Size: 1})
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

// buildZip 生成一个分发包，entries 是「ZIP 内路径 → 内容」。
func buildZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("构造 ZIP: %v", err)
		}
		if _, err = entry.Write(body); err != nil {
			t.Fatalf("构造 ZIP: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("构造 ZIP: %v", err)
	}
	return buf.Bytes()
}

func TestDownloadVerifiesAndExtracts(t *testing.T) {
	const version = "1.1.0"
	root := "AgentDeck-v" + version + "-win-x64"
	exe := []byte("fake exe payload")
	zip := buildZip(t, map[string][]byte{
		root + "/AgentDeck-v" + version + "-win-x64.exe": exe,
		root + "/CHANGELOG.md":                           []byte("# 变更\n"),
	})
	sum := sha256.Sum256(zip)
	exeSum := sha256.Sum256(exe)
	sums := hex.EncodeToString(sum[:]) + "  AgentDeck-v" + version + "-win-x64.zip\n" +
		hex.EncodeToString(exeSum[:]) + "  AgentDeck-v" + version + "-win-x64.exe\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case SumsAsset:
			w.Write([]byte(sums))
		default:
			w.Write(zip)
		}
	}))
	defer server.Close()

	result := Result{
		Current:   "0.9.0",
		Latest:    version,
		Available: true,
		ExeAsset:  Asset{Name: AssetPrefix + version + AssetSuffix, URL: server.URL + "/pkg.zip"},
		SumsAsset: Asset{Name: SumsAsset, URL: server.URL + "/" + SumsAsset},
	}
	dir := t.TempDir()
	var progressCalls int
	out, err := Client{Version: "0.9.0"}.Download(context.Background(), result, dir, func(done, total int64) {
		progressCalls++
		if done <= 0 || done > total {
			t.Errorf("进度不合理：%d/%d", done, total)
		}
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if out != filepath.Join(dir, root) {
		t.Fatalf("解压目录 = %s", out)
	}
	got, err := os.ReadFile(filepath.Join(out, AssetPrefix+version+AssetSuffix))
	if err != nil || string(got) != string(exe) {
		t.Fatalf("解压内容不符：%q, %v", got, err)
	}
	if progressCalls == 0 {
		t.Fatal("应当报告过下载进度")
	}
}

func TestDownloadRejectsHashMismatch(t *testing.T) {
	const version = "1.1.0"
	root := "AgentDeck-v" + version + "-win-x64"
	zip := buildZip(t, map[string][]byte{root + "/" + AssetPrefix + version + AssetSuffix: []byte("payload")})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == SumsAsset {
			// 故意给出错误的哈希：与内容不符时必须丢弃。
			w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  AgentDeck-v1.1.0-win-x64.zip\n" +
				"0000000000000000000000000000000000000000000000000000000000000000  AgentDeck-v1.1.0-win-x64.exe\n"))
			return
		}
		w.Write(zip)
	}))
	defer server.Close()

	result := Result{
		Latest:    version,
		Available: true,
		ExeAsset:  Asset{Name: AssetPrefix + version + AssetSuffix, URL: server.URL + "/pkg.zip"},
		SumsAsset: Asset{Name: SumsAsset, URL: server.URL + "/" + SumsAsset},
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

func TestExtractRejectsUnexpectedEntry(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "pkg.zip")
	zip := buildZip(t, map[string][]byte{
		"AgentDeck-v1.1.0-win-x64/evil.exe": nil,
		"../outside.exe":                    []byte("x"),
	})
	if err := os.WriteFile(zipPath, zip, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := extract(zipPath, dir, "AgentDeck-v1.1.0-win-x64", Result{ExeAsset: Asset{Name: "evil.exe"}}, map[string]string{"evil.exe": ""})
	if err == nil {
		t.Fatal("包含越界路径的包应当被拒绝")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "outside.exe")); !os.IsNotExist(statErr) {
		t.Fatal("越界文件不该被写出")
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
