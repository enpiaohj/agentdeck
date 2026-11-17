package main

import (
	"slices"
	"sync"
	"sync/atomic"
)

const startupFlag = "--startup"

// 保留启动期间的激活请求，Wails 平台就绪后才能分发 UI 操作。
type activationGate struct {
	mu                      sync.Mutex
	ready, pending, stopped bool
	dispatch                func()
}

func (g *activationGate) request() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return
	}
	if !g.ready {
		g.pending = true
		return
	}
	g.dispatch()
}

func (g *activationGate) start() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return
	}
	g.ready = true
	if g.pending {
		g.pending = false
		g.dispatch()
	}
}

func (g *activationGate) stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stopped = true
}

// 显式退出及数据迁移重启必须绕过关闭到托盘。
type desktopLifecycle struct {
	closeToTray func() bool
	hide        func()
	quit        func()
	quitting    atomic.Bool
}

func (l *desktopLifecycle) closeRequested() bool {
	if !l.quitting.Load() && l.closeToTray() {
		l.hide()
		return true
	}
	return false
}

func (l *desktopLifecycle) exit() {
	if l.quitting.CompareAndSwap(false, true) {
		l.quit()
	}
}

func startupHidden(args []string, trayAvailable bool) bool {
	return trayAvailable && slices.Contains(args, startupFlag)
}
