package update

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// maxDownload 是单个资产的下载上限。正式包不到 20 MB，留足余量同时挡住
// 异常响应把磁盘写满。
const maxDownload = 256 << 20

// Progress 是下载进度回调：done 为已下载字节，total 为总字节（未知时为 0）。
type Progress func(done, total int64)

// Download 把 Release 分发包下载到 dir 并解压，返回解压后的目录。
//
// 步骤：取 SHA256SUMS → 下载分发 ZIP → 比对哈希 → 解压到 dir/vX.Y.Z/。
// 任何一步失败都不留下半成品：先写临时文件，校验通过才改名；解压目录
// 最后一次性改名到位。
func (c Client) Download(ctx context.Context, result Result, dir string, progress Progress) (string, error) {
	if !result.Available {
		return "", fmt.Errorf("没有可下载的新版本")
	}
	if result.ExeAsset.URL == "" || result.SumsAsset.URL == "" {
		return "", fmt.Errorf("发布缺少分发文件或校验清单")
	}
	client := c.httpClient()
	sums, err := c.fetchSums(ctx, client, result.SumsAsset.URL)
	if err != nil {
		return "", err
	}
	// 清单里记的是每个资产自身的哈希：ZIP 的哈希用于校验下载内容，
	// exe 的哈希在解压后再比对一次（见 extract）。
	zipName := strings.TrimSuffix(result.ExeAsset.Name, ".exe") + ".zip"
	want, ok := sums[zipName]
	if !ok {
		return "", fmt.Errorf("校验清单里没有 %s", zipName)
	}

	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建下载目录：%w", err)
	}
	zipPath := filepath.Join(dir, zipName+".part")
	defer os.Remove(zipPath)

	got, err := c.downloadTo(ctx, client, result.ExeAsset.URL, zipPath, progress)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", fmt.Errorf("下载文件的 SHA-256 与发布清单不一致（期望 %s，实际 %s），已丢弃", want, got)
	}

	final := filepath.Join(dir, zipName)
	if err = os.Rename(zipPath, final); err != nil {
		return "", fmt.Errorf("无法保存下载文件：%w", err)
	}
	out, err := extract(final, dir, fmt.Sprintf("AgentDeck-v%s-win-x64", result.Latest), result, sums)
	if err != nil {
		os.Remove(final)
		return "", err
	}
	return out, nil
}

// fetchSums 读取 SHA256SUMS，返回「文件名 → 小写十六进制哈希」。
// 清单格式与 sha256sum 一致：<hash> *<name>。
func (c Client) fetchSums(ctx context.Context, client *http.Client, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgentPrefix+firstNonEmpty(c.Version, "dev"))
	resp, err := client.Do(req)
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

// downloadTo 下载 url 到 dest，返回内容的 SHA-256。边写边算，不把整个
// 文件读进内存。
func (c Client) downloadTo(ctx context.Context, client *http.Client, url, dest string, progress Progress) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgentPrefix+firstNonEmpty(c.Version, "dev"))
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("无法下载更新：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载更新失败：HTTP %s", resp.Status)
	}
	if resp.ContentLength > maxDownload {
		return "", fmt.Errorf("分发包大小异常（%d 字节），已拒绝下载", resp.ContentLength)
	}
	file, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("无法写入下载文件：%w", err)
	}
	digest := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, digest), &progressReader{
		reader:   io.LimitReader(resp.Body, maxDownload+1),
		total:    resp.ContentLength,
		progress: progress,
	})
	closeErr := file.Close()
	if copyErr != nil {
		return "", fmt.Errorf("下载中断：%w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("保存下载文件失败：%w", closeErr)
	}
	if written > maxDownload {
		return "", fmt.Errorf("分发包超过 %d 字节上限，已拒绝保存", maxDownload)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// progressReader 汇报下载进度。进度回调按 128 KiB 节流，避免每读一次
// 就跨进程推一次事件。
type progressReader struct {
	reader   io.Reader
	total    int64
	done     int64
	last     int64
	progress Progress
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.done += int64(n)
	if r.progress != nil && (r.done-r.last >= 128<<10 || err == io.EOF) {
		r.last = r.done
		r.progress(r.done, r.total)
	}
	return n, err
}

// extract 解压分发 ZIP 到 dir/<root>/，返回该目录。
//
// 只接受以 expectedRoot 开头的条目，拒绝绝对路径、盘符与 ..：发布包由
// 本项目生成，但下载内容一律当作不可信输入处理。
func extract(zipPath, dir, expectedRoot string, result Result, sums map[string]string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("下载的文件不是有效的 ZIP：%w", err)
	}
	defer reader.Close()

	target := filepath.Join(dir, expectedRoot)
	// 只清理本目录内、名字正是我们生成的那一个解压目录：不碰用户放在
	// 下载目录里的其他东西。
	if filepath.Dir(target) != filepath.Clean(dir) {
		return "", fmt.Errorf("解压目标 %q 越出了下载目录", target)
	}
	staging := target + ".part"
	if err = os.RemoveAll(staging); err != nil {
		return "", fmt.Errorf("无法清理临时目录：%w", err)
	}
	defer os.RemoveAll(staging)

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(strings.ReplaceAll(entry.Name, "\\", "/"))
		if !strings.HasPrefix(name, expectedRoot+"/") {
			return "", fmt.Errorf("分发包包含意外路径 %q，已停止解压", entry.Name)
		}
		rel := strings.TrimPrefix(name, expectedRoot+"/")
		if rel == "" || strings.HasPrefix(rel, "../") || strings.Contains(rel, ":") {
			return "", fmt.Errorf("分发包包含意外路径 %q，已停止解压", entry.Name)
		}
		if entry.UncompressedSize64 > maxDownload {
			return "", fmt.Errorf("分发包内的文件过大：%s", entry.Name)
		}
		dest := filepath.Join(staging, filepath.FromSlash(rel))
		if err = extractOne(entry, dest); err != nil {
			return "", err
		}
	}

	// 清单里记的是 exe 的哈希，用来确认解压出来的程序就是发布的那一个。
	exePath := filepath.Join(staging, result.ExeAsset.Name)
	got, err := hashFile(exePath)
	if err != nil {
		return "", fmt.Errorf("分发包内没有 %s：%w", result.ExeAsset.Name, err)
	}
	if want, ok := sums[result.ExeAsset.Name]; ok && got != want {
		return "", fmt.Errorf("%s 的 SHA-256 与发布清单不一致，已丢弃", result.ExeAsset.Name)
	}

	os.RemoveAll(target)
	if err = os.Rename(staging, target); err != nil {
		return "", fmt.Errorf("无法完成解压：%w", err)
	}
	return target, nil
}

func extractOne(entry *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("无法创建目录：%w", err)
	}
	src, err := entry.Open()
	if err != nil {
		return fmt.Errorf("无法读取分发包：%w", err)
	}
	defer src.Close()
	// 0o755：解压出来的 exe 要能直接替换运行。
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("无法写入 %s：%w", filepath.Base(dest), err)
	}
	if _, err = io.Copy(out, src); err != nil {
		out.Close()
		return fmt.Errorf("解压 %s 失败：%w", filepath.Base(dest), err)
	}
	if err = out.Close(); err != nil {
		return fmt.Errorf("解压 %s 失败：%w", filepath.Base(dest), err)
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// DownloadTimeout 是整次下载的上限。18 MB 的包在慢网络上也能完成，同时
// 避免请求无限期挂着。
const DownloadTimeout = 10 * time.Minute
