// Command agentdeck-desktop 是 AgentDeck 的桌面入口(Phase 2)。
//
// 这里是全程序唯一的 wails 触点:internal/app 服务层不 import wails,
// 事件推送/剪贴板等能力经回调注入,服务层可脱离窗口环境单测。
package main

import (
	"embed"
	"fmt"
	"os"

	appkg "github.com/enpiaohj/agentdeck/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:assets
var assets embed.FS

// lateEmitter 把服务层 Emitter 延迟绑定到 wails 窗口:services 在
// application.New 之前构造,而窗口对象在其之后才存在。
type lateEmitter struct {
	win *application.WebviewWindow
}

func (e *lateEmitter) Emit(event string, data any) {
	if e.win != nil {
		e.win.EmitEvent(event, data)
	}
}

func main() {
	e := &lateEmitter{}
	a, err := appkg.New(appkg.Config{
		Emit: e.Emit,
		Clipboard: func(text string) error {
			if !application.Get().Clipboard.SetText(text) {
				return fmt.Errorf("写入剪贴板失败")
			}
			return nil
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "装配失败:", err)
		os.Exit(1)
	}
	defer a.Close()

	wails := application.New(application.Options{
		Name:        "AgentDeck",
		Description: "跨 Agent 历史会话工作台",
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Services: []application.Service{
			application.NewService(a.NewSessionService()),
			application.NewService(a.NewSearchService()),
			application.NewService(a.NewMarkService()),
			application.NewService(a.NewResumeService()),
		},
	})

	win := wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "AgentDeck",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
	})
	e.win = win

	// 启动即后台重建全文索引;进度/完成经 index:progress / index:ready 推送
	a.NewSearchService().Rebuild()

	if err := wails.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
