package auth

import (
	"net/http/httptest"
	"testing"
)

// TestWebConsoleURL 钉死「浏览器误访问 API 端口」的分流：
// 只有文档导航（Accept: text/html）的根路径才 302 到前端站点；
// 接口路径与无 text/html 的 API 客户端保持原 401 语义。
func TestWebConsoleURL(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		host   string
		accept string
		want   string
	}{
		{"根路径_浏览器导航", "/", "127.0.0.1:8810", "text/html,application/xhtml+xml", "http://127.0.0.1:9402/"},
		{"登录路径_浏览器导航", "/login", "192.168.0.108:8810", "text/html", "http://192.168.0.108:9402/"},
		{"接口路径不跳转", "/v1/admins/current", "127.0.0.1:8810", "text/html", ""},
		{"非浏览器请求不跳转", "/", "127.0.0.1:8810", "application/json", ""},
		{"健康检查不跳转", "/healthz", "127.0.0.1:8810", "text/html", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+c.host+c.path, nil)
			r.Host = c.host
			r.Header.Set("Accept", c.accept)
			if got := webConsoleURL(r); got != c.want {
				t.Errorf("webConsoleURL(%s, accept=%q) = %q, want %q", c.path, c.accept, got, c.want)
			}
		})
	}
}

func TestWebConsoleURL_EnvOverride(t *testing.T) {
	t.Setenv("ARANEA_WEB_URL", "https://aranea.example.com/")
	r := httptest.NewRequest("GET", "http://127.0.0.1:8810/", nil)
	r.Host = "127.0.0.1:8810"
	r.Header.Set("Accept", "text/html")
	if got := webConsoleURL(r); got != "https://aranea.example.com/" {
		t.Errorf("webConsoleURL env override = %q", got)
	}
}

// TestIsNoAuthPath_ICalFeedPublic 钉死 E 阶段 K-3：ics 只读订阅 token-in-path
// 公开端点豁免 JWT（日历客户端无法带 Bearer，令牌即凭据、吊销即 401）。
// 豁免是前缀级；令牌段本身不参与判定。相邻前缀不得误豁免（如 tokens 管理面）。
func TestIsNoAuthPath_ICalFeedPublic(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/v1/office/calendar/feeds/d224ecd4c1f30aaed224997d8d765f621e4d22c7e7b934fb75a3776ae7de51ec", true},
		{"/v1/office/calendar/feeds/", true},
		{"/v1/office/calendar/tokens", false}, // 管理面需登录
		{"/v1/office/calendar/feedsx", false}, // 相邻前缀不误豁免
		{"/v1/office/events", false},          // 日程面需登录
		{"/v1/admins/login", true},            // 既有精确豁免不回归
		{"/v1/a2a/public/anything", true},     // 既有前缀豁免不回归
	}
	for _, c := range cases {
		if got := isNoAuthPath(c.path); got != c.want {
			t.Errorf("isNoAuthPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
