package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// httpClient 返回实际使用的客户端：10 秒超时，足够覆盖慢网络下的
// 一次 JSON 查询，又不至于让「检查更新」按钮长时间无响应。
func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
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

	resp, err := c.httpClient().Do(req)
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
	if asset, ok := release.ExeAsset(); ok {
		result.ExeAsset = asset
	} else {
		result.Available = false
	}
	if asset, ok := release.SumsAsset(); ok {
		result.SumsAsset = asset
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
