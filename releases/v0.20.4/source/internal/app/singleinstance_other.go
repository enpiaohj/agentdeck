//go:build !windows

package app

// 非 Windows 平台没有命名互斥锁的现成路径，而单实例对桌面工作台不是
// 核心需求：这里退化为不拦截。多开时 session.lock 会误报一次崩溃，
// Windows 上的互斥锁才是这个问题的解。

func claimSingleInstance(string) (func(), bool) { return func() {}, false }
