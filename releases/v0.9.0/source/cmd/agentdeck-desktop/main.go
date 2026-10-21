// Command agentdeck-desktop 是 AgentDeck 的桌面入口(Phase 2)。
//
// 这里是全程序唯一的 wails 触点:internal/app 服务层不 import wails,
// 事件推送/剪贴板等能力经回调注入,服务层可脱离窗口环境单测。
package main

import (
	"embed"
	"fmt"
	"os"
	"slices"
	"time"

	appkg "github.com/enpiaohj/agentdeck/internal/app"
	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
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

// relaunchFlag 标记本次启动是数据位置变更后的自动重启。
//
// 新实例不能立刻要求单实例所有权：上一个实例还在走正常退出路径（释放
// 互斥锁、清除运行标记）。带这个标记时耐心等它退出，而不是判定「已在
// 运行」直接退出——否则重启永远只是关掉应用。
const relaunchFlag = "--relaunch"

// relaunchWait 是自动重启时等待上一个实例退出的上限。
const relaunchWait = 20 * time.Second

func main() {
	e := &lateEmitter{}
	relaunch := slices.Contains(os.Args[1:], relaunchFlag)
	trayIcon, iconErr := assets.ReadFile("assets/appicon-256.png")
	trayAvailable := iconErr == nil && len(trayIcon) > 0
	startup := desktopStartup()
	var lifecycle *desktopLifecycle
	a, err := appkg.New(appkg.Config{
		Emit: e.Emit,
		Clipboard: func(text string) error {
			if !application.Get().Clipboard.SetText(text) {
				return fmt.Errorf("写入剪贴板失败")
			}
			return nil
		},
		Restart:            func() error { return relaunchSelf(lifecycle.exit) },
		StartAtLoginStatus: startup.enabled,
		SetStartAtLogin:    startup.set,
		TrayAvailable:      trayAvailable,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "装配失败:", err)
		os.Exit(1)
	}
	defer a.Close()

	// 单实例：已有窗口在运行时直接退出。此前只有 session.lock 文件标记，
	// 多开一次就被误判成「上次未正常退出」，崩溃横幅会无端亮起。
	deadline := time.Now().Add(relaunchWait)
	for {
		if !a.ClaimSingleInstance() {
			break
		}
		if !relaunch || time.Now().After(deadline) {
			if !slices.Contains(os.Args[1:], startupFlag) {
				activateExistingWindow()
			}
			fmt.Fprintln(os.Stderr, "AgentDeck 已在运行，本次启动已退出。")
			os.Exit(0)
		}
		time.Sleep(200 * time.Millisecond)
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
			application.NewService(a.NewProjectService()),
			application.NewService(a.NewSettingsService()),
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
		Hidden:    startupHidden(os.Args[1:], trayAvailable),
	})
	e.win = win
	lifecycle = &desktopLifecycle{closeToTray: a.CloseToTrayEnabled, hide: func() { win.Hide() }, quit: wails.Quit}
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if lifecycle.closeRequested() {
			event.Cancel()
		}
	})
	open := func() {
		if win.IsMinimised() {
			win.Restore()
		}
		win.Show()
		win.Focus()
	}
	activation := &activationGate{dispatch: func() { application.InvokeAsync(open) }}
	wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { activation.start() })
	if cleanup, err := listenForActivation(activationEventName, activation.request); err == nil {
		defer cleanup()
	} else {
		fmt.Fprintln(os.Stderr, "无法注册窗口激活通知:", err)
	}
	defer activation.stop()
	if trayAvailable {
		menu := wails.NewMenu()
		menu.Add("打开 AgentDeck").OnClick(func(*application.Context) { open() })
		menu.AddSeparator()
		menu.Add("退出").OnClick(func(*application.Context) { lifecycle.exit() })
		tray := wails.SystemTray.New().SetIcon(trayIcon).SetMenu(menu).OnClick(open).OnDoubleClick(open)
		tray.SetTooltip("AgentDeck")
	}

	// 启动即后台重建全文索引;进度/完成经 index:progress / index:ready 推送
	a.NewSearchService().Rebuild()

	if err := wails.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// relaunchSelf 启动一个新的自己并退出当前进程。
//
// 新实例带 relaunchFlag，会等本进程走完正常退出（defer a.Close() 释放
// 单实例互斥锁、清除运行标记），因此不会出现「重启后什么都没发生」。
// 不用 os.Exit 硬退：那会留下假的「上次异常退出」标记。
func relaunchSelf(quit func()) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位 AgentDeck 程序路径：%w", err)
	}
	cmd := proc.Command(exe, relaunchFlag)
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("启动新实例失败：%w", err)
	}
	// 先让本次调用返回前端（界面要显示正在重启），再退出。
	go func() {
		time.Sleep(300 * time.Millisecond)
		quit()
	}()
	return nil
}
