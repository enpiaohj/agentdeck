package update

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

// proxyFunc 决定一次请求走哪个代理。
//
// 先按 Go 的惯例读环境变量（HTTPS_PROXY / HTTP_PROXY / NO_PROXY）；只要环境里配置过
// 任何一个代理变量，就完全交给它，不再看系统设置——用户显式写的优先。环境里没有时
// 才读 Windows 的系统代理（「设置 → 网络 → 代理」，也是 Clash 等工具的「系统代理」
// 模式写入的位置）。Go 默认只认环境变量，于是只开系统代理的用户检查更新会一直
// 显示「无法访问 GitHub」。
func proxyFunc(req *http.Request) (*url.URL, error) {
	if envProxyConfigured() {
		return http.ProxyFromEnvironment(req)
	}
	return systemProxyURL(req.URL), nil
}

func envProxyConfigured() bool {
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return true
		}
	}
	return false
}

// parseProxyServer 解析 Windows 的 ProxyServer 值，返回 scheme 对应的代理地址。
//
// 两种写法：单个 "host:port"（所有协议共用），或 "http=host:port;https=host:port;socks=…"。
// 只支持 HTTP 代理；socks 项、空值与无法识别的内容返回空串（宁可直连，也不把请求
// 发给一个按错协议说话的代理）。
func parseProxyServer(server, scheme string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return ""
	}
	pick := server
	if strings.Contains(server, "=") {
		pick = ""
		for _, part := range strings.FieldsFunc(server, func(r rune) bool { return r == ';' || r == ' ' }) {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			switch strings.ToLower(key) {
			case scheme:
				pick = value
			case "http":
				// https 没有单独配置时沿用 http 的那一项。
				if pick == "" && scheme == "https" {
					pick = value
				}
			}
		}
	}
	pick = strings.TrimSpace(pick)
	if pick == "" || strings.HasPrefix(strings.ToLower(pick), "socks") {
		return ""
	}
	if !strings.Contains(pick, "://") {
		pick = "http://" + pick
	}
	u, err := url.Parse(pick)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.String()
}

// proxyBypassed 判断 host 是否在 Windows 的「例外」列表里（分号分隔、支持 * 通配，
// 「<local>」表示不含点的内网主机名）。
func proxyBypassed(host, override string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, pattern := range strings.Split(override, ";") {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		switch {
		case pattern == "":
			continue
		case pattern == "<local>":
			if !strings.Contains(host, ".") {
				return true
			}
		default:
			if ok, _ := path.Match(pattern, host); ok {
				return true
			}
		}
	}
	return false
}
