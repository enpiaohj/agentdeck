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

	// 单实例：已有窗口在运行时直接退出。此前只有 session.lock 文件标记，
	// 多开一次就被误判成「上次未正常退出」，崩溃横幅会无端亮起。
	if a.ClaimSingleInstance() {
		fmt.Fprintln(os.Stderr, "AgentDeck 已在运行，本次启动已退出。")
		os.Exit(0)
	}

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

	// 窗口初始尺寸按屏幕工作区自适应(逻辑像素):设计基准 1280×800、
	// 最小 960×600,但小屏(如 200% DPI 笔记本逻辑宽仅 ~720)按屏缩减,
	// 否则窗口被系统钳制到屏宽、三栏布局横向溢出视口。
	initW, initH := 1280, 800
	minW, minH := 960, 600
	if sw, sh := workAreaLogical(); sw > 0 && sh > 0 {
		if initW > sw {
			initW = sw
		}
		if initH > sh {
			initH = sh
		}
		if minW > sw {
			minW = sw
		}
		if minH > sh {
			minH = sh
		}
	}
	win := wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "AgentDeck",
		X:         -1, // 居中
		Y:         -1,
		Width:     initW,
		Height:    initH,
		MinWidth:  minW,
		MinHeight: minH,
		// 无边框:标题栏由前端自绘(顶栏品牌区即拖拽区,右侧为窗口控制按钮),
		// 以便整屏都是应用自身的内容层级。
		Frameless: true,
	})
	e.win = win

	// 启动即后台重建全文索引;进度/完成经 index:progress / index:ready 推送
	a.NewSearchService().Rebuild()

	if err := wails.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
