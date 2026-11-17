//go:build windows

package update

import (
	"net/url"

	"golang.org/x/sys/windows/registry"
)

const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// systemProxyURL 读当前用户的 Windows 系统代理。
//
// 只处理「手动设置的 HTTP 代理」（ProxyEnable=1 + ProxyServer）；自动配置脚本（PAC）
// 需要执行脚本才能知道某个地址走哪里，这里不做，按直连处理。读取失败同样直连：
// 检查更新不该因为注册表读不到而多出一种失败。
func systemProxyURL(target *url.URL) *url.URL {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()
	if enabled, _, err := key.GetIntegerValue("ProxyEnable"); err != nil || enabled == 0 {
		return nil
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return nil
	}
	if override, _, err := key.GetStringValue("ProxyOverride"); err == nil && proxyBypassed(target.Hostname(), override) {
		return nil
	}
	raw := parseProxyServer(server, target.Scheme)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return u
}
