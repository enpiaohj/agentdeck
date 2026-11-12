package update

import "testing"

func TestParseProxyServer(t *testing.T) {
	cases := []struct{ server, scheme, want string }{
		{"127.0.0.1:7890", "https", "http://127.0.0.1:7890"},
		{"http://127.0.0.1:7890", "https", "http://127.0.0.1:7890"},
		{"http=10.0.0.1:8080;https=10.0.0.2:8443", "https", "http://10.0.0.2:8443"},
		{"http=10.0.0.1:8080;https=10.0.0.2:8443", "http", "http://10.0.0.1:8080"},
		{"http=10.0.0.1:8080", "https", "http://10.0.0.1:8080"},
		{"socks=127.0.0.1:1080", "https", ""},
		{"socks5://127.0.0.1:1080", "https", ""},
		{"ftp=10.0.0.1:21", "https", ""},
		{"", "https", ""},
		{"   ", "https", ""},
	}
	for _, c := range cases {
		if got := parseProxyServer(c.server, c.scheme); got != c.want {
			t.Errorf("parseProxyServer(%q, %q) = %q，期望 %q", c.server, c.scheme, got, c.want)
		}
	}
}

func TestProxyBypassed(t *testing.T) {
	override := "localhost;127.*;10.*;*.corp.example;<local>"
	for host, want := range map[string]bool{
		"localhost":          true,
		"127.0.0.1":          true,
		"intranet":           true,
		"build.corp.example": true,
		"github.com":         false,
		"api.github.com":     false,
	} {
		if got := proxyBypassed(host, override); got != want {
			t.Errorf("proxyBypassed(%q) = %v，期望 %v", host, got, want)
		}
	}
	if proxyBypassed("github.com", "") {
		t.Error("空的例外列表不应放行任何主机")
	}
}
