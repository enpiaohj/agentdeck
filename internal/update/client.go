package update

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// 发布仓库与资产命名。与 README、发布流程保持一致：资产名形如
// AgentDeck-v0.9.0-win-x64.exe，校验清单是 SHA256SUMS。
const (
	LatestReleaseURL = "https://api.github.com/repos/enpiaohj/agentdeck/releases/latest"
	ReleasePageURL   = "https://github.com/enpiaohj/agentdeck/releases"
	AssetPrefix      = "AgentDeck-v"
	AssetSuffix      = "-win-x64.exe"
	SumsAsset        = "SHA256SUMS"
	userAgentPrefix  = "AgentDeck/"

	// checkTimeout 是「检查更新」这类一次 JSON 查询的上限。
	checkTimeout = 10 * time.Second
)

// 失败原因分类，界面据此给出不同的下一步提示。
var (
	// ErrNotFound 表示仓库还没有正式发布（或 latest 不存在）。
	ErrNotFound = errors.New("没有找到正式发布版本")
	// ErrRateLimited 表示 GitHub 的匿名额度用尽。
	ErrRateLimited = errors.New("GitHub 接口访问过于频繁，请稍后再试")
)

// Asset 是 Release 附带的文件。
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
	// Digest 是 GitHub 随资产一起给出的摘要，形如 "sha256:<64 位十六进制>"。
	// 有它就按它校验下载内容；发布早于该字段时回退到 SHA256SUMS（见 Download）。
	Digest string `json:"digest"`
}

// SHA256 返回资产自带的 SHA-256（小写十六进制），没有或格式不对时返回空串。
func (a Asset) SHA256() string {
	algo, value, ok := strings.Cut(strings.TrimSpace(a.Digest), ":")
	if !ok || !strings.EqualFold(algo, "sha256") {
		return ""
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

// Release 是一次正式发布的摘要。
type Release struct {
	Tag         string    `json:"tag_name"`
	Name        string    `json:"name"`
	Notes       string    `json:"body"`
	PageURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	Assets      []Asset   `json:"assets"`
}

// Version 返回去掉前导 v 的版本号，无法识别时返回空串。
func (r Release) Version() string {
	v, err := ParseVersion(r.Tag)
	if err != nil {
		return ""
	}
	return v.String()
}

// ExeAsset 返回本平台的安装包资产。
func (r Release) ExeAsset() (Asset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == AssetPrefix+r.Version()+AssetSuffix {
			return asset, true
		}
	}
	return Asset{}, false
}

// SumsAsset 返回校验清单资产。
func (r Release) SumsAsset() (Asset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == SumsAsset {
			return asset, true
		}
	}
	return Asset{}, false
}

// Result 是一次检查的结论。
type Result struct {
	// Current 是本程序的版本。
	Current string
	// Latest 是远端最新正式版的版本号；无法解析时为空。
	Latest string
	// Available 为真表示 Latest 比 Current 新。
	Available bool
	// Notes 是发布说明正文（Markdown）。
	Notes string
	// PageURL 是 Release 页面。
	PageURL string
	// PublishedAt 是发布时间。
	PublishedAt time.Time
	// ExeAsset 与 SumsAsset 在 Available 为真时可用。
	ExeAsset  Asset
	SumsAsset Asset
	// CheckedAt 是本次检查的时间。
	CheckedAt time.Time
}

// Client 查询 GitHub Releases。字段可替换以便测试指向本机服务。
type Client struct {
	// APIURL 是 latest release 接口；空用 LatestReleaseURL。
	APIURL string
	// HTTP 是请求用的客户端；空用带超时的默认客户端。
	HTTP *http.Client
	// Version 用于 User-Agent，便于在 GitHub 侧分辨来源。
	Version string
}

// checkClient 是查询用的客户端：10 秒总超时足够覆盖一次 JSON 查询，又不至于让
// 「检查更新」按钮长时间无响应。
func (c Client) checkClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: checkTimeout}
}

// downloadClient 是下载用的客户端：**不设整次请求的总超时**。
//
// 20 MB 的包在慢链路（国内直连 GitHub 常见 100 KB/s～2 MB/s）下会远超任何
// 合理的总时限，用 10 秒的检查客户端会让下载每次都在同一秒数被砍断（曾如此）。
// 卡死由 stallTimeout 的停滞看门狗与调用方的 context 上限兜住。
func (c Client) downloadClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}}
}

// Check 查询最新的正式发布，并与 current 比较。
//
// 网络或解析失败时返回错误；没有正式发布时返回 ErrNotFound；额度用尽
// 返回 ErrRateLimited。其余情况一律返回 Result（Available 可能为假）。
func (c Client) Check(ctx context.Context, current string) (Result, error) {
	url := c.APIURL
	if url == "" {
		url = LatestReleaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgentPrefix+firstNonEmpty(c.Version, "dev"))

	resp, err := c.checkClient().Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("无法连接 GitHub：%w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Result{}, ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		return Result{}, ErrRateLimited
	default:
		return Result{}, fmt.Errorf("GitHub 返回 %s", resp.Status)
	}

	var release Release
	// 限制读取量：接口正常情况下只有几十 KB，异常响应不该被整段读进内存。
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&release); err != nil {
		return Result{}, fmt.Errorf("无法解析 GitHub 的响应：%w", err)
	}
	if release.Draft || release.Prerelease {
		return Result{}, ErrNotFound
	}
	latest := release.Version()
	if latest == "" {
		return Result{}, fmt.Errorf("无法识别的发布版本号：%s", release.Tag)
	}
	result := Result{
		Current:     current,
		Latest:      latest,
		Available:   IsNewer(latest, current),
		Notes:       strings.TrimSpace(release.Notes),
		PageURL:     release.PageURL,
		PublishedAt: release.PublishedAt,
		CheckedAt:   time.Now(),
	}
	// 可下载的前提：有这个平台的 exe 资产，并且**能校验它**——资产自带 digest，
	// 或者发布提供了 SHA256SUMS。两者都没有时不报「有新版本」：那只会诱使用户
	// 点一个注定失败的下载。
	exeAsset, hasExe := release.ExeAsset()
	sumsAsset, hasSums := release.SumsAsset()
	if hasExe && (hasSums || exeAsset.SHA256() != "") {
		result.ExeAsset = exeAsset
		result.SumsAsset = sumsAsset
	} else {
		result.Available = false
	}
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
