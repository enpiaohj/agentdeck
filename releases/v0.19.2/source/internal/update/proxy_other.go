//go:build !windows

package update

import "net/url"

// systemProxyURL 在非 Windows 平台没有系统代理可读，一律直连（环境变量由调用方处理）。
func systemProxyURL(*url.URL) *url.URL { return nil }
