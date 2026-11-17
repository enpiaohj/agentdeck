package app

// ChatGPT 会话里的图片在阅读区用 /_chatgpt/… 地址显示，这个地址只在 AgentDeck
// 自己的窗口里有意义。导出 Markdown 时把图片拷到导出目录的 assets/ 下，并把
// 正文里的引用改成相对路径，导出的文件离开 AgentDeck 也能看到图。

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/enpiaohj/agentdeck/internal/chatgpthistory"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

const exportAssetsDir = "assets"

// exportImages 返回 parts 的副本，其中已导入的 ChatGPT 图片引用改指向
// destDir/assets/ 下的拷贝；拷不出来的退回占位文字。
func (a *App) exportImages(parts []sessions.Part, destDir string) []sessions.Part {
	out := make([]sessions.Part, len(parts))
	copy(out, parts)
	root := a.chatgptDir()
	for i, p := range out {
		if p.Text == "" {
			continue
		}
		out[i].Text = chatgpthistory.AssetAddress.ReplaceAllStringFunc(p.Text, func(m string) string {
			address := chatgpthistory.AssetAddress.FindStringSubmatch(m)[2]
			file, name, ok := chatgpthistory.AssetFile(root, address)
			if !ok || copyExportAsset(file, filepath.Join(destDir, exportAssetsDir), name) != nil {
				return "*[图片]*"
			}
			return "![图片](" + exportAssetsDir + "/" + name + ")"
		})
	}
	return out
}

// copyExportAsset 把 src 拷成 dir/name。名字是内容的哈希，目标已存在就是同一张图，不重复写。
func copyExportAsset(src, dir, name string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, name)
	if _, err := os.Lstat(target); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, ".agentdeck-asset-*.tmp")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
