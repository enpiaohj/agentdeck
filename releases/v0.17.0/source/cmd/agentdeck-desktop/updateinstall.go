package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/enpiaohj/agentdeck/internal/updateinstall"
)

// updateInstaller 把校验通过的更新包在进程退出时落地为新的可执行文件。
//
// 一切都在退出的最后一步进行：正在运行的 exe 不能被覆盖。因此 Stage 只做
// 可写性检查并记下更新包，真正的写入由 Finish 在窗口关闭、数据库关闭、
// 单实例互斥锁释放之后执行（见 main 的 defer 顺序）。
type updateInstaller struct {
	mu       sync.Mutex
	current  string // 当前运行的可执行文件
	package_ string // 已登记待落地的更新包
	target   string // 新版本落地后的路径
	retarget func(oldPath, newPath string) bool
}

// newUpdateInstaller 装配安装器。retarget 负责把自启动项改指向新路径并报告是否
// 确认改完；快捷方式由 updateinstall 自己处理（见 handOver）。
func newUpdateInstaller(retarget func(oldPath, newPath string) bool) *updateInstaller {
	exe, err := os.Executable()
	if err != nil {
		// 定位不到自己就不提供应用内更新：装错地方比不装更糟。
		return nil
	}
	return &updateInstaller{current: exe, retarget: retarget}
}

// Stage 登记一个已校验的更新包，返回新版本将落到的路径。
//
// 现在就确认所在目录可写：退出时已经没有界面可以报错，权限问题必须提前
// 暴露给用户。
func (i *updateInstaller) Stage(packagePath string) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := updateinstall.Probe(i.current); err != nil {
		return "", fmt.Errorf("%w。请把 AgentDeck 移到有写入权限的目录，或到发布页手动下载", err)
	}
	if info, err := os.Stat(packagePath); err != nil || info.IsDir() || info.Size() == 0 {
		return "", fmt.Errorf("更新包不可用，请重新下载")
	}
	i.package_ = packagePath
	i.target = updateinstall.ResolveTarget(i.current, packagePath)
	return i.target, nil
}

// Finish 在退出的最后一步落地已登记的更新包，返回应启动的程序路径。
//
// 没有登记时返回当前路径。落地失败时原文件保持可用，并留下标记供下次启动提示。
func (i *updateInstaller) Finish() string {
	i.mu.Lock()
	pkg, target, current := i.package_, i.target, i.current
	i.package_, i.target = "", ""
	i.mu.Unlock()
	if pkg == "" || target == "" {
		return current
	}
	if updateinstall.Install(current, target, pkg) {
		return target
	}
	usable := updateinstall.UsablePath(current)
	updateinstall.MarkFailed(usable)
	return usable
}

// cleanupPrevious 在启动时善后上一次更新：清掉遗留的临时文件与回滚副本，
// 文件名变过的更新则确认启动项与快捷方式已改指向后删除旧文件。
//
// 顺带对齐快捷方式名：目标已是最新、名字却还写着旧版本的条目会自己出现
// （跨版本手工替换、改名失败后重试），而那时「上一个版本」早已不是它，改指向
// 那条路匹配不到。这里按当前 exe 名对齐一次，让这类残留自愈。
func (i *updateInstaller) cleanupPrevious() {
	if i == nil {
		return
	}
	healStaleTarget(i.current)
	updateinstall.AlignShortcutNames(i.current)
	updateinstall.Cleanup(i.current, i.handOver)
}

// handOver 把自启动项与快捷方式从 oldPath 改指向 newPath，返回是否确认改完。
//
// 两条时机都要用：退出的最后一步（落地后立刻改指向，否则下次从快捷方式启动的
// 还是旧版本），以及新版本启动时的善后（退出的那一次可能没成功）。
func (i *updateInstaller) handOver(oldPath, newPath string) bool {
	ok := true
	if i.retarget != nil {
		ok = i.retarget(oldPath, newPath)
	}
	updateinstall.RetargetShortcuts(oldPath, newPath)
	return ok && !updateinstall.ShortcutsPointingTo(oldPath)
}

// launch 启动指定程序。返回的错误自解释，供重启路径原样展示。
func launch(exe string, args ...string) error {
	cmd := proc.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 %s 失败：%w", filepath.Base(exe), err)
	}
	return nil
}
