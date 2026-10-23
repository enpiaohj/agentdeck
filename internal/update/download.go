package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// maxDownload 是单个资产的下载上限。正式包不到 25 MB，留足余量同时挡住
// 异常响应把磁盘写满。
const maxDownload = 256 << 20

// stallTimeout 是「一段时间内没有任何数据到达」的上限。
//
// 与「整次下载的总时限」不同：慢而稳定的链路不会被误杀，只有真的卡住才中断。
// 曾是变量以便测试调小；正常运行不要改它。
var stallTimeout = 45 * time.Second

// Progress 是下载进度回调：done 为已下载字节，total 为总字节（未知时为 0）。
type Progress func(done, total int64)

// Download 下载并校验新版本的可执行文件，返回它在 dir 下的完整路径。
//
// 只下载发布资产里那一个 exe：不经 ZIP 中转，因此不存在「下载 A 却按 B 校验」
// 的可能（曾经正是如此——下载 exe 却比对 ZIP 的哈希，导致应用内更新 100% 失败）。
// 校验优先用资产自带的 digest，没有时回退到 SHA256SUMS 里的同名行；大小也按
// 发布记录核对。先写 .part，全部通过才改名到位，目录里不会出现不完整的 exe。
//
// 三种省事的情形：同名文件已在且校验通过时直接复用，不再下载；上次中断留下的 .part
// 用 Range 续传（服务器不支持就从头来），最终仍对整个文件做 SHA-256；其他版本遗留的
// 安装包与 .part 在开始前清掉。校验失败与用户取消不留 .part。
func (c Client) Download(ctx context.Context, result Result, dir string, progress Progress) (string, error) {
	if !result.Available {
		return "", fmt.Errorf("没有可下载的新版本")
	}
	asset := result.ExeAsset
	if asset.URL == "" || asset.Name == "" {
		// 24 小时缓存里的检查结论不含资产地址（旧版本写入的 check.json 也没有）。
		return "", fmt.Errorf("本地缓存的检查结果里没有下载地址，请先重新检查更新")
	}
	want := asset.SHA256()
	if want == "" {
		sums, err := c.fetchSums(ctx, result.SumsAsset.URL)
		if err != nil {
			return "", fmt.Errorf("%w，无法校验下载内容", err)
		}
		value, ok := sums[asset.Name]
		if !ok {
			return "", fmt.Errorf("发布没有提供 %s 的 SHA-256，无法校验，已取消下载", asset.Name)
		}
		want = value
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建下载目录：%w", err)
	}
	final := filepath.Join(dir, asset.Name)
	staging := final + ".part"
	removeStalePackages(dir, asset.Name)

	if size, ok := verifiedFile(final, asset.Size, want); ok {
		_ = os.Remove(staging)
		if progress != nil {
			progress(size, size)
		}
		return final, nil
	}

	written, got, err := c.downloadTo(ctx, asset.URL, staging, asset.Size, progress)
	if err != nil {
		// 用户取消不留半成品；网络中断、停滞这类保留 .part，下次从断点继续。
		if errors.Is(ctx.Err(), context.Canceled) {
			_ = os.Remove(staging)
		}
		return "", err
	}
	if asset.Size > 0 && written != asset.Size {
		_ = os.Remove(staging)
		return "", fmt.Errorf("下载的更新包大小与发布记录不一致（期望 %d 字节，实际 %d 字节），已丢弃", asset.Size, written)
	}
	if got != want {
		_ = os.Remove(staging)
		return "", fmt.Errorf("下载的更新包未通过 SHA-256 校验（期望 %s，实际 %s），已丢弃", want, got)
	}
	if err := os.Rename(staging, final); err != nil {
		_ = os.Remove(staging)
		return "", fmt.Errorf("无法保存下载文件：%w", err)
	}
	return final, nil
}

// packageName 匹配本程序下载目录里自己产生的文件：安装包与它的 .part。
var packageName = regexp.MustCompile(`^AgentDeck-v\d+\.\d+\.\d+-win-x64\.exe(\.part)?$`)

// removeStalePackages 清掉 dir 里其他版本遗留的安装包与 .part（保留 keep 及其 .part）。
// 只认发布产物命名，不碰目录里的其他文件（如 check.json）。
func removeStalePackages(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !packageName.MatchString(name) || name == keep || name == keep+".part" {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// verifiedFile 判断 path 是否已经是完整、校验通过的安装包，返回其大小。
// size 为 0 表示发布记录里没有大小，只比哈希。
func verifiedFile(path string, size int64, want string) (int64, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 || (size > 0 && info.Size() != size) {
		return 0, false
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return 0, false
	}
	return info.Size(), hex.EncodeToString(digest.Sum(nil)) == want
}

// fetchSums 读取 SHA256SUMS，返回「文件名 → 小写十六进制哈希」。
// 清单格式与 sha256sum 一致：<hash> *<name>。
//
// 只在资产没有自带 digest 时使用（发布早于 GitHub 提供 digest 字段）。
func (c Client) fetchSums(ctx context.Context, url string) (map[string]string, error) {
	if url == "" {
		return nil, fmt.Errorf("发布没有提供校验清单")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgentPrefix+firstNonEmpty(c.Version, "dev"))
	resp, err := c.checkClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法下载校验清单：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载校验清单失败：HTTP %s", resp.Status)
	}
	sums := map[string]string{}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		hash, name, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if name == "" || len(hash) != 64 {
			continue
		}
		sums[name] = strings.ToLower(hash)
	}
	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取校验清单失败：%w", err)
	}
	if len(sums) == 0 {
		return nil, fmt.Errorf("校验清单为空或格式无法识别")
	}
	return sums, nil
}

// errRestart 表示续传请求被服务器拒绝或应答对不上，需要丢掉 .part 从头下载。
var errRestart = errors.New("续传请求未被接受")

// downloadTo 下载 url 到 dest，返回文件最终大小与内容的 SHA-256。
//
// dest 已有不完整内容（小于发布记录的大小）时用 Range 从断点续传：先把已有部分喂进
// 哈希，再追加新数据，所以返回的哈希覆盖整个文件。服务器不支持或应答对不上就丢弃
// .part 从头下。边写边算，不把整个文件读进内存。expected 是发布记录里的字节数，
// 用于进度显示与决定能否续传。
func (c Client) downloadTo(ctx context.Context, url, dest string, expected int64, progress Progress) (int64, string, error) {
	var offset int64
	if info, err := os.Stat(dest); err == nil && expected > 0 && info.Size() > 0 && info.Size() < expected {
		offset = info.Size()
	}
	written, sum, err := c.fetchInto(ctx, url, dest, expected, offset, progress)
	if errors.Is(err, errRestart) {
		_ = os.Remove(dest)
		return c.fetchInto(ctx, url, dest, expected, 0, progress)
	}
	return written, sum, err
}

func (c Client) fetchInto(ctx context.Context, url, dest string, expected, offset int64, progress Progress) (int64, string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", userAgentPrefix+firstNonEmpty(c.Version, "dev"))
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := c.downloadClient().Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("无法下载更新：%w", err)
	}
	defer resp.Body.Close()

	total := resp.ContentLength
	switch {
	case offset > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		return 0, "", errRestart
	case offset > 0 && resp.StatusCode == http.StatusPartialContent:
		if !strings.HasPrefix(resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-", offset)) {
			return 0, "", errRestart
		}
		if total > 0 {
			total += offset
		}
	case resp.StatusCode == http.StatusOK:
		// 没有请求续传，或服务器忽略了 Range 给出完整内容：从头写。
		offset = 0
	default:
		return 0, "", fmt.Errorf("下载更新失败：HTTP %s", resp.Status)
	}
	if total <= 0 {
		total = expected
	}
	if total > maxDownload {
		return 0, "", fmt.Errorf("分发包大小异常（%d 字节），已拒绝下载", total)
	}

	digest := sha256.New()
	var file *os.File
	if offset > 0 {
		if file, err = openResume(dest, offset, digest); err != nil {
			return 0, "", errRestart
		}
	} else if file, err = os.Create(dest); err != nil {
		return 0, "", fmt.Errorf("无法写入下载文件：%w", err)
	}
	// 停滞看门狗：每收到一段数据就重置，真的卡住时才取消请求，并给出可执行的
	// 说明，而不是把 context canceled 原样抛给用户。
	var stalled atomic.Bool
	watchdog := time.AfterFunc(stallTimeout, func() {
		stalled.Store(true)
		cancel()
	})
	reader := &progressReader{
		reader:   io.LimitReader(resp.Body, maxDownload+1-offset),
		total:    total,
		done:     offset,
		last:     offset,
		progress: progress,
		tick:     func() { watchdog.Reset(stallTimeout) },
	}
	copied, copyErr := io.Copy(io.MultiWriter(file, digest), reader)
	watchdog.Stop()
	closeErr := file.Close()
	if copyErr != nil {
		if stalled.Load() {
			return 0, "", fmt.Errorf("下载停滞超过 %s 没有任何数据，已中断；再次下载会从断点继续，也可以到发布页手动下载", stallTimeout)
		}
		return 0, "", fmt.Errorf("下载中断：%w", copyErr)
	}
	if closeErr != nil {
		return 0, "", fmt.Errorf("保存下载文件失败：%w", closeErr)
	}
	written := offset + copied
	if written > maxDownload {
		return 0, "", fmt.Errorf("分发包超过 %d 字节上限，已拒绝保存", maxDownload)
	}
	return written, hex.EncodeToString(digest.Sum(nil)), nil
}

// openResume 以追加方式打开 .part，并把已有的 offset 字节喂进 digest。
// 文件实际长度对不上 offset（被改动过）时报错，由调用方改为从头下载。
func openResume(path string, offset int64, digest io.Writer) (*os.File, error) {
	existing, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	n, copyErr := io.CopyN(digest, existing, offset)
	existing.Close()
	if copyErr != nil || n != offset {
		return nil, fmt.Errorf("已下载部分不完整")
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
}

// progressReader 汇报下载进度并喂看门狗。进度回调按 128 KiB 节流，避免每读一次
// 就跨进程推一次事件。
type progressReader struct {
	reader   io.Reader
	total    int64
	done     int64
	last     int64
	progress Progress
	tick     func()
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && r.tick != nil {
		r.tick()
	}
	r.done += int64(n)
	if r.progress != nil && (r.done-r.last >= 128<<10 || err == io.EOF) {
		r.last = r.done
		r.progress(r.done, r.total)
	}
	return n, err
}

// DownloadTimeout 是整次下载的兜底上限，真正判断「卡住」的是 stallTimeout。
// 20 MB 的包按 30 分钟算，只有持续低于约 12 KB/s 的链路才会触顶；之前的 10 分钟
// 会把 35 KB/s 以下但一直有数据的慢链路在下载中途砍掉，与「慢而稳定不误杀」矛盾。
const DownloadTimeout = 30 * time.Minute
