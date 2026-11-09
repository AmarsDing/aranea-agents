package auth

import "testing"

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
