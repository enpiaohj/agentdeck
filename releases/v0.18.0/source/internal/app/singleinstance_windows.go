//go:build windows

package app

// 单实例守卫：进程级的命名互斥锁，与 session.lock 文件标记互补。
//
// session.lock 只回答「上次是否异常退出」，它是文件标记而不是所有权：
// 第二个实例启动时发现 lock 存在就判「上次崩过」，于是每次误开两个窗口
// 都会亮一次崩溃横幅。互斥锁先回答「现在是否已有一个我」——已有一个就
// 拒绝启动，lock 从此只在真正的异常退出后残留。

import (
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW   = kernel32.NewProc("CreateMutexW")
	procReleaseMutex   = kernel32.NewProc("ReleaseMutex")
	procCloseHandle    = kernel32.NewProc("CloseHandle")
	errorAlreadyExists = syscall.Errno(183) // ERROR_ALREADY_EXISTS
)

// claimSingleInstance 尝试取得本应用唯一实例的所有权。返回的 release 在
// 进程退出时调用；already 为 true 表示已有实例在运行，调用方应直接退出。
func claimSingleInstance(name string) (release func(), already bool) {
	ptr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		// 名字取不出 UTF-16 时不做拦截：宁可多开一个窗口，也不要启动不了。
		return func() {}, false
	}
	h, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(ptr)))
	if h == 0 {
		// 互斥锁创建失败（如句柄耗尽）同样不拦截。
		return func() {}, false
	}
	if err == errorAlreadyExists {
		procCloseHandle.Call(h)
		return func() {}, true
	}
	return func() {
		procReleaseMutex.Call(h)
		procCloseHandle.Call(h)
	}, false
}
